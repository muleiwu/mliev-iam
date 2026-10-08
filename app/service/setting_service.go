package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"gorm.io/gorm"
)

const settingCacheTTL = time.Hour

func settingCacheKey(key string) string {
	return "setting:" + key
}

// Get 返回指定 key 的设置值；未命中返回空字符串。
// 替代原 internal/helper.GetSetting，内部走 go-web cache + 项目 dao。
func (s *SettingService) Get(key string) string {
	return s.GetWithDefault(key, "")
}

// GetWithDefault 返回指定 key 的设置值，未命中时回退到默认值。
func (s *SettingService) GetWithDefault(key, defaultValue string) string {
	ctx := context.Background()

	if cache := helper.GetCache(); cache != nil {
		var cached string
		if err := cache.Get(ctx, settingCacheKey(key), &cached); err == nil && cached != "" {
			return cached
		}
	}

	setting, err := s.settingDao.GetByKey(key)
	if err != nil {
		return defaultValue
	}

	if cache := helper.GetCache(); cache != nil {
		_ = cache.Set(ctx, settingCacheKey(key), setting.Value, settingCacheTTL)
	}

	return setting.Value
}

// SetSimple 更新 key 对应的设置值并失效缓存；用于替代原 helper.SetSetting。
// 区别于 Update(key, req): SetSimple 只写 value，不动 category/remark。
func (s *SettingService) SetSimple(key, value string) error {
	if err := s.settingDao.UpdateByKey(key, value); err != nil {
		return err
	}
	if cache := helper.GetCache(); cache != nil {
		ctx := context.Background()
		_ = cache.Del(ctx, settingCacheKey(key))
	}
	return nil
}

const (
	// RegisterFieldsConfigKey 注册字段配置的key
	RegisterFieldsConfigKey = "register_fields_config"
	// RegisterCategory 注册相关配置分类
	RegisterCategory = "register"
	// PushSettingsConfigKey 推送服务配置的key
	PushSettingsConfigKey = "push_settings_config"
	// PushCategory 推送相关配置分类
	PushCategory = "push"
	// CaptchaSceneBindingsKey 验证码场景绑定配置的key
	CaptchaSceneBindingsKey = "captcha_scene_bindings"
	// CaptchaCategory 验证码相关配置分类
	CaptchaCategory = "captcha"
	// StorageSettingsConfigKey 对象存储配置的key
	StorageSettingsConfigKey = "storage_settings_config"
	// StorageCategory 存储相关配置分类
	StorageCategory = "storage"
)

// SettingService 设置服务
type SettingService struct {
	db         *gorm.DB
	settingDao *dao.SettingDao
}

// NewSettingService 创建设置服务实例
func NewSettingService(db *gorm.DB) *SettingService {
	return &SettingService{
		db:         db,
		settingDao: dao.NewSettingDao(db),
	}
}

// GetByKey 根据key获取设置
func (s *SettingService) GetByKey(key string) (*dto.SettingResponse, error) {
	setting, err := s.settingDao.GetByKey(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("设置不存在: %s", key)
		}
		return nil, fmt.Errorf("获取设置失败: %w", err)
	}

	return s.convertToResponse(setting), nil
}

// GetByCategory 根据分类获取设置列表
func (s *SettingService) GetByCategory(category string) ([]dto.SettingResponse, error) {
	settings, err := s.settingDao.GetByCategory(category)
	if err != nil {
		return nil, fmt.Errorf("获取设置列表失败: %w", err)
	}

	return s.convertToResponses(settings), nil
}

// GetAll 获取所有设置
func (s *SettingService) GetAll() ([]dto.SettingResponse, error) {
	settings, err := s.settingDao.GetAll()
	if err != nil {
		return nil, fmt.Errorf("获取所有设置失败: %w", err)
	}

	return s.convertToResponses(settings), nil
}

// Create 创建设置
func (s *SettingService) Create(req *dto.CreateSettingRequest) (*dto.SettingResponse, error) {
	// 检查key是否已存在
	exists, err := s.settingDao.Exists(req.Key)
	if err != nil {
		return nil, fmt.Errorf("检查设置是否存在失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("设置key已存在: %s", req.Key)
	}

	setting := &model.Setting{
		Key:      req.Key,
		Value:    req.Value,
		Category: req.Category,
		Remark:   req.Remark,
	}

	if err := s.settingDao.Create(setting); err != nil {
		return nil, fmt.Errorf("创建设置失败: %w", err)
	}

	return s.convertToResponse(setting), nil
}

// Update 更新设置
func (s *SettingService) Update(key string, req *dto.UpdateSettingRequest) (*dto.SettingResponse, error) {
	setting, err := s.settingDao.GetByKey(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("设置不存在: %s", key)
		}
		return nil, fmt.Errorf("获取设置失败: %w", err)
	}

	setting.Value = req.Value
	if req.Category != "" {
		setting.Category = req.Category
	}
	if req.Remark != "" {
		setting.Remark = req.Remark
	}

	if err := s.settingDao.Update(setting); err != nil {
		return nil, fmt.Errorf("更新设置失败: %w", err)
	}

	return s.convertToResponse(setting), nil
}

