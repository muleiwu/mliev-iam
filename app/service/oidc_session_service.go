package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// OIDCSession OIDC授权会话
type OIDCSession struct {
	SessionID           string    `json:"session_id"`
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	Scope               string    `json:"scope"`
	State               string    `json:"state"`
	Nonce               string    `json:"nonce"`
	ResponseType        string    `json:"response_type"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	Prompt              string    `json:"prompt,omitempty"`
	UserID              *uint     `json:"user_id,omitempty"`
	IsAuthenticated     bool      `json:"is_authenticated"`
	IsConsentGranted    bool      `json:"is_consent_granted"`
	AuthTime            time.Time `json:"auth_time,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`

	// CsrfToken 绑定到会话的一次性 CSRF 令牌。由服务端在 CreateSession 时生成，
	// 通过 /api/oidc/auth-info 交给前端；前端在 POST /webhook/oidc/consent 时携带，
	// 服务端使用 subtle.ConstantTimeCompare 校验后再执行授权动作。
	CsrfToken string `json:"csrf_token"`
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
	}

	if err := s.saveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("保存会话失败: %w", err)
	}

	return session, nil
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

// MarkAsAuthenticated 标记会话为已认证
func (s *OIDCSessionService) MarkAsAuthenticated(ctx context.Context, sessionID string, userID uint) error {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.UserID = &userID
	session.IsAuthenticated = true
	session.AuthTime = time.Now()
	session.ExpiresAt = time.Now().Add(30 * time.Minute)

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

	session.UserID = nil
	session.IsAuthenticated = false
	session.IsConsentGranted = false
	session.AuthTime = time.Time{}
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
