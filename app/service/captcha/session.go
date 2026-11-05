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

// ValidateSession 校验会话的场景与指纹，但不消费会话。
// 验证码交互允许在会话有效期内重试，只有服务商二次校验成功后才应消费。
func (s *CaptchaSessionService) ValidateSession(sessionUUID, scene, fingerprint string) (*CaptchaSession, error) {
	session, _, err := s.loadSession(sessionUUID, scene, fingerprint)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// ConsumeSession 原子校验并消费会话。
//
// 语义约定（有意为之）：
//   - 场景或指纹不匹配时返回错误，但不会删除会话；用户可在有效期内修正后重试。
//   - 只有校验通过且 Redis 中原始数据仍与读取时一致，才用 DEL 消费。
//   - 并发请求读取同一 raw 后，只有第一个 compare-and-delete 成功，
//     其余请求拿到 0，保证一个会话最多支撑一次成功的业务校验。
func (s *CaptchaSessionService) ConsumeSession(sessionUUID, scene, fingerprint string) (*CaptchaSession, error) {
	session, raw, err := s.loadSession(sessionUUID, scene, fingerprint)
	if err != nil {
		return nil, err
	}

	// Lua 比较原始值与删除结果，避免 GET 与 DEL 之间被并发复用或覆盖。
	const script = `
local current = redis.call('GET', KEYS[1])
if current ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
`
	deleted, err := s.redis.Eval(context.Background(), script,
		[]string{s.keyPrefix + strings.TrimSpace(sessionUUID)}, raw).Int()
	if err != nil {
		return nil, fmt.Errorf("消费会话失败: %w", err)
	}
	if deleted == 0 {
		return nil, fmt.Errorf("会话不存在、已过期或已被使用")
	}
	return session, nil
}

// loadSession 读取并校验会话，返回原始数据供后续 compare-and-delete 使用。
func (s *CaptchaSessionService) loadSession(sessionUUID, scene, fingerprint string) (*CaptchaSession, string, error) {
	sessionUUID = strings.TrimSpace(sessionUUID)
	if sessionUUID == "" {
		return nil, "", fmt.Errorf("会话不存在或已过期")
	}

	raw, err := s.redis.Get(context.Background(), s.keyPrefix+sessionUUID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, "", fmt.Errorf("会话不存在或已过期")
	}
	if err != nil {
		return nil, "", fmt.Errorf("获取会话失败: %w", err)
	}
	if raw == "" {
		return nil, "", fmt.Errorf("会话不存在或已过期")
	}

	var session CaptchaSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, "", fmt.Errorf("解析会话数据失败: %w", err)
	}
	// 会话绑定场景：禁止用低风险场景的会话去通过高风险场景。
	if scene != "" && session.Scene != scene {
		return nil, "", fmt.Errorf("验证码会话与当前场景不匹配")
	}
	if session.Fingerprint != fingerprint {
		return nil, "", fmt.Errorf("设备指纹不匹配")
	}
	return &session, raw, nil
}

// DeleteSession 删除验证码会话
func (s *CaptchaSessionService) DeleteSession(sessionUUID string) error {
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID
	return s.redis.Del(ctx, key).Err()
}