// Delete 删除设置
func (s *SettingService) Delete(key string) error {
	exists, err := s.settingDao.Exists(key)
	if err != nil {
		return fmt.Errorf("检查设置是否存在失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("设置不存在: %s", key)
	}

	if err := s.settingDao.DeleteByKey(key); err != nil {
		return fmt.Errorf("删除设置失败: %w", err)
	}

	return nil
}

// GetRegisterFieldsSettings 获取注册字段配置
func (s *SettingService) GetRegisterFieldsSettings() (*dto.RegisterFieldsSettings, error) {
	setting, err := s.settingDao.GetByKey(RegisterFieldsConfigKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认配置
			return s.getDefaultRegisterFieldsSettings(), nil
		}
		return nil, fmt.Errorf("获取注册字段配置失败: %w", err)
	}

	var config dto.RegisterFieldsSettings
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		// 如果解析失败，返回默认配置
		return s.getDefaultRegisterFieldsSettings(), nil
	}
	s.ensureRegisterFieldsDefaults(&config, []byte(setting.Value))

	return &config, nil
}

// UpdateRegisterFieldsSettings 更新注册字段配置
func (s *SettingService) UpdateRegisterFieldsSettings(req *dto.UpdateRegisterFieldsRequest) error {
	defaults := s.getDefaultRegisterFieldsSettings()
	config := dto.RegisterFieldsSettings{
		Username: s.normalizeRegisterUsernameConfig(req.Username, defaults.Username),
		Email:    s.normalizeRegisterFieldConfig(req.Email, defaults.Email),
		Phone:    s.normalizeRegisterFieldConfig(req.Phone, defaults.Phone),
		RealName: s.normalizeRegisterFieldConfig(req.RealName, defaults.RealName),
		Nickname: s.normalizeRegisterFieldConfig(req.Nickname, defaults.Nickname),
	}
	if err := s.validateRegisterIdentityFields(config); err != nil {
		return err
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	setting := &model.Setting{
		Key:      RegisterFieldsConfigKey,
		Value:    string(configJSON),
		Category: RegisterCategory,
		Remark:   "注册字段配置",
	}

	if err := s.settingDao.CreateOrUpdate(setting); err != nil {
		return fmt.Errorf("保存注册字段配置失败: %w", err)
	}

	return nil
}

// getDefaultRegisterFieldsSettings 获取默认的注册字段配置
func (s *SettingService) getDefaultRegisterFieldsSettings() *dto.RegisterFieldsSettings {
	return &dto.RegisterFieldsSettings{
		Username: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    true,
			Label:       "用户名",
			Placeholder: "请输入用户名",
		},
		Email: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    true,
			Label:       "邮箱地址",
			Placeholder: "请输入邮箱地址",
		},
		Phone: dto.RegisterFieldConfig{
			Visible:     false,
			Required:    false,
			Label:       "手机号码",
			Placeholder: "请输入手机号码",
		},
		RealName: dto.RegisterFieldConfig{
			Visible:     true,
			Required:    true,
			Label:       "真实姓名",
			Placeholder: "请输入真实姓名",
		},
		Nickname: dto.RegisterFieldConfig{
			Visible:     false,
			Required:    false,
			Label:       "昵称",
			Placeholder: "请输入昵称",
		},
	}
}

func (s *SettingService) ensureRegisterFieldsDefaults(config *dto.RegisterFieldsSettings, rawValue []byte) {
	defaults := s.getDefaultRegisterFieldsSettings()
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rawValue, &raw)

	if _, ok := raw["username"]; !ok {
		config.Username = defaults.Username
	} else {
		config.Username = s.normalizeRegisterFieldConfig(config.Username, defaults.Username)
	}
	config.Email = s.normalizeRegisterFieldConfig(config.Email, defaults.Email)
	config.Phone = s.normalizeRegisterFieldConfig(config.Phone, defaults.Phone)
	config.RealName = s.normalizeRegisterFieldConfig(config.RealName, defaults.RealName)
	config.Nickname = s.normalizeRegisterFieldConfig(config.Nickname, defaults.Nickname)
}

