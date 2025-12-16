package service

import (
	"encoding/json"
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

const (
	// RegisterFieldsConfigKey 注册字段配置的key
	RegisterFieldsConfigKey = "register_fields_config"
	// RegisterCategory 注册相关配置分类
	RegisterCategory = "register"
	// PushSettingsConfigKey 推送服务配置的key
	PushSettingsConfigKey = "push_settings_config"
	// PushCategory 推送相关配置分类
	PushCategory = "push"
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

	return &config, nil
}

// UpdateRegisterFieldsSettings 更新注册字段配置
func (s *SettingService) UpdateRegisterFieldsSettings(req *dto.UpdateRegisterFieldsRequest) error {
	config := dto.RegisterFieldsSettings{
		Email:    req.Email,
		Phone:    req.Phone,
		RealName: req.RealName,
		Nickname: req.Nickname,
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

// ValidateRegisterFields 验证注册字段是否满足配置要求
func (s *SettingService) ValidateRegisterFields(req *dto.CreateUserRequest) error {
	config, err := s.GetRegisterFieldsSettings()
	if err != nil {
		// 如果获取配置失败，使用默认配置
		config = s.getDefaultRegisterFieldsSettings()
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
	config := dto.PushSettings{
		BaseURL:        req.BaseURL,
		AppID:          req.AppID,
		AppSecret:      req.AppSecret,
		SMSChannelID:   req.SMSChannelID,
		EmailChannelID: req.EmailChannelID,
		SMSSignature:   req.SMSSignature,
		EmailSignature: req.EmailSignature,
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
		SMSChannelID:   1,
		EmailChannelID: 2,
		SMSSignature:   "",
		EmailSignature: "",
	}
}
