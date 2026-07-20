package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 扫码会话状态。
const (
	QRStatePending   = "pending"    // 已创建，等待扫码
	QRStateScanned   = "scanned"    // 已扫码，等待手机端点"确认"
	QRStateConfirmed = "confirmed"  // 手机端确认通过且已绑定，可直接登录
	QRStateNeedsBind = "needs_bind" // 手机端确认通过但 openid 未绑定，PC 侧需走注册/绑定
	QRStateCanceled  = "canceled"   // 手机端取消
	QRStateExpired   = "expired"    // 仅用于状态查询返回值
)

const (
	qrSessionTTL     = 5 * time.Minute
	qrSessionKey     = "external-auth:miniprogram:qrcode:"
	qrDefaultPage    = "pages/login-confirm/login-confirm"
	qrDefaultEnvVer  = "release"
	weChatWXACodeAPI = "https://api.weixin.qq.com/wxa/getwxacodeunlimit"
)

// SocialQRCodeService 扫码登录编排。
// 与 ExternalAuthService 共用 redis/DB；票据状态完全落在 Redis。
type SocialQRCodeService struct {
	db                *gorm.DB
	redis             *redis.Client
	connectionService *IdentityConnectionService
	authService       *ExternalAuthService
	userService       *UserService
	jwtAuthService    *JWTAuthService
	tokenCache        *WeChatAccessTokenCache
	httpClient        *http.Client
}

