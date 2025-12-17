package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	mlievpush "github.com/mliev-sdk/push-go"
	"github.com/redis/go-redis/v9"
)

// VerificationType 验证类型
type VerificationType string

const (
	VerificationTypeEmail VerificationType = "email"
	VerificationTypePhone VerificationType = "phone"
)

// VerificationCodeConfig 验证码服务配置
type VerificationCodeConfig struct {
	BaseURL        string
	AppID          string
	AppSecret      string
	SMSChannelID   int
	EmailChannelID int
	SMSSignature   string
	EmailSignature string
}

// VerificationCodeService 验证码服务
type VerificationCodeService struct {
	redis      *redis.Client
	pushClient *mlievpush.Client
	config     *VerificationCodeConfig
	keyPrefix  string
	expiry     time.Duration
}

// NewVerificationCodeService 创建验证码服务
func NewVerificationCodeService(redisClient *redis.Client, config *VerificationCodeConfig) *VerificationCodeService {
	var pushClient *mlievpush.Client
	if config != nil && config.BaseURL != "" && config.AppID != "" && config.AppSecret != "" {
		pushClient = mlievpush.NewClient(config.BaseURL, config.AppID, config.AppSecret)
	}

	return &VerificationCodeService{
		redis:      redisClient,
		pushClient: pushClient,
		config:     config,
		keyPrefix:  "verification:code:",
		expiry:     5 * time.Minute, // 验证码5分钟过期
	}
}

// GenerateCode 生成6位随机验证码
func (s *VerificationCodeService) GenerateCode() (string, error) {
	const digits = "0123456789"
	code := make([]byte, 6)

	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", fmt.Errorf("生成验证码失败: %w", err)
		}
		code[i] = digits[n.Int64()]
	}

	return string(code), nil
}

// SendCode 发送验证码
// receiver: 接收者（邮箱或手机号）
// verificationType: 验证类型（email 或 phone）
// sessionID: 会话ID，用于生成唯一的 Redis key
func (s *VerificationCodeService) SendCode(ctx context.Context, receiver string, verificationType VerificationType, sessionID string) error {
	if s.pushClient == nil {
		return fmt.Errorf("推送服务未配置")
	}

	// 检查是否已发送过验证码（防止频繁发送）
	rateLimitKey := s.getRateLimitKey(receiver, verificationType)
	exists, err := s.redis.Exists(ctx, rateLimitKey).Result()
	if err != nil {
		return fmt.Errorf("检查发送频率失败: %w", err)
	}
	if exists > 0 {
		return fmt.Errorf("发送过于频繁，请稍后再试")
	}

	// 生成验证码
	code, err := s.GenerateCode()
	if err != nil {
		return err
	}

	// 存储验证码到 Redis
	codeKey := s.getCodeKey(receiver, verificationType, sessionID)
	if err := s.redis.Set(ctx, codeKey, code, s.expiry).Err(); err != nil {
		return fmt.Errorf("存储验证码失败: %w", err)
	}

	// 设置发送频率限制（60秒内不能重复发送）
	if err := s.redis.Set(ctx, rateLimitKey, "1", 60*time.Second).Err(); err != nil {
		// 频率限制设置失败不影响主流程
		fmt.Printf("设置发送频率限制失败: %v\n", err)
	}

	// 发送验证码
	var channelID int
	SignatureName := ""
	if verificationType == VerificationTypePhone {
		channelID = s.config.SMSChannelID
		SignatureName = s.config.SMSSignature
	} else {
		channelID = s.config.EmailChannelID
		SignatureName = s.config.EmailSignature
	}

	req := &mlievpush.SendMessageRequest{
		ChannelID:     channelID,
		SignatureName: SignatureName,
		Receiver:      receiver,
		TemplateParams: map[string]string{
			"code":        code,
			"expire_time": "5",
		},
	}

	_, err = s.pushClient.SendMessage(ctx, req)
	if err != nil {
		// 发送失败时删除存储的验证码
		s.redis.Del(ctx, codeKey)
		s.redis.Del(ctx, rateLimitKey)
		return fmt.Errorf("发送验证码失败: %w", err)
	}

	return nil
}

// VerifyCode 验证验证码
func (s *VerificationCodeService) VerifyCode(ctx context.Context, receiver string, verificationType VerificationType, sessionID, code string) error {
	codeKey := s.getCodeKey(receiver, verificationType, sessionID)

	// 获取存储的验证码
	storedCode, err := s.redis.Get(ctx, codeKey).Result()
	if err == redis.Nil {
		return fmt.Errorf("验证码不存在或已过期")
	}
	if err != nil {
		return fmt.Errorf("验证失败: %w", err)
	}

	// 比对验证码
	if storedCode != code {
		return fmt.Errorf("验证码错误")
	}

	// 验证成功后删除验证码（一次性使用）
	s.redis.Del(ctx, codeKey)

	return nil
}

// getCodeKey 获取验证码存储的 Redis key
func (s *VerificationCodeService) getCodeKey(receiver string, verificationType VerificationType, sessionID string) string {
	return fmt.Sprintf("%s%s:%s:%s", s.keyPrefix, verificationType, receiver, sessionID)
}

// getRateLimitKey 获取发送频率限制的 Redis key
func (s *VerificationCodeService) getRateLimitKey(receiver string, verificationType VerificationType) string {
	return fmt.Sprintf("%sratelimit:%s:%s", s.keyPrefix, verificationType, receiver)
}

// IsPushServiceConfigured 检查推送服务是否已配置
func (s *VerificationCodeService) IsPushServiceConfigured() bool {
	return s.pushClient != nil
}
