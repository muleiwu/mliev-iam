package captcha

import (
	"fmt"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// CaptchaManager 验证码管理器
type CaptchaManager struct {
	providers      map[string]CaptchaProvider
	db             *gorm.DB
	configDao      *dao.CaptchaConfigDao
	settingService *service.SettingService
}

// NewManager 创建验证码管理器实例
func NewManager(db *gorm.DB) *CaptchaManager {
	manager := &CaptchaManager{
		providers:      make(map[string]CaptchaProvider),
		db:             db,
		configDao:      dao.NewCaptchaConfigDao(db),
		settingService: service.NewSettingService(db),
	}

	// 注册内置提供商
	manager.RegisterProvider(&TencentCaptchaProvider{})
	manager.RegisterProvider(&TencentMiniProgramProvider{})
	manager.RegisterProvider(&GeetestCaptchaProvider{})

	return manager
}

// RegisterProvider 注册验证码提供商
func (m *CaptchaManager) RegisterProvider(provider CaptchaProvider) {
	m.providers[provider.GetProviderType()] = provider
}

// GetProvider 获取验证码提供商
func (m *CaptchaManager) GetProvider(providerType string) (CaptchaProvider, error) {
	provider, ok := m.providers[providerType]
	if !ok {
		return nil, fmt.Errorf("不支持的验证码提供商: %s", providerType)
	}
	return provider, nil
}

// GetClientParamsForScene 根据场景获取前端初始化参数
// scene: 场景名称 (login, register, sendSms, sendEmail, resetPassword)
// 返回 nil 表示该场景未启用验证码
func (m *CaptchaManager) GetClientParamsForScene(scene string) (*dto.CaptchaClientParamsResponse, error) {
	// 获取场景绑定配置
	bindings, err := m.settingService.GetCaptchaSceneBindings()
	if err != nil {
		return nil, fmt.Errorf("获取场景绑定配置失败: %w", err)
	}

	// 获取当前场景绑定的配置ID
	configID := m.getConfigIDForScene(bindings, scene)
	if configID == 0 {
		// 未启用验证码
		return &dto.CaptchaClientParamsResponse{
			Enabled: false,
		}, nil
	}

	// 获取验证码配置
	config, err := m.configDao.GetByID(configID)
	if err != nil {
		return nil, fmt.Errorf("获取验证码配置失败: %w", err)
	}

	if !config.Enabled {
		// 配置已禁用
		return &dto.CaptchaClientParamsResponse{
			Enabled: false,
		}, nil
	}

	// 获取提供商
	provider, err := m.GetProvider(config.Provider)
	if err != nil {
		return nil, err
	}

	// 获取前端参数
	params := provider.GetClientParams(config)

	return &dto.CaptchaClientParamsResponse{
		Enabled:  true,
		Provider: config.Provider,
		Params:   params,
	}, nil
}

// ValidateForScene 根据场景验证ticket
// 业务接口调用此方法进行验证码校验
// 返回 nil 表示验证通过或无需验证
func (m *CaptchaManager) ValidateForScene(scene string, ticket *dto.CaptchaTicket, userIP string, redisClient *redis.Client) error {
	// 获取场景绑定配置
	bindings, err := m.settingService.GetCaptchaSceneBindings()
	if err != nil {
		return fmt.Errorf("获取场景绑定配置失败: %w", err)
	}

	// 获取当前场景绑定的配置ID
	configID := m.getConfigIDForScene(bindings, scene)
	if configID == 0 {
		// 未启用验证码，直接通过
		return nil
	}

	// 获取验证码配置
	config, err := m.configDao.GetByID(configID)
	if err != nil {
		return fmt.Errorf("获取验证码配置失败: %w", err)
	}

	if !config.Enabled {
		// 配置已禁用，直接通过
		return nil
	}

	// 场景需要验证码，但ticket为空
	if ticket == nil || ticket.Data == nil {
		return fmt.Errorf("请完成验证码验证")
	}

	// 验证提供商类型是否匹配
	if ticket.Provider != config.Provider {
		return fmt.Errorf("验证码提供商类型不匹配")
	}

	// 验证会话（设备指纹验证）
	// 场景已启用验证码时必须存在一次性会话：允许省略 sessionUuid 会让调用方
	// 绕过 /api/captcha/params 直接提交票据，验证码形同虚设。
	if redisClient == nil {
		return fmt.Errorf("验证码会话服务不可用")
	}
	if strings.TrimSpace(ticket.SessionUuid) == "" {
		return fmt.Errorf("验证码会话已失效，请重新完成验证")
	}
	fingerprint, ok := ticket.Data["fingerprint"]
	if !ok || fingerprint == "" {
		return fmt.Errorf("缺少设备指纹信息")
	}
	// 先只校验会话（场景 + 指纹），不消费。
	// 极验超时、captcha_output 过期或用户答错时都会返回失败，
	// 这些都属于可在会话有效期内重试的情况，不能提前把会话烧掉。
	sessionService := NewCaptchaSessionService(redisClient)
	if _, err := sessionService.ValidateSession(ticket.SessionUuid, scene, fingerprint); err != nil {
		return fmt.Errorf("验证码会话验证失败: %w", err)
	}

	// 获取提供商
	provider, err := m.GetProvider(config.Provider)
	if err != nil {
		return err
	}

	// 调用提供商验证，传入用户IP
	result, err := provider.Verify(config, ticket.Data, userIP)
	if err != nil {
		return fmt.Errorf("验证码验证失败: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("验证码验证失败: %s", result.Message)
	}

	// 服务商二次校验通过后才消费会话。并发下只有第一个请求能删成功，
	// 保证同一会话最多支撑一次成功的业务校验（防重放）。
	if _, err := sessionService.ConsumeSession(ticket.SessionUuid, scene, fingerprint); err != nil {
		return fmt.Errorf("验证码会话验证失败: %w", err)
	}

	return nil
}

// getConfigIDForScene 根据场景获取配置ID
func (m *CaptchaManager) getConfigIDForScene(bindings *dto.CaptchaSceneBindings, scene string) uint {
	switch scene {
	case "login":
		return bindings.Login
	case "register":
		return bindings.Register
	case "resetPassword":
		return bindings.ResetPassword
	case "sendSms":
		return bindings.SendSms
	case "sendEmail":
		return bindings.SendEmail
	default:
		return 0
	}
}

// GetSupportedProviders 获取支持的验证码提供商列表及参数模板
// 遍历所有已注册的提供商，调用它们的 GetProviderInfo 方法获取配置信息
func (m *CaptchaManager) GetSupportedProviders() []dto.CaptchaProviderInfo {
	var providerInfos []dto.CaptchaProviderInfo
	for _, provider := range m.providers {
		providerInfos = append(providerInfos, provider.GetProviderInfo())
	}
	return providerInfos
}