// NewSocialQRCodeService 创建实例。
// 依赖复用 SocialAuthService 里已有的工具，避免 JWT/用户服务重复初始化。
func NewSocialQRCodeService(db *gorm.DB, rdb *redis.Client) *SocialQRCodeService {
	return &SocialQRCodeService{
		db: db, redis: rdb, connectionService: NewIdentityConnectionService(db),
		authService: NewExternalAuthService(db, rdb), userService: NewUserService(db),
		jwtAuthService: newOptionalJWTAuthService(), tokenCache: NewWeChatAccessTokenCache(rdb),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// qrSession Redis 里存的会话状态
type qrSession struct {
	ScanHandle           string `json:"scanHandle"`
	PollTokenHash        string `json:"pollTokenHash"`
	ConfirmTokenHash     string `json:"confirmTokenHash,omitempty"`
	ConnectionID         uint   `json:"connectionId"`
	ConnectionIdentifier string `json:"connectionIdentifier"`
	State                string `json:"state"`
	OpenID               string `json:"openId,omitempty"`
	UnionID              string `json:"unionId,omitempty"`
	Nickname             string `json:"nickname,omitempty"`
	Avatar               string `json:"avatar,omitempty"`
	UserID               uint   `json:"userId,omitempty"`
	ContinuationID       string `json:"continuationId,omitempty"`
	ContinuationCSRF     string `json:"continuationCsrf,omitempty"`
	SessionID            string `json:"sessionId,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
}

// CreateSession 创建扫码登录票据。
func (s *SocialQRCodeService) CreateSession(
	ctx context.Context,
	providerIdentifier string,
	req *dto.MPQRCodeCreateRequest,
) (*dto.MPQRCodeCreateResponse, error) {
	connection, err := s.connectionService.GetByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}
	if !connection.IsEnabled() {
		return nil, fmt.Errorf("提供商未启用")
	}
	if connection.Category != model.ConnectionCategorySocial || connection.ProviderType != "wechat_miniprogram" {
		return nil, fmt.Errorf("提供商 %s 不是微信小程序类型", providerIdentifier)
	}
	if _, err := BuildAuthenticationAdapter(s.db, connection, s.redis); err != nil {
		return nil, err
	}

	scanHandle := shortTicket()
	pollToken, err := newCsrfToken()
	if err != nil {
		return nil, err
	}

	session := qrSession{
		ScanHandle:           scanHandle,
		PollTokenHash:        qrTokenHash(pollToken),
		ConnectionID:         connection.ID,
		ConnectionIdentifier: connection.Identifier,
		State:                QRStatePending,
		SessionID:            req.SessionID,
		CreatedAt:            time.Now().Unix(),
	}
	if err := s.saveSession(ctx, &session); err != nil {
		return nil, err
	}

	resp := &dto.MPQRCodeCreateResponse{
		ScanHandle: scanHandle,
		PollToken:  pollToken,
		ExpiresIn:  int(qrSessionTTL / time.Second),
	}

	// 尝试抓小程序码；失败不阻断（PC 可退化为展示 scene 字符串让测试人员手工跳进小程序）。
	page := req.Page
	if page == "" {
		page = qrDefaultPage
	}
	envVersion := req.EnvVersion
	if envVersion == "" {
		envVersion = qrDefaultEnvVer
	}
	credentials, credentialErr := s.connectionService.CredentialValues(connection.ID, model.CredentialPurposeAuthentication)
	clientID, _ := credentials["clientId"].(string)
	clientSecret, _ := credentials["clientSecret"].(string)
	if credentialErr == nil {
		if pngB64, err := s.fetchWxacode(ctx, clientID, clientSecret, "h="+scanHandle, page, envVersion); err == nil {
			resp.QRCode = "data:image/png;base64," + pngB64
		} else {
			fmt.Printf("[qrcode] 拉取小程序码失败: %v\n", err)
		}
	} else {
		fmt.Printf("[qrcode] 读取小程序凭据失败: %v\n", credentialErr)
	}

	return resp, nil
}

// GetStatus PC 端轮询状态。
func (s *SocialQRCodeService) GetStatus(
	ctx context.Context,
	scanHandle, pollToken string,
) (*dto.MPQRCodeStatusResponse, error) {
	session, err := s.loadSession(ctx, scanHandle)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &dto.MPQRCodeStatusResponse{State: QRStateExpired}, nil
		}
		return nil, err
	}
	if !qrTokenMatches(session.PollTokenHash, pollToken) {
		return nil, fmt.Errorf("二维码轮询凭据无效")
	}

	resp := &dto.MPQRCodeStatusResponse{
		State:     session.State,
		SessionID: session.SessionID,
	}

	switch session.State {
	case QRStateConfirmed:
		// JWT 不再通过轮询响应返回；controller 直接签发 HttpOnly Cookie。
		userModel, err := s.userService.GetUserModelByID(session.UserID)
		if err != nil || userModel == nil || !userModel.CanLogin() {
			return nil, fmt.Errorf("用户未激活或已禁用")
		}
		userResp, err := s.userService.GetUserByID(session.UserID)
		if err != nil {
			return nil, fmt.Errorf("获取用户信息失败: %w", err)
		}
		resp.User = userResp
		// 确认后立即回收票据，避免重复使用
		_ = s.deleteSession(ctx, scanHandle)

	case QRStateNeedsBind:
		resp.ContinuationID = session.ContinuationID
		resp.ContinuationCSRF = session.ContinuationCSRF
		resp.SocialInfo = &dto.MPSocialBrief{
			Nickname: session.Nickname,
			Avatar:   session.Avatar,
		}
		// 票据转交给 PC 后续调用 /external-auth/bind-with-login 或 /external-auth/register
		// 不在这里删除，避免 PC 页面刷新后丢失引导信息；到期由 Redis TTL 回收
	case QRStateCanceled:
		_ = s.deleteSession(ctx, scanHandle)
	}

	return resp, nil
}

func (s *SocialQRCodeService) GetStatusForConnection(ctx context.Context, identifier, scanHandle, pollToken string) (*dto.MPQRCodeStatusResponse, error) {
	if err := s.ensureSessionConnection(ctx, identifier, scanHandle); err != nil {
		return nil, err
	}
	return s.GetStatus(ctx, scanHandle, pollToken)
}

func (s *SocialQRCodeService) ScanForConnection(ctx context.Context, identifier string, req *dto.MPQRCodeScanRequest) (*dto.MPQRCodeScanResponse, error) {
	if err := s.ensureSessionConnection(ctx, identifier, req.ScanHandle); err != nil {
		return nil, err
	}
	return s.Scan(ctx, req)
}

func (s *SocialQRCodeService) ConfirmForConnection(ctx context.Context, identifier string, req *dto.MPQRCodeConfirmRequest) error {
	if err := s.ensureSessionConnection(ctx, identifier, req.ScanHandle); err != nil {
		return err
	}
	return s.Confirm(ctx, req)
}

func (s *SocialQRCodeService) ensureSessionConnection(ctx context.Context, identifier, ticket string) error {
	session, err := s.loadSession(ctx, ticket)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("扫码会话已过期")
		}
		return err
	}
	if session.ConnectionIdentifier != identifier {
		return fmt.Errorf("扫码票据不属于连接 %s", identifier)
	}
	return nil
}

// Scan 小程序端扫码上报：用 jsCode 换 openid 并写入会话。
func (s *SocialQRCodeService) Scan(
	ctx context.Context,
	req *dto.MPQRCodeScanRequest,
) (*dto.MPQRCodeScanResponse, error) {
	session, err := s.loadSession(ctx, req.ScanHandle)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("扫码会话已过期")
		}
		return nil, err
	}
	if session.State != QRStatePending {
		return nil, fmt.Errorf("扫码会话状态无效: %s", session.State)
	}

	connection, err := s.connectionService.GetByIdentifier(session.ConnectionIdentifier)
	if err != nil {
		return nil, err
	}
	adapter, err := BuildAuthenticationAdapter(s.db, connection, s.redis)
	if err != nil {
		return nil, fmt.Errorf("创建适配器失败: %w", err)
	}
	codeFlow, ok := adapter.(protocol.CodeAuthenticationAdapter)
	if !ok {
		return nil, fmt.Errorf("提供商 %s 不支持 code 登录流程", connection.Identifier)
	}
	profile, err := codeFlow.LoginWithCode(ctx, req.JsCode)
	if err != nil {
		return nil, fmt.Errorf("code 登录失败: %w", err)
	}
	primary, err := primaryLoginSubject(profile)
	if err != nil {
		return nil, err
	}

	// 优先使用小程序上报的 userInfo（来自 wx.getUserProfile）
	nickname := profile.DisplayName
	avatar := profile.Avatar
	if req.UserInfo != nil {
		if req.UserInfo.Nickname != "" {
			nickname = req.UserInfo.Nickname
		}
		if req.UserInfo.Avatar != "" {
			avatar = req.UserInfo.Avatar
		}
	}

	session.State = QRStateScanned
	confirmToken, err := newCsrfToken()
	if err != nil {
		return nil, err
	}
	session.ConfirmTokenHash = qrTokenHash(confirmToken)
	session.OpenID = primary.Value
	for _, identifier := range profile.Identifiers {
		if identifier.Namespace == model.IdentifierNamespaceUnionID {
			session.UnionID = identifier.Value
		}
	}
	session.Nickname = nickname
	session.Avatar = avatar

	resp := &dto.MPQRCodeScanResponse{State: QRStateScanned, Nickname: nickname, Avatar: avatar, ConfirmToken: confirmToken}

	// 查已绑定账号
	var principal model.ExternalPrincipal
	err = s.db.Joins("JOIN external_identifiers ei ON ei.principal_id = external_principals.id").
		Where("ei.connection_id = ? AND ei.namespace = ? AND ei.value = ?", connection.ID, model.IdentifierNamespaceLoginSubject, primary.Value).
		First(&principal).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询绑定账号失败: %w", err)
	}
	if err == nil && principal.UserID != nil && principal.Status == model.PrincipalStatusActive {
		session.UserID = *principal.UserID
		resp.PendingAction = "login"
		if userResp, err := s.userService.GetUserByID(*principal.UserID); err == nil && userResp != nil {
			if userResp.Nickname != "" {
				resp.Nickname = userResp.Nickname
			}
			if userResp.Avatar != "" {
				resp.Avatar = userResp.Avatar
			}
		}
	} else {
		// 预先生成服务端 continuation；PC 只会收到 CSRF，ID 写入 HttpOnly Cookie。
		profile.DisplayName = nickname
		profile.Avatar = avatar
		continuationID, csrf, err := s.authService.storeContinuation(connection, profile, session.SessionID)
		if err != nil {
			return nil, fmt.Errorf("存储临时用户信息失败: %w", err)
		}
		session.ContinuationID = continuationID
		session.ContinuationCSRF = csrf
		resp.PendingAction = "bind"
	}

	if err := s.saveSession(ctx, session); err != nil {
		return nil, err
	}
	return resp, nil
}

// Confirm 小程序端确认或取消。
func (s *SocialQRCodeService) Confirm(
	ctx context.Context,
	req *dto.MPQRCodeConfirmRequest,
) error {
	session, err := s.loadSession(ctx, req.ScanHandle)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("扫码会话已过期")
		}
		return err
	}
	if !qrTokenMatches(session.ConfirmTokenHash, req.ConfirmToken) {
		return fmt.Errorf("二维码确认凭据无效")
	}
	if session.State != QRStateScanned && req.Action == "confirm" {
		return fmt.Errorf("扫码会话尚未完成扫码")
	}

	switch req.Action {
	case "confirm":
		if session.UserID > 0 {
			session.State = QRStateConfirmed
		} else {
			session.State = QRStateNeedsBind
		}
	case "cancel":
		session.State = QRStateCanceled
	default:
		return fmt.Errorf("未知操作: %s", req.Action)
	}

	return s.saveSession(ctx, session)
}

// ==================== Redis 读写 ====================

func (s *SocialQRCodeService) keyFor(scanHandle string) string {
	return qrSessionKey + scanHandle
}

func (s *SocialQRCodeService) saveSession(ctx context.Context, session *qrSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化扫码会话失败: %w", err)
	}
	return s.redis.Set(ctx, s.keyFor(session.ScanHandle), data, qrSessionTTL).Err()
}

func (s *SocialQRCodeService) loadSession(ctx context.Context, scanHandle string) (*qrSession, error) {
	raw, err := s.redis.Get(ctx, s.keyFor(scanHandle)).Result()
	if err != nil {
		return nil, err
	}
	var session qrSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, fmt.Errorf("解析扫码会话失败: %w", err)
	}
	return &session, nil
}

func (s *SocialQRCodeService) deleteSession(ctx context.Context, scanHandle string) error {
	return s.redis.Del(ctx, s.keyFor(scanHandle)).Err()
}

func qrTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func qrTokenMatches(expectedHash, token string) bool {
	if expectedHash == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(qrTokenHash(token))) == 1
}

// ==================== 小程序码拉取 ====================

type wxacodeErrResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (s *SocialQRCodeService) fetchWxacode(
	ctx context.Context,
	clientID, clientSecret string,
	scene, page, envVersion string,
) (string, error) {
	accessToken, err := s.tokenCache.Resolve(ctx, clientID, clientSecret)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]any{
		"scene":       scene,
		"page":        page,
		"check_path":  false,
		"env_version": envVersion,
		"width":       280,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		weChatWXACodeAPI+"?access_token="+accessToken,
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 getwxacodeunlimit 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 成功时微信返回 image/jpeg 或 image/png；失败时返回 JSON。
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "application/json") || strings.Contains(ct, "text/plain") {
		var errResp wxacodeErrResp
		if err := json.Unmarshal(raw, &errResp); err == nil && errResp.ErrCode != 0 {
			return "", fmt.Errorf("getwxacodeunlimit 错误 %d: %s", errResp.ErrCode, errResp.ErrMsg)
		}
	}

	return base64.StdEncoding.EncodeToString(raw), nil
}

// shortTicket 生成一个去掉 hyphen 的 UUID v7，截到 24 位做票据。
func shortTicket() string {
	v7, err := uuid.NewV7()
	if err != nil {
		return strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	}
	s := strings.ReplaceAll(v7.String(), "-", "")
	if len(s) > 24 {
		return s[:24]
	}
	return s
}
