package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// ConsumeSession 原子校验并消费会话。
// 校验场景、指纹后立即删除，避免 GET 与 DEL 之间的竞态让同一会话被并发复用。
func (s *CaptchaSessionService) ConsumeSession(sessionUUID, scene, fingerprint string) (*CaptchaSession, error) {
	if strings.TrimSpace(sessionUUID) == "" {
		return nil, fmt.Errorf("会话不存在或已过期")
	}
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID

	// Lua 保证读取与删除原子化：只有第一个匹配的请求能拿到会话。
	const script = `
local raw = redis.call('GET', KEYS[1])
if not raw then return nil end
if redis.call('DEL', KEYS[1]) == 0 then return nil end
return raw
`
	raw, err := s.redis.Eval(ctx, script, []string{key}).Text()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("会话不存在或已过期")
		}
		return nil, fmt.Errorf("获取会话失败: %w", err)
	}
	if raw == "" {
		return nil, fmt.Errorf("会话不存在或已过期")
	}

	var session CaptchaSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, fmt.Errorf("解析会话数据失败: %w", err)
	}
	// 会话绑定场景：禁止用低风险场景的会话去通过高风险场景。
	if scene != "" && session.Scene != scene {
		return nil, fmt.Errorf("验证码会话与当前场景不匹配")
	}
	if session.Fingerprint != fingerprint {
		return nil, fmt.Errorf("设备指纹不匹配")
	}
	return &session, nil
}

// DeleteSession 删除验证码会话
func (s *CaptchaSessionService) DeleteSession(sessionUUID string) error {
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID
	return s.redis.Del(ctx, key).Err()
}