func (s *SettingService) normalizeRegisterFieldConfig(config, defaultConfig dto.RegisterFieldConfig) dto.RegisterFieldConfig {
	if config.Required {
		config.Visible = true
	}
	if config.Label == "" {
		config.Label = defaultConfig.Label
	}
	if config.Placeholder == "" {
		config.Placeholder = defaultConfig.Placeholder
	}
	return config
}

func (s *SettingService) normalizeRegisterUsernameConfig(config, defaultConfig dto.RegisterFieldConfig) dto.RegisterFieldConfig {
	if config.Label == "" && config.Placeholder == "" && !config.Visible && !config.Required {
		return defaultConfig
	}
	return s.normalizeRegisterFieldConfig(config, defaultConfig)
}

func (s *SettingService) validateRegisterIdentityFields(config dto.RegisterFieldsSettings) error {
	identityFields := []dto.RegisterFieldConfig{config.Username, config.Email, config.Phone}

	hasVisible := false
	hasRequired := false
	for _, field := range identityFields {
		hasVisible = hasVisible || field.Visible
		hasRequired = hasRequired || field.Required
	}

	if !hasVisible {
		return fmt.Errorf("用户名、邮箱、手机号码至少需要开启一个")
	}
	if !hasRequired {
		return fmt.Errorf("用户名、邮箱、手机号码至少需要设置一个必填项")
	}
	return nil
}

// ValidateRegisterFields 验证注册字段是否满足配置要求
func (s *SettingService) ValidateRegisterFields(req *dto.CreateUserRequest) error {
	config, err := s.GetRegisterFieldsSettings()
	if err != nil {
		// 如果获取配置失败，使用默认配置
		config = s.getDefaultRegisterFieldsSettings()
	}

	// 验证用户名
	if config.Username.Required && req.Username == "" {
		return fmt.Errorf("用户名为必填项")
	}

	// 验证邮箱
	if config.Email.Required && req.Email == "" {
		return fmt.Errorf("邮箱为必填项")
	}

	// 验证手机
	if config.Phone.Required && req.Phone == "" {
		return fmt.Errorf("手机号码为必填项")
	}

	// 验证真实姓名
	if config.RealName.Required && req.RealName == "" {
		return fmt.Errorf("真实姓名为必填项")
	}

	// 验证昵称
	if config.Nickname.Required && req.Nickname == "" {
		return fmt.Errorf("昵称为必填项")
	}

	return nil
}

// convertToResponse 将模型转换为响应DTO
func (s *SettingService) convertToResponse(setting *model.Setting) *dto.SettingResponse {
	return &dto.SettingResponse{
		ID:        setting.ID,
		Key:       setting.Key,
		Value:     setting.Value,
		Category:  setting.Category,
		Remark:    setting.Remark,
		CreatedAt: setting.CreatedAt,
		UpdatedAt: setting.UpdatedAt,
	}
}

// convertToResponses 批量转换模型为响应DTO
func (s *SettingService) convertToResponses(settings []model.Setting) []dto.SettingResponse {
	responses := make([]dto.SettingResponse, len(settings))
	for i, setting := range settings {
		responses[i] = *s.convertToResponse(&setting)
	}
	return responses
}

// GetPushSettings 获取推送服务配置
func (s *SettingService) GetPushSettings() (*dto.PushSettings, error) {
	setting, err := s.settingDao.GetByKey(PushSettingsConfigKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认配置
			return s.getDefaultPushSettings(), nil
		}
		return nil, fmt.Errorf("获取推送服务配置失败: %w", err)
	}

	var config dto.PushSettings
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		// 如果解析失败，返回默认配置
		return s.getDefaultPushSettings(), nil
	}

	return &config, nil
}

// UpdatePushSettings 更新推送服务配置
func (s *SettingService) UpdatePushSettings(req *dto.UpdatePushSettingsRequest) error {
	return s.UpdatePushSettingsWithContext(context.Background(), req)
}

