package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// OIDCSession OIDC授权会话
type OIDCSession struct {
	LoginSessionID              string                    `json:"login_session_id,omitempty"`
	SessionID                   string                    `json:"session_id"`
	ClientID                    string                    `json:"client_id"`
	RedirectURI                 string                    `json:"redirect_uri"`
	Scope                       string                    `json:"scope"`
	State                       string                    `json:"state"`
	Nonce                       string                    `json:"nonce"`
	ResponseType                string                    `json:"response_type"`
	CodeChallenge               string                    `json:"code_challenge,omitempty"`
	CodeChallengeMethod         string                    `json:"code_challenge_method,omitempty"`
	Prompt                      string                    `json:"prompt,omitempty"`
	RegistrationCandidateUserID *uint                     `json:"registration_candidate_user_id,omitempty"`
	RegistrationDecision        string                    `json:"registration_decision,omitempty"`
	UserID                      *uint                     `json:"user_id,omitempty"`
	IsAuthenticated             bool                      `json:"is_authenticated"`
	IsConsentGranted            bool                      `json:"is_consent_granted"`
	AuthTime                    time.Time                 `json:"auth_time,omitempty"`
	Error                       *dto.AuthorizationFailure `json:"error,omitempty"`
	CreatedAt                   time.Time                 `json:"created_at"`
	ExpiresAt                   time.Time                 `json:"expires_at"`

	// CsrfToken 绑定到会话的一次性 CSRF 令牌。注册意图选择后会轮换，
	// 后续授权确认使用新令牌，防止选择请求重放。
	CsrfToken string `json:"csrf_token"`

	// BrowserBindingHash binds a public session_id to the browser that started
	// the authorization transaction. The raw secret only exists transiently and
	// is delivered through an HttpOnly cookie.
	BrowserBindingHash  string `json:"browser_binding_hash"`
	BrowserBindingToken string `json:"-"`
}

type OIDCLoginTicket struct {
	SessionID    string `json:"session_id"`
	BrowserToken string `json:"browser_token"`
}

// newCsrfToken 生成 32 字节随机值并 base64url 编码，用作 CSRF 令牌。
func newCsrfToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OIDCSessionService OIDC会话管理服务
// 所有方法均要求 ctx（从 *gin.Context.Request.Context() 传入），
// 以支持请求级取消和追踪；不再使用 context.Background()。
type OIDCSessionService struct {
	redis         *redis.Client
	keyPrefix     string
	sessionExpiry time.Duration
}

const oidcLoginTicketPrefix = "oidc:login-ticket:"

const (
	OIDCRegistrationChoiceCreate  = "create"
	OIDCRegistrationChoiceCurrent = "current"
	OIDCRegistrationChoiceCancel  = "cancel"
)

var (
	ErrOIDCRegistrationChoiceNotRequired = errors.New("当前会话无需确认注册意图")
	ErrOIDCRegistrationChoiceRequired    = errors.New("请先确认是否创建新账号")
	ErrOIDCRegistrationChoiceResolved    = errors.New("注册意图已确认")
	ErrOIDCRegistrationChoiceCSRF        = errors.New("注册意图 CSRF 校验失败")
	ErrOIDCRegistrationUserChanged       = errors.New("当前 IAM 登录账号已变更")
)

// NewOIDCSessionService 创建OIDC会话服务
func NewOIDCSessionService(redisClient *redis.Client) *OIDCSessionService {
	return &OIDCSessionService{
		redis:         redisClient,
		keyPrefix:     "oidc:session:",
		sessionExpiry: 10 * time.Minute,
	}
}

// CreateSession 创建新的授权会话
func (s *OIDCSessionService) CreateSession(
	ctx context.Context,
	clientID, redirectURI, scope, state, nonce, responseType string,
	codeChallenge, codeChallengeMethod string,
	prompt string,
) (*OIDCSession, error) {
	sessionID, err := s.generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("生成会话ID失败: %w", err)
	}

	csrfToken, err := newCsrfToken()
	if err != nil {
		return nil, fmt.Errorf("生成CSRF令牌失败: %w", err)
	}
	browserToken, err := newCsrfToken()
	if err != nil {
		return nil, fmt.Errorf("生成浏览器绑定密钥失败: %w", err)
	}
	browserDigest := sha256.Sum256([]byte(browserToken))

	now := time.Now()
	session := &OIDCSession{
		SessionID:           sessionID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		State:               state,
		Nonce:               nonce,
		ResponseType:        responseType,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Prompt:              prompt,
		IsAuthenticated:     false,
		IsConsentGranted:    false,
		CreatedAt:           now,
		ExpiresAt:           now.Add(s.sessionExpiry),
		CsrfToken:           csrfToken,
		BrowserBindingHash:  base64.RawURLEncoding.EncodeToString(browserDigest[:]),
		BrowserBindingToken: browserToken,
	}

	if err := s.saveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("保存会话失败: %w", err)
	}

	return session, nil
}

