package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/open/go-web/pkg/helper"
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
	BaseURL               string
	AppID                 string
	AppSecret             string
	SMSChannelID          int
	EmailChannelID        int
	SMSSignature          string
	EmailSignature        string
	SMSTemplateBindings   map[string]dto.PushTemplateBinding
	EmailTemplateBindings map[string]dto.PushTemplateBinding
}

// VerificationCodeService 验证码服务
type VerificationCodeService struct {
	redis      *redis.Client
	pushClient *PushService
	config     *VerificationCodeConfig
	keyPrefix  string
	expiry     time.Duration
}

// NewVerificationCodeService 创建验证码服务
func NewVerificationCodeService(redisClient *redis.Client, config *VerificationCodeConfig) *VerificationCodeService {
	var pushClient *PushService
	if config != nil && config.BaseURL != "" && config.AppID != "" && config.AppSecret != "" {
		pushClient, _ = NewPushService(dto.PushCredentials{BaseURL: config.BaseURL, AppID: config.AppID, AppSecret: config.AppSecret})
	}

	return &VerificationCodeService{
		redis:      redisClient,
		pushClient: pushClient,
		config:     config,
		keyPrefix:  "verification:code:",
		expiry:     verificationCodeExpiry,
	}
}

// GenerateCode 生成6位随机验证码
func (s *VerificationCodeService) GenerateCode() (string, error) {
	return generateVerificationCode()
}

func generateVerificationCode() (string, error) {
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
func (s *VerificationCodeService) SendCode(ctx context.Context, receiver string, verificationType VerificationType, sessionID, traceID string) error {
	logVerificationCodeInfo(traceID, "发送验证码开始 type=%s receiver=%s session=%s", verificationType, MaskVerificationReceiver(receiver), shortVerificationLogValue(sessionID))

	if s.pushClient == nil {
		logVerificationCodeError(traceID, "推送服务未配置或连接配置无效")
		return fmt.Errorf("推送服务未配置")
	}

	// 检查是否已发送过验证码（防止频繁发送）
	rateLimitKey := s.getRateLimitKey(receiver, verificationType)
	exists, err := s.redis.Exists(ctx, rateLimitKey).Result()
	if err != nil {
		logVerificationCodeError(traceID, "检查发送频率失败 key=%s err=%v", maskVerificationKey(rateLimitKey, receiver), err)
		return fmt.Errorf("检查发送频率失败: %w", err)
	}
	if exists > 0 {
		logVerificationCodeWarn(traceID, "发送频率限制命中 key=%s type=%s receiver=%s", maskVerificationKey(rateLimitKey, receiver), verificationType, MaskVerificationReceiver(receiver))
		return fmt.Errorf("发送过于频繁，请稍后再试")
	}

	// 生成验证码
	code, err := s.GenerateCode()
	if err != nil {
		logVerificationCodeError(traceID, "生成验证码失败 err=%v", err)
		return err
	}

	// 存储验证码到 Redis
	codeKey := s.getCodeKey(receiver, verificationType, sessionID)
	if err := s.redis.Set(ctx, codeKey, code, s.expiry).Err(); err != nil {
		logVerificationCodeError(traceID, "存储验证码失败 key=%s err=%v", maskVerificationKey(codeKey, receiver), err)
		return fmt.Errorf("存储验证码失败: %w", err)
	}
	logVerificationCodeInfo(traceID, "验证码已写入Redis key=%s ttl=%s", maskVerificationKey(codeKey, receiver), s.expiry)

	// 设置发送频率限制（60秒内不能重复发送）
	if err := s.redis.Set(ctx, rateLimitKey, "1", 60*time.Second).Err(); err != nil {
		// 频率限制设置失败不影响主流程
		logVerificationCodeWarn(traceID, "设置发送频率限制失败 key=%s err=%v", maskVerificationKey(rateLimitKey, receiver), err)
	}

	// 发送验证码
	var channelID int
	signatureName := ""
	var bindings map[string]dto.PushTemplateBinding
	if verificationType == VerificationTypePhone {
		channelID = s.config.SMSChannelID
		signatureName = s.config.SMSSignature
		bindings = s.config.SMSTemplateBindings
	} else {
		channelID = s.config.EmailChannelID
		signatureName = s.config.EmailSignature
		bindings = s.config.EmailTemplateBindings
	}
	if channelID <= 0 {
		s.redis.Del(ctx, codeKey)
		s.redis.Del(ctx, rateLimitKey)
		logVerificationCodeError(traceID, "推送渠道未配置 type=%s channelId=%d", verificationType, channelID)
		return fmt.Errorf("%s验证码推送渠道未配置", verificationTypeLabel(verificationType))
	}
	logVerificationCodeInfo(traceID, "调用推送服务 type=%s receiver=%s channelId=%d signatureConfigured=%t", verificationType, MaskVerificationReceiver(receiver), channelID, signatureName != "")

	_, err = s.pushClient.SendCode(ctx, channelID, signatureName, receiver, bindings, code, s.expiry)
	if err != nil {
		// 发送失败时删除存储的验证码
		s.redis.Del(ctx, codeKey)
		s.redis.Del(ctx, rateLimitKey)
		logVerificationCodeError(traceID, "推送服务发送失败 type=%s receiver=%s channelId=%d err=%v", verificationType, MaskVerificationReceiver(receiver), channelID, err)
		return fmt.Errorf("发送验证码失败: %w", err)
	}

	logVerificationCodeInfo(traceID, "验证码发送成功 type=%s receiver=%s channelId=%d", verificationType, MaskVerificationReceiver(receiver), channelID)
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

// MaskVerificationReceiver 脱敏验证码接收人，避免日志泄露完整邮箱或手机号。
func MaskVerificationReceiver(receiver string) string {
	if receiver == "" {
		return ""
	}
	if strings.Contains(receiver, "@") {
		parts := strings.SplitN(receiver, "@", 2)
		local := parts[0]
		domain := parts[1]
		if len(local) <= 1 {
			return local + "***@" + domain
		}
		return local[:1] + "***" + local[len(local)-1:] + "@" + domain
	}
	if len(receiver) <= 7 {
		if len(receiver) <= 2 {
			return "***"
		}
		return receiver[:2] + "***"
	}
	return receiver[:3] + "****" + receiver[len(receiver)-4:]
}

func shortVerificationLogValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value
	}
	return value[:8] + "..." + value[len(value)-4:]
}

func maskVerificationKey(key, receiver string) string {
	if receiver == "" {
		return key
	}
	return strings.ReplaceAll(key, receiver, MaskVerificationReceiver(receiver))
}

func verificationTypeLabel(verificationType VerificationType) string {
	if verificationType == VerificationTypeEmail {
		return "邮箱"
	}
	return "手机"
}

func logVerificationCodeInfo(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Info(formatVerificationCodeLog(traceID, format, args...))
	}
}

func logVerificationCodeWarn(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Warn(formatVerificationCodeLog(traceID, format, args...))
	}
}

func logVerificationCodeError(traceID, format string, args ...any) {
	if logger := helper.GetLogger(); logger != nil {
		logger.Error(formatVerificationCodeLog(traceID, format, args...))
	}
}

func formatVerificationCodeLog(traceID, format string, args ...any) string {
	if traceID == "" {
		traceID = "-"
	}
	return fmt.Sprintf("[verification-code] traceId=%s %s", traceID, fmt.Sprintf(format, args...))
}
