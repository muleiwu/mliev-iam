package protocol

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// CASProtocolHandler CAS协议处理器
type CASProtocolHandler struct {
	db *gorm.DB
}

// NewCASProtocolHandler 创建CAS协议处理器
func NewCASProtocolHandler(db *gorm.DB) *CASProtocolHandler {
	return &CASProtocolHandler{db: db}
}

// ValidateConfig 验证CAS配置
func (h *CASProtocolHandler) ValidateConfig(config map[string]interface{}) error {
	// 验证Service URL
	serviceURL, ok := config["serviceUrl"].(string)
	if !ok || serviceURL == "" {
		return errors.New("Service URL 是必需的")
	}

	// 验证URL格式
	if err := h.validateURLFormat(serviceURL); err != nil {
		return fmt.Errorf("Service URL格式错误: %w", err)
	}

	return nil
}

// PrepareConfig 准备CAS配置
func (h *CASProtocolHandler) PrepareConfig(uuid string, config map[string]interface{}) (map[string]interface{}, error) {
	// 将UUID存储到配置中作为唯一标识
	config["serviceId"] = uuid

	// 初始化属性映射（如果未设置）
	if _, ok := config["attributes"]; !ok {
		config["attributes"] = make(map[string]string)
	}

	return config, nil
}

// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
func (h *CASProtocolHandler) EncryptSensitiveFields(config map[string]interface{}) error {
	// CAS协议暂时没有需要加密的敏感字段
	return nil
}

// CreateApplication 创建CAS应用
func (h *CASProtocolHandler) CreateApplication(app *model.Application) error {
	// CAS协议在创建时不需要额外处理
	return nil
}

// UpdateApplication 更新CAS应用
func (h *CASProtocolHandler) UpdateApplication(app *model.Application, updates map[string]interface{}) error {
	// 如果更新了配置，需要验证
	if protocolConfig, ok := updates["protocolConfig"].(map[string]interface{}); ok {
		// 合并现有配置和更新
		currentConfig := app.ProtocolConfig
		for k, v := range protocolConfig {
			currentConfig[k] = v
		}

		// 验证合并后的配置
		if err := h.ValidateConfig(currentConfig); err != nil {
			return err
		}

		app.ProtocolConfig = currentConfig
	}

	return nil
}

// DeleteApplication 删除CAS应用
func (h *CASProtocolHandler) DeleteApplication(id uint) error {
	// TODO: 在删除应用前，需要清理相关的CAS票据数据
	return nil
}

// GetProtocolDetails 获取CAS协议详细信息
func (h *CASProtocolHandler) GetProtocolDetails(app *model.Application) (interface{}, error) {
	config := app.ProtocolConfig

	// 构造CAS配置响应
	details := map[string]interface{}{
		"serviceId":  config["serviceId"],
		"serviceUrl": config["serviceUrl"],
		"attributes": config["attributes"],
	}

	return details, nil
}

// 私有方法

// validateURLFormat 验证URL格式
func (h *CASProtocolHandler) validateURLFormat(url string) error {
	if url == "" {
		return errors.New("URL不能为空")
	}

	// 简单的URL格式验证
	if len(url) < 7 {
		return errors.New("URL格式无效")
	}

	// 检查是否包含scheme
	hasValidScheme := false
	validSchemes := []string{"http://", "https://"}
	for _, scheme := range validSchemes {
		if len(url) >= len(scheme) && url[:len(scheme)] == scheme {
			hasValidScheme = true
			break
		}
	}

	if !hasValidScheme {
		return errors.New("URL必须使用 http:// 或 https:// scheme")
	}

	return nil
}
