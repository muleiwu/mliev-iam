package captcha

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// 凭证状态机取值。
// pending 表示凭证正在被某个请求用于二次校验，used 表示校验已成功且凭证已消费。
// 只有 pending 可以在校验失败后被释放，used 记录必须保留到自然过期。
const (
	proofStatePending = "pending"
	proofStateUsed    = "used"
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

// ProofKey 生成服务商凭证去重键，始终返回非空摘要。
//
// 已内置的提供商按官方字段取值；未登记的第三方提供商退化为
// 「除 fingerprint 外的全部字段」按键名排序后的摘要，避免未知 provider
// 拿到空键后跳过 SetNX 抢占、把同一份凭证跨会话重放。
func ProofKey(provider string, data map[string]string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	parts := proofDigestParts(provider, data)
	raw := provider + "\x00" + strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// proofDigestParts 返回参与去重摘要的稳定字段。
func proofDigestParts(provider string, data map[string]string) []string {
	switch provider {
	case GeetestProviderType:
		// 极验 4.0：流水号 + 验证输出 + 通过标识
		return []string{
			strings.TrimSpace(data["lot_number"]),
			strings.TrimSpace(data["captcha_output"]),
			strings.TrimSpace(data["pass_token"]),
		}
	case TencentProviderType, TencentMiniProgramProviderType:
		// 腾讯云：ticket + randstr
		return []string{
			strings.TrimSpace(data["ticket"]),
			strings.TrimSpace(data["randstr"]),
		}
	}

	// 未知提供商：排除 fingerprint（每次会话/请求都可能不同），
	// 其余字段排序后参与摘要，保证同一份凭证得到同一个键。
	keys := make([]string, 0, len(data))
	for key := range data {
		if strings.EqualFold(strings.TrimSpace(key), "fingerprint") {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		// 凭证里没有任何可区分字段时，退化为按设备指纹限流，
		// 至少不会让所有请求共用一个全局键，也不会退化为空键。
		return []string{"fingerprint=" + strings.TrimSpace(data["fingerprint"])}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, strings.TrimSpace(key)+"="+strings.TrimSpace(data[key]))
	}
	return parts
}

// BeginProof 在调用服务商二次校验之前抢占凭证。
//
// 同一份凭证只允许一个请求进入二次校验；使用 SET NX 抢占而不是等校验
// 成功后再写，避免并发请求都先通过服务商校验、再各自消费不同的会话，
// 从而让一张凭证换出多份登录结果。抢占失败说明凭证正在使用或已用过。
func (s *CaptchaSessionService) BeginProof(provider string, data map[string]string) (string, bool, error) {
	proof := ProofKey(provider, data)
	key := s.proofKeyPrefix() + proof
	ok, err := s.redis.SetNX(context.Background(), key, proofStatePending, s.expiry).Result()
	if err != nil {
		return "", false, fmt.Errorf("凭证状态存储失败: %w", err)
	}
	return key, ok, nil
}

// CommitProof 在服务商校验成功后把凭证从 pending 原子地改为 used。
//
// 必须 compare-and-set：若只做 SET，失败分支 ReleaseProof 删除 key 后，
// 另一个请求可能用同一凭证重新抢到 pending，而此前已通过服务商校验的请求
// 随后会把新的 pending 直接改写成 used，导致已用凭证被复活、可再次兑换结果。
// 只有当前值仍是 pending 才提交；key 已释放或已被提交都返回错误，不静默覆盖。
func (s *CaptchaSessionService) CommitProof(key string) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	const script = `
local current = redis.call('GET', KEYS[1])
if current == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  return 1
end
return 0
`
	committed, err := s.redis.Eval(context.Background(), script, []string{key},
		proofStatePending, proofStateUsed, s.expiry.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("凭证状态提交失败: %w", err)
	}
	if committed == 0 {
		return errors.New("凭证状态已失效，请重新验证")
	}
	return nil
}

// ReleaseProof 在服务商校验失败或出现异常时释放凭证占用，允许用户重试。
//
// 只释放 pending 记录：已提交为 used 的凭证即便后续请求走到失败分支也不能被清掉，
// 否则「先成功提交、再触发一次失败」即可让同一份凭证重新可用。
func (s *CaptchaSessionService) ReleaseProof(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	const script = `
local current = redis.call('GET', KEYS[1])
if current == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`
	_ = s.redis.Eval(context.Background(), script, []string{key}, proofStatePending).Err()
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

func (s *CaptchaSessionService) proofKeyPrefix() string {
	return strings.TrimSuffix(s.keyPrefix, "session:") + "proof:"
}

// DeleteSession 删除验证码会话
func (s *CaptchaSessionService) DeleteSession(sessionUUID string) error {
	ctx := context.Background()
	key := s.keyPrefix + sessionUUID
	return s.redis.Del(ctx, key).Err()
}
