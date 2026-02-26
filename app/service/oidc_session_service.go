package service

import (
	"context"
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
	UserID              *uint     `json:"user_id,omitempty"`
	IsAuthenticated     bool      `json:"is_authenticated"`
	IsConsentGranted    bool      `json:"is_consent_granted"`
	AuthTime            time.Time `json:"auth_time,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// OIDCSessionService OIDC会话管理服务
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
		sessionExpiry: 10 * time.Minute, // 会话默认10分钟过期
	}
}

// CreateSession 创建新的授权会话
func (s *OIDCSessionService) CreateSession(
	clientID, redirectURI, scope, state, nonce, responseType string,
	codeChallenge, codeChallengeMethod string,
) (*OIDCSession, error) {
	sessionID, err := s.generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("生成会话ID失败: %w", err)
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
		IsAuthenticated:     false,
		IsConsentGranted:    false,
		CreatedAt:           now,
		ExpiresAt:           now.Add(s.sessionExpiry),
	}

	// 保存到Redis
	if err := s.saveSession(session); err != nil {
		return nil, fmt.Errorf("保存会话失败: %w", err)
	}

	return session, nil
}

// GetSession 获取会话
func (s *OIDCSessionService) GetSession(sessionID string) (*OIDCSession, error) {
	ctx := context.Background()
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

	// 检查会话是否过期
	if time.Now().After(session.ExpiresAt) {
		s.DeleteSession(sessionID)
		return nil, fmt.Errorf("会话已过期")
	}

	return &session, nil
}

// UpdateSession 更新会话
func (s *OIDCSessionService) UpdateSession(session *OIDCSession) error {
	return s.saveSession(session)
}

// MarkAsAuthenticated 标记会话为已认证
func (s *OIDCSessionService) MarkAsAuthenticated(sessionID string, userID uint) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.UserID = &userID
	session.IsAuthenticated = true
	session.AuthTime = time.Now()
	session.ExpiresAt = time.Now().Add(30 * time.Minute) // 延长会话时间到30分钟

	return s.UpdateSession(session)
}

// MarkAsConsentGranted 标记会话为已授权
func (s *OIDCSessionService) MarkAsConsentGranted(sessionID string) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	if !session.IsAuthenticated {
		return fmt.Errorf("用户尚未认证")
	}

	session.IsConsentGranted = true

	return s.UpdateSession(session)
}

// DeleteSession 删除会话
func (s *OIDCSessionService) DeleteSession(sessionID string) error {
	ctx := context.Background()
	key := s.keyPrefix + sessionID

	return s.redis.Del(ctx, key).Err()
}

// ValidateSession 验证会话是否有效且完整
func (s *OIDCSessionService) ValidateSession(sessionID string) (*OIDCSession, error) {
	session, err := s.GetSession(sessionID)
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
func (s *OIDCSessionService) ExtendSession(sessionID string, duration time.Duration) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.ExpiresAt = time.Now().Add(duration)

	return s.UpdateSession(session)
}

// CleanupExpiredSessions 清理过期会话（可以通过定时任务调用）
func (s *OIDCSessionService) CleanupExpiredSessions() error {
	// Redis会自动清理过期的key，这里可以实现额外的清理逻辑
	// 或者通过SCAN命令查找并删除过期会话
	return nil
}

// 私有方法

// generateSessionID 生成唯一的会话ID
func (s *OIDCSessionService) generateSessionID() (string, error) {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String(), nil
}

// saveSession 保存会话到Redis
func (s *OIDCSessionService) saveSession(session *OIDCSession) error {
	ctx := context.Background()
	key := s.keyPrefix + session.SessionID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化会话数据失败: %w", err)
	}

	// 计算剩余过期时间
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("会话已过期")
	}

	// 保存到Redis，设置过期时间
	return s.redis.Set(ctx, key, data, ttl).Err()
}
