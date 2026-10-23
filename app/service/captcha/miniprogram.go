package captcha

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type MiniProgramManager struct {
	db       *gorm.DB
	redis    *redis.Client
	provider *TencentMiniProgramProvider
}

func NewMiniProgramManager(db *gorm.DB, rdb *redis.Client) *MiniProgramManager {
	return &MiniProgramManager{db: db, redis: rdb, provider: &TencentMiniProgramProvider{}}
}
func (m *MiniProgramManager) configuration(app *model.Application) (*model.CaptchaConfig, bool, error) {
	cfg, err := model.DecodeMiniProgramConfig(app.ProtocolConfig)
	if err != nil {
		return nil, false, err
	}
	inherited := cfg.CaptchaConfigID == nil
	id, err := service.ResolveMiniProgramCaptchaConfigID(m.db, app)
	if err != nil {
		return nil, inherited, err
	}
	if id == 0 {
		return nil, inherited, nil
	}
	config, err := dao.NewCaptchaConfigDao(m.db).GetByID(id)
	if err != nil {
		return nil, inherited, errors.New("小程序验证码配置不可用")
	}
	if !config.Enabled {
		if inherited {
			return nil, true, nil
		}
		return nil, false, errors.New("已选择的小程序验证码配置被停用")
	}
	if !inherited && config.Provider != TencentMiniProgramProviderType {
		return nil, false, errors.New("请选择微信小程序验证码配置")
	}
	return config, inherited, nil
}

type miniCaptchaSession struct {
	ClientID        string
	TransactionHash string
	ConfigHash      string
	Fingerprint     string
}

const miniCaptchaPrefix = "captcha:miniprogram:"

func captchaConfigurationHash(c *model.CaptchaConfig) string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (m *MiniProgramManager) Params(ctx context.Context, app *model.Application, transactionHash string, req dto.MiniProgramCaptchaParamsRequest, ip string) (*dto.CaptchaClientParamsResponse, error) {
	config, _, err := m.configuration(app)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return &dto.CaptchaClientParamsResponse{Enabled: false}, nil
	}
	if config.Provider != TencentMiniProgramProviderType {
		// Keep the existing browser policy intact. Native SDKs report unsupported
		// configuration instead of silently disabling or treating it as a mini ticket.
		return &dto.CaptchaClientParamsResponse{Enabled: true, Provider: config.Provider}, nil
	}
	if m.redis == nil || transactionHash == "" || req.Fingerprint == "" || len(req.Fingerprint) > 256 {
		return nil, errors.New("验证码会话参数无效")
	}
	params, err := miniCaptchaPublicParams(config, time.Now())
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	raw, _ := json.Marshal(miniCaptchaSession{ClientID: app.UUID, TransactionHash: transactionHash, ConfigHash: captchaConfigurationHash(config), Fingerprint: req.Fingerprint})
	if err = m.redis.Set(ctx, miniCaptchaPrefix+id, raw, 5*time.Minute).Err(); err != nil {
		return nil, errors.New("验证码会话服务不可用")
	}
	return &dto.CaptchaClientParamsResponse{Enabled: true, Provider: TencentMiniProgramProviderType, Params: params, SessionUuid: id, ExpiresIn: 300}, nil
}
func (m *MiniProgramManager) Validate(ctx context.Context, app *model.Application, transactionHash string, ticket *dto.CaptchaTicket, ip string) error {
	config, _, err := m.configuration(app)
	if err != nil {
		return err
	}
	if config == nil {
		return nil
	}
	// Browser captcha tickets have no native application/transaction binding.
	// Reject them here even when inherited; only browser endpoints may use them.
	if config.Provider != TencentMiniProgramProviderType {
		return errors.New("小程序验证码类型不支持")
	}
	if m.redis == nil || ticket == nil || ticket.Provider != TencentMiniProgramProviderType || ticket.SessionUuid == "" || len(ticket.SessionUuid) > 64 || ticket.Data["ticket"] == "" {
		return errors.New("请完成小程序验证码验证")
	}
	// Check ownership before consuming: an unrelated transaction must not burn
	// another login's captcha session. Compare-and-delete still fences races.
	key := miniCaptchaPrefix + ticket.SessionUuid
	raw, err := m.redis.Get(ctx, key).Bytes()
	if err != nil {
		return errors.New("验证码会话已过期或已使用")
	}
	var session miniCaptchaSession
	if json.Unmarshal(raw, &session) != nil || session.ClientID != app.UUID || session.TransactionHash != transactionHash || session.ConfigHash != captchaConfigurationHash(config) || session.Fingerprint != ticket.Data["fingerprint"] {
		return errors.New("验证码会话与当前登录不匹配")
	}
	consumed, err := m.redis.Eval(ctx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{key}, string(raw)).Int()
	if err != nil || consumed != 1 {
		return errors.New("验证码会话已过期或已使用")
	}
	if len(ticket.Data["ticket"]) > 8192 {
		return errors.New("小程序验证码票据无效")
	}
	appID, _ := config.Params["captchaAppId"].(string)
	digest := sha256.Sum256([]byte(appID + "\x00" + ticket.Data["ticket"]))
	claimed, err := m.redis.SetNX(ctx, miniCaptchaPrefix+"ticket:"+hex.EncodeToString(digest[:]), "1", 5*time.Minute).Result()
	if err != nil || !claimed {
		return errors.New("验证码票据已使用，请重新验证")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := m.provider.Verify(config, ticket.Data, ip)
	if err != nil {
		return err
	}
	if result == nil || !result.Success {
		return errors.New("小程序验证码未通过，请重新验证")
	}
	return ctx.Err()
}