func (s *SettingService) UpdatePushSettingsWithContext(ctx context.Context, req *dto.UpdatePushSettingsRequest) error {
	config := dto.PushSettings(*req)
	if config.SMSChannelID < 0 || config.EmailChannelID < 0 {
		return invalidPushConfig("通道 ID 不能为负数")
	}
	if config.SMSChannelID > 0 || config.EmailChannelID > 0 {
		push, err := NewPushService(config.Credentials())
		if err != nil {
			return err
		}
		for _, kind := range []string{"sms", "email"} {
			id, _, _ := pushChannelSettings(config, kind)
			if id > 0 {
				if err := push.ValidateChannel(ctx, config, kind); err != nil {
					return fmt.Errorf("%s配置校验失败: %w", kind, err)
				}
			}
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	setting := &model.Setting{
		Key:      PushSettingsConfigKey,
		Value:    string(configJSON),
		Category: PushCategory,
		Remark:   "推送服务配置",
	}

	if err := s.settingDao.CreateOrUpdate(setting); err != nil {
		return fmt.Errorf("保存推送服务配置失败: %w", err)
	}

	return nil
}

// getDefaultPushSettings 获取默认的推送服务配置
func (s *SettingService) getDefaultPushSettings() *dto.PushSettings {
	return &dto.PushSettings{
		BaseURL:        "https://push.mliev.com",
		AppID:          "",
		AppSecret:      "",
		SMSChannelID:   0,
		EmailChannelID: 0,
		SMSSignature:   "",
		EmailSignature: "",
	}
}

// GetStorageSettings 获取对象存储配置
func (s *SettingService) GetStorageSettings() (*dto.StorageSettings, error) {
	setting, err := s.settingDao.GetByKey(StorageSettingsConfigKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认配置
			return s.getDefaultStorageSettings(), nil
		}
		return nil, fmt.Errorf("获取对象存储配置失败: %w", err)
	}

	var config dto.StorageSettings
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		// 如果解析失败，返回默认配置
		return s.getDefaultStorageSettings(), nil
	}

	return &config, nil
}

// UpdateStorageSettings 更新对象存储配置
func (s *SettingService) UpdateStorageSettings(req *dto.UpdateStorageSettingsRequest) error {
	config := dto.StorageSettings{
		Enabled:     req.Enabled,
		Endpoint:    req.Endpoint,
		Region:      req.Region,
		Bucket:      req.Bucket,
		AccessKey:   req.AccessKey,
		SecretKey:   req.SecretKey,
		CDN:         req.CDN,
		UploadPath:  req.UploadPath,
		MaxFileSize: req.MaxFileSize,
		PublicRead:  req.PublicRead,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	setting := &model.Setting{
		Key:      StorageSettingsConfigKey,
		Value:    string(configJSON),
		Category: StorageCategory,
		Remark:   "对象存储配置",
	}

	if err := s.settingDao.CreateOrUpdate(setting); err != nil {
		return fmt.Errorf("保存对象存储配置失败: %w", err)
	}

	return nil
}

// getDefaultStorageSettings 获取默认的对象存储配置
func (s *SettingService) getDefaultStorageSettings() *dto.StorageSettings {
	return &dto.StorageSettings{
		Enabled:     false,
		Region:      "us-east-1",
		UploadPath:  "iam",
		MaxFileSize: 5,
		PublicRead:  false,
	}
}

// GetCaptchaSceneBindings 获取验证码场景绑定配置
func (s *SettingService) GetCaptchaSceneBindings() (*dto.CaptchaSceneBindings, error) {
	setting, err := s.settingDao.GetByKey(CaptchaSceneBindingsKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认配置（全部不启用）
			return s.getDefaultCaptchaSceneBindings(), nil
		}
		return nil, fmt.Errorf("获取验证码场景绑定配置失败: %w", err)
	}

	var bindings dto.CaptchaSceneBindings
	if err := json.Unmarshal([]byte(setting.Value), &bindings); err != nil {
		// 如果解析失败，返回默认配置
		return s.getDefaultCaptchaSceneBindings(), nil
	}

	return &bindings, nil
}

// UpdateCaptchaSceneBindings 更新验证码场景绑定配置
func (s *SettingService) UpdateCaptchaSceneBindings(req *dto.UpdateCaptchaSceneBindingsRequest) error {
	bindings := dto.CaptchaSceneBindings{
		Login:         req.Login,
		Register:      req.Register,
		ResetPassword: req.ResetPassword,
		SendSms:       req.SendSms,
		SendEmail:     req.SendEmail,
	}

	bindingsJSON, err := json.Marshal(bindings)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	setting := &model.Setting{
		Key:      CaptchaSceneBindingsKey,
		Value:    string(bindingsJSON),
		Category: CaptchaCategory,
		Remark:   "验证码场景绑定配置",
	}

	if err := s.settingDao.CreateOrUpdate(setting); err != nil {
		return fmt.Errorf("保存验证码场景绑定配置失败: %w", err)
	}

	return nil
}

// getDefaultCaptchaSceneBindings 获取默认的验证码场景绑定配置
func (s *SettingService) getDefaultCaptchaSceneBindings() *dto.CaptchaSceneBindings {
	return &dto.CaptchaSceneBindings{
		Login:         0, // 0 表示不启用
		Register:      0,
		ResetPassword: 0,
		SendSms:       0,
		SendEmail:     0,
	}
}
