package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CaptchaSession 验证码会话数据
type CaptchaSession struct {
	UUID        string    `json:"uuid"`
	Scene       string    `json:"scene"`
	Fingerprint string    `json:"fingerprint"`
	UserAgent   string    `json:"userAgent"`
	Platform    string    `json:"platform"`
	Language    string    `json:"language"`
	Timezone    string    `json:"timezone"`
	ClientIP    string    `json:"clientIp"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CaptchaSessionService 验证码会话管理服务
type CaptchaSessionService struct {
	redis     *redis.Client
	keyPrefix string
	expiry    time.Duration
}

// NewCaptchaSessionService 创建验证码会话服务
func NewCaptchaSessionService(redisClient *redis.Client) *CaptchaSessionService {
	return &CaptchaSessionService{
		redis:     redisClient,
		keyPrefix: "captcha:session:",
		expiry:    5 * time.Minute, // 5分钟过期
	}
}

// CreateSession 创建验证码会话
func (s *CaptchaSessionService) CreateSession(req *dto.CaptchaParamsRequest, clientIP string) (string, error) {
	// 生成UUID
	sessionUUID := uuid.New().String()

	// 创建会话数据
	session := &CaptchaSession{
		UUID:        sessionUUID,
		Scene:       req.Scene,
		Fingerprint: req.Fingerprint,
		UserAgent:   req.UserAgent,
		Platform:    req.Platform,
		Language:    req.Language,
		Timezone:    req.Timezone,
		ClientIP:    clientIP,
		CreatedAt:   time.Now(),
	}

	// 序列化为JSON
	data, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("序列化会话数据失败: %w", err)
	}

	// 存储到Redis
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID
	if err := s.redis.Set(ctx, key, data, s.expiry).Err(); err != nil {
		return "", fmt.Errorf("存储会话失败: %w", err)
	}

	return sessionUUID, nil
}

// GetSession 获取验证码会话
func (s *CaptchaSessionService) GetSession(sessionUUID string) (*CaptchaSession, error) {
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID

	// 从Redis获取
	data, err := s.redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("会话不存在或已过期")
	}
	if err != nil {
		return nil, fmt.Errorf("获取会话失败: %w", err)
	}

	// 反序列化
	var session CaptchaSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("解析会话数据失败: %w", err)
	}

	return &session, nil
}

// ValidateSession 验证会话（灵活模式：指纹必须一致）
func (s *CaptchaSessionService) ValidateSession(sessionUUID string, fingerprint string) error {
	session, err := s.GetSession(sessionUUID)
	if err != nil {
		return err
	}

	// 验证指纹必须一致
	if session.Fingerprint != fingerprint {
		return fmt.Errorf("设备指纹不匹配")
	}

	// 其他字段（IP、UA等）作为日志参考，不强制验证
	// 因为用户可能在移动网络下IP会变化

	return nil
}

// DeleteSession 删除验证码会话
func (s *CaptchaSessionService) DeleteSession(sessionUUID string) error {
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID
	return s.redis.Del(ctx, key).Err()
}