func (s *OIDCSessionService) ValidateBrowserBinding(session *OIDCSession, browserToken string) error {
	if session == nil || session.BrowserBindingHash == "" || browserToken == "" {
		return errors.New("OIDC 会话缺少浏览器绑定")
	}
	digest := sha256.Sum256([]byte(browserToken))
	actual := base64.RawURLEncoding.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(actual), []byte(session.BrowserBindingHash)) != 1 {
		return errors.New("OIDC 会话与当前浏览器不匹配")
	}
	return nil
}

func (s *OIDCSessionService) CreateLoginTicket(ctx context.Context, session *OIDCSession) (string, error) {
	if session == nil || session.SessionID == "" || session.BrowserBindingToken == "" {
		return "", errors.New("OIDC 会话缺少可兑换的浏览器绑定密钥")
	}
	ticket, err := newCsrfToken()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(OIDCLoginTicket{SessionID: session.SessionID, BrowserToken: session.BrowserBindingToken})
	if err != nil {
		return "", err
	}
	if err := s.redis.Set(ctx, oidcLoginTicketPrefix+ticket, payload, 2*time.Minute).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

// ConsumeLoginTicket is deliberately atomic. Concurrent browser loads cannot
// redeem the same ticket into two browser-bound sessions.
func (s *OIDCSessionService) ConsumeLoginTicket(ctx context.Context, ticket string) (*OIDCLoginTicket, error) {
	if ticket == "" {
		return nil, errors.New("login ticket 不能为空")
	}
	raw, err := s.redis.GetDel(ctx, oidcLoginTicketPrefix+ticket).Result()
	if errors.Is(err, redis.Nil) {
		return nil, errors.New("login ticket 已过期或已使用")
	}
	if err != nil {
		return nil, err
	}
	var value OIDCLoginTicket
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, errors.New("login ticket 数据无效")
	}
	session, err := s.GetSession(ctx, value.SessionID)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateBrowserBinding(session, value.BrowserToken); err != nil {
		return nil, err
	}
	return &value, nil
}

// GetSession 获取会话
func (s *OIDCSessionService) GetSession(ctx context.Context, sessionID string) (*OIDCSession, error) {
	key := s.keyPrefix + sessionID

	data, err := s.redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("会话不存在或已过期")
	}
	if err != nil {
		return nil, fmt.Errorf("获取会话失败: %w", err)
	}

	var session OIDCSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("解析会话数据失败: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		_ = s.DeleteSession(ctx, sessionID)
		return nil, fmt.Errorf("会话已过期")
	}

	return &session, nil
}

// UpdateSession 更新会话
func (s *OIDCSessionService) UpdateSession(ctx context.Context, session *OIDCSession) error {
	return s.saveSession(ctx, session)
}

func (s *OIDCSessionService) SetRegistrationCandidate(ctx context.Context, sessionID string, userID uint) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	session.RegistrationCandidateUserID = &userID
	session.RegistrationDecision = ""
	return s.UpdateSession(ctx, session)
}

// ApplyRegistrationChoice atomically consumes the pending registration choice.
// Concurrent clicks and CSRF replay can only commit one transition.
func (s *OIDCSessionService) ApplyRegistrationChoice(
	ctx context.Context,
	sessionID, csrfToken, choice string,
	currentUserID *uint,
) (*OIDCSession, error) {
	key := s.keyPrefix + sessionID
	var updated *OIDCSession

	for attempt := 0; attempt < 3; attempt++ {
		err := s.redis.Watch(ctx, func(tx *redis.Tx) error {
			data, err := tx.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				return fmt.Errorf("会话不存在或已过期")
			}
			if err != nil {
				return err
			}

			var session OIDCSession
			if err := json.Unmarshal(data, &session); err != nil {
				return fmt.Errorf("解析会话数据失败: %w", err)
			}
			if time.Now().After(session.ExpiresAt) {
				return fmt.Errorf("会话已过期")
			}
			deleteSession, err := applyOIDCRegistrationChoice(&session, csrfToken, choice, currentUserID)
			if err != nil {
				return err
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if deleteSession {
					pipe.Del(ctx, key)
					return nil
				}
				payload, marshalErr := json.Marshal(&session)
				if marshalErr != nil {
					return marshalErr
				}
				ttl := time.Until(session.ExpiresAt)
				if ttl <= 0 {
					return fmt.Errorf("会话已过期")
				}
				pipe.Set(ctx, key, payload, ttl)
				return nil
			})
			if err == nil {
				updated = &session
			}
			return err
		}, key)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return updated, err
	}

	return nil, redis.TxFailedErr
}

