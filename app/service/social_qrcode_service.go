package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
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
	qrSessionKey     = "social:mp_qrcode:"
	qrDefaultPage    = "pages/login-confirm/login-confirm"
	qrDefaultEnvVer  = "release"
	weChatWXACodeAPI = "https://api.weixin.qq.com/wxa/getwxacodeunlimit"
)

// SocialQRCodeService 扫码登录编排。
// 与 SocialAuthService 共用 redis/DB；票据状态完全落在 Redis。
type SocialQRCodeService struct {
	db              *gorm.DB
	redis           *redis.Client
	providerService *SocialProviderService
	authService     *SocialAuthService
	accountDAO      *dao.UserSocialAccountDAO
	userService     *UserService
	jwtAuthService  *JWTAuthService
	tokenCache      *WeChatAccessTokenCache
	httpClient      *http.Client
}

// NewSocialQRCodeService 创建实例。
// 依赖复用 SocialAuthService 里已有的工具，避免 JWT/用户服务重复初始化。
func NewSocialQRCodeService(db *gorm.DB, rdb *redis.Client) *SocialQRCodeService {
	auth := NewSocialAuthService(db, rdb)
	return &SocialQRCodeService{
		db:              db,
		redis:           rdb,
		providerService: NewSocialProviderService(db, rdb),
		authService:     auth,
		accountDAO:      dao.NewUserSocialAccountDAO(db),
		userService:     NewUserService(db),
		jwtAuthService:  newOptionalJWTAuthService(),
		tokenCache:      NewWeChatAccessTokenCache(rdb),
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// qrSession Redis 里存的会话状态
type qrSession struct {
	Ticket             string `json:"ticket"`
	ProviderID         uint   `json:"providerId"`
	ProviderIdentifier string `json:"providerIdentifier"`
	State              string `json:"state"`
	OpenID             string `json:"openId,omitempty"`
	UnionID            string `json:"unionId,omitempty"`
	Nickname           string `json:"nickname,omitempty"`
	Avatar             string `json:"avatar,omitempty"`
	UserID             uint   `json:"userId,omitempty"`
	BindToken          string `json:"bindToken,omitempty"`
	SessionID          string `json:"sessionId,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
}

// CreateSession 创建扫码登录票据。
func (s *SocialQRCodeService) CreateSession(
	ctx context.Context,
	providerIdentifier string,
	req *dto.MPQRCodeCreateRequest,
) (*dto.MPQRCodeCreateResponse, error) {
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}
	if !provider.IsEnabled() {
		return nil, fmt.Errorf("提供商未启用")
	}
	if provider.Type != model.ProviderTypeWeChatMiniProgram {
		return nil, fmt.Errorf("提供商 %s 不是微信小程序类型", providerIdentifier)
	}

	ticket := shortTicket()

	session := qrSession{
		Ticket:             ticket,
		ProviderID:         provider.ID,
		ProviderIdentifier: provider.Identifier,
		State:              QRStatePending,
		SessionID:          req.SessionID,
		CreatedAt:          time.Now().Unix(),
	}
	if err := s.saveSession(ctx, &session); err != nil {
		return nil, err
	}

	resp := &dto.MPQRCodeCreateResponse{
		Ticket:    ticket,
		Scene:     "t=" + ticket,
		ExpiresIn: int(qrSessionTTL / time.Second),
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
	if pngB64, err := s.fetchWxacode(ctx, provider, resp.Scene, page, envVersion); err == nil {
		resp.QRCode = "data:image/png;base64," + pngB64
	} else {
		fmt.Printf("[qrcode] 拉取小程序码失败: %v\n", err)
	}

	return resp, nil
}

// GetStatus PC 端轮询状态。
func (s *SocialQRCodeService) GetStatus(
	ctx context.Context,
	ticket string,
) (*dto.MPQRCodeStatusResponse, error) {
	session, err := s.loadSession(ctx, ticket)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &dto.MPQRCodeStatusResponse{State: QRStateExpired}, nil
		}
		return nil, err
	}

	resp := &dto.MPQRCodeStatusResponse{
		State:     session.State,
		SessionID: session.SessionID,
	}

	switch session.State {
	case QRStateConfirmed:
		// 已确认且已绑定：按需生成 JWT。
		userResp, err := s.userService.GetUserByID(session.UserID)
		if err != nil {
			return nil, fmt.Errorf("获取用户信息失败: %w", err)
		}
		jwtAuthService, err := ensureJWTAuthService(s.jwtAuthService)
		if err != nil {
			return nil, fmt.Errorf("JWT配置错误: %w", err)
		}
		s.jwtAuthService = jwtAuthService
		token, err := jwtAuthService.GenerateToken(userResp)
		if err != nil {
			return nil, fmt.Errorf("生成令牌失败: %w", err)
		}
		resp.Token = token
		resp.User = userResp
		// 确认后立即回收票据，避免重复使用
		_ = s.deleteSession(ctx, ticket)

	case QRStateNeedsBind:
		resp.BindToken = session.BindToken
		resp.SocialInfo = &dto.MPSocialBrief{
			Nickname: session.Nickname,
			Avatar:   session.Avatar,
		}
		// 票据转交给 PC 后续调用 /social/bind-with-login 或 /register-with-social
		// 不在这里删除，避免 PC 页面刷新后丢失引导信息；到期由 Redis TTL 回收
	case QRStateCanceled:
		_ = s.deleteSession(ctx, ticket)
	}

	return resp, nil
}

// Scan 小程序端扫码上报：用 jsCode 换 openid 并写入会话。
func (s *SocialQRCodeService) Scan(
	ctx context.Context,
	req *dto.MPQRCodeScanRequest,
) (*dto.MPQRCodeScanResponse, error) {
	session, err := s.loadSession(ctx, req.Ticket)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("扫码会话已过期")
		}
		return nil, err
	}
	if session.State != QRStatePending {
		return nil, fmt.Errorf("扫码会话状态无效: %s", session.State)
	}

	provider, err := s.providerService.GetProviderByIdentifier(session.ProviderIdentifier)
	if err != nil {
		return nil, err
	}

	adapter, err := protocol.NewSocialProviderAdapter(provider, s.redis)
	if err != nil {
		return nil, fmt.Errorf("创建适配器失败: %w", err)
	}
	codeFlow, ok := adapter.(protocol.CodeFlowAdapter)
	if !ok {
		return nil, fmt.Errorf("提供商 %s 不支持 code 登录流程", provider.Identifier)
	}

	socialUser, err := codeFlow.LoginWithCode(ctx, req.JsCode)
	if err != nil {
		return nil, fmt.Errorf("code 登录失败: %w", err)
	}

	// 优先使用小程序上报的 userInfo（来自 wx.getUserProfile）
	nickname := socialUser.Nickname
	avatar := socialUser.Avatar
	if req.UserInfo != nil {
		if req.UserInfo.Nickname != "" {
			nickname = req.UserInfo.Nickname
		}
		if req.UserInfo.Avatar != "" {
			avatar = req.UserInfo.Avatar
		}
	}

	session.State = QRStateScanned
	session.OpenID = socialUser.OpenID
	session.UnionID = socialUser.UnionID
	session.Nickname = nickname
	session.Avatar = avatar

	resp := &dto.MPQRCodeScanResponse{State: QRStateScanned, Nickname: nickname, Avatar: avatar}

	// 查已绑定账号
	account, err := s.accountDAO.GetByProviderAndOpenID(provider.ID, socialUser.OpenID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询绑定账号失败: %w", err)
	}
	if account != nil {
		session.UserID = account.UserID
		resp.PendingAction = "login"
		if userResp, err := s.userService.GetUserByID(account.UserID); err == nil && userResp != nil {
			if userResp.Nickname != "" {
				resp.Nickname = userResp.Nickname
			}
			if userResp.Avatar != "" {
				resp.Avatar = userResp.Avatar
			}
		}
	} else {
		// 预先生成 bindToken + 存临时社交信息，手机端确认后 PC 端可以直接拿去走绑定/注册
		bindToken := uuid.New().String()
		temp := &TempSocialUserInfo{
			ProviderID:  provider.ID,
			OpenID:      socialUser.OpenID,
			UnionID:     socialUser.UnionID,
			Nickname:    nickname,
			Avatar:      avatar,
			RawUserInfo: socialUser.RawUserInfo,
		}
		if err := s.authService.storeSocialUserTemp(bindToken, temp); err != nil {
			return nil, fmt.Errorf("存储临时用户信息失败: %w", err)
		}
		session.BindToken = bindToken
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
	session, err := s.loadSession(ctx, req.Ticket)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("扫码会话已过期")
		}
		return err
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

func (s *SocialQRCodeService) keyFor(ticket string) string {
	return qrSessionKey + ticket
}

func (s *SocialQRCodeService) saveSession(ctx context.Context, session *qrSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化扫码会话失败: %w", err)
	}
	return s.redis.Set(ctx, s.keyFor(session.Ticket), data, qrSessionTTL).Err()
}

func (s *SocialQRCodeService) loadSession(ctx context.Context, ticket string) (*qrSession, error) {
	raw, err := s.redis.Get(ctx, s.keyFor(ticket)).Result()
	if err != nil {
		return nil, err
	}
	var session qrSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, fmt.Errorf("解析扫码会话失败: %w", err)
	}
	return &session, nil
}

func (s *SocialQRCodeService) deleteSession(ctx context.Context, ticket string) error {
	return s.redis.Del(ctx, s.keyFor(ticket)).Err()
}

// ==================== 小程序码拉取 ====================

type wxacodeErrResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (s *SocialQRCodeService) fetchWxacode(
	ctx context.Context,
	provider *model.SocialProvider,
	scene, page, envVersion string,
) (string, error) {
	accessToken, err := s.tokenCache.Resolve(ctx, provider.ClientID, provider.ClientSecret)
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