func applyOIDCRegistrationChoice(
	session *OIDCSession,
	csrfToken, choice string,
	currentUserID *uint,
) (bool, error) {
	if session == nil || !oidcSessionPromptHas(session.Prompt, "create") || session.RegistrationCandidateUserID == nil {
		return false, ErrOIDCRegistrationChoiceNotRequired
	}
	if session.RegistrationDecision != "" {
		return false, ErrOIDCRegistrationChoiceResolved
	}
	if session.CsrfToken == "" || subtle.ConstantTimeCompare([]byte(session.CsrfToken), []byte(csrfToken)) != 1 {
		return false, ErrOIDCRegistrationChoiceCSRF
	}
	if choice != OIDCRegistrationChoiceCreate && choice != OIDCRegistrationChoiceCurrent && choice != OIDCRegistrationChoiceCancel {
		return false, fmt.Errorf("无效的注册意图选择")
	}
	if choice == OIDCRegistrationChoiceCurrent &&
		(currentUserID == nil || *currentUserID != *session.RegistrationCandidateUserID) {
		return false, ErrOIDCRegistrationUserChanged
	}

	session.RegistrationDecision = choice
	if choice == OIDCRegistrationChoiceCancel {
		return true, nil
	}
	csrf, err := newCsrfToken()
	if err != nil {
		return false, err
	}
	session.CsrfToken = csrf
	return false, nil
}

func (s *OIDCSessionService) ResetRegistrationChoice(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.RegistrationDecision != OIDCRegistrationChoiceCurrent || session.Error != nil {
		return nil
	}
	if err := resetAuthenticationState(session); err != nil {
		return err
	}
	session.RegistrationDecision = ""
	return s.UpdateSession(ctx, session)
}

func oidcSessionPromptHas(prompt, value string) bool {
	for _, item := range strings.Fields(prompt) {
		if item == value {
			return true
		}
	}
	return false
}

// MarkAsAuthenticated 标记会话为已认证
func (s *OIDCSessionService) MarkAsAuthenticated(ctx context.Context, sessionID string, userID uint, loginID string) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if loginID == "" {
		return fmt.Errorf("缺少登录会话标识")
	}
	session.LoginSessionID = loginID
	session.UserID = &userID
	session.IsAuthenticated = true
	session.AuthTime = time.Now()
	session.Error = nil
	session.ExpiresAt = time.Now().Add(30 * time.Minute)

	return s.UpdateSession(ctx, session)
}

// MarkAsFailed records a terminal authorization failure without authenticating
// the OIDC transaction. The original request is preserved so the frontend can
// display the outcome and safely return to the validated client redirect URI.
func (s *OIDCSessionService) MarkAsFailed(ctx context.Context, sessionID string, failure *dto.AuthorizationFailure) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.LoginSessionID = ""
	session.UserID = nil
	session.IsAuthenticated = false
	session.IsConsentGranted = false
	session.AuthTime = time.Time{}
	session.Error = failure

	return s.UpdateSession(ctx, session)
}

// ResetAuthentication removes the user binding from an active OIDC session
// while preserving the original authorization request. A fresh CSRF token is
// issued so that a token from the previous authenticated state cannot be
// reused after switching accounts.
func (s *OIDCSessionService) ResetAuthentication(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if err := resetAuthenticationState(session); err != nil {
		return err
	}

	return s.UpdateSession(ctx, session)
}

// MarkAsConsentGranted 标记会话为已授权
func (s *OIDCSessionService) MarkAsConsentGranted(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	if !session.IsAuthenticated {
		return fmt.Errorf("用户尚未认证")
	}

	session.IsConsentGranted = true

	return s.UpdateSession(ctx, session)
}

// DeleteSession 删除会话
func (s *OIDCSessionService) DeleteSession(ctx context.Context, sessionID string) error {
	key := s.keyPrefix + sessionID
	return s.redis.Del(ctx, key).Err()
}

// ValidateSession 验证会话是否有效且完整
func (s *OIDCSessionService) ValidateSession(ctx context.Context, sessionID string) (*OIDCSession, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !session.IsAuthenticated {
		return nil, fmt.Errorf("用户尚未认证")
	}
	if !session.IsConsentGranted {
		return nil, fmt.Errorf("用户尚未授权")
	}
	return session, nil
}

// ExtendSession 延长会话有效期
func (s *OIDCSessionService) ExtendSession(ctx context.Context, sessionID string, duration time.Duration) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	session.ExpiresAt = time.Now().Add(duration)
	return s.UpdateSession(ctx, session)
}

// CleanupExpiredSessions Redis 会自动清理过期 key，这里保留为扩展点
func (s *OIDCSessionService) CleanupExpiredSessions(ctx context.Context) error {
	_ = ctx
	return nil
}

func resetAuthenticationState(session *OIDCSession) error {
	csrfToken, err := newCsrfToken()
	if err != nil {
		return fmt.Errorf("生成CSRF令牌失败: %w", err)
	}

	session.LoginSessionID = ""
	session.UserID = nil
	session.IsAuthenticated = false
	session.IsConsentGranted = false
	session.AuthTime = time.Time{}
	session.Error = nil
	session.CsrfToken = csrfToken

	return nil
}

// 私有方法

func (s *OIDCSessionService) generateSessionID() (string, error) {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String(), nil
}

func (s *OIDCSessionService) saveSession(ctx context.Context, session *OIDCSession) error {
	key := s.keyPrefix + session.SessionID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化会话数据失败: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("会话已过期")
	}

	return s.redis.Set(ctx, key, data, ttl).Err()
}
