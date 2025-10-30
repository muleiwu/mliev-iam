package protocol

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// SAMLProtocolHandler SAML协议处理器
type SAMLProtocolHandler struct {
	db *gorm.DB
}

// NewSAMLProtocolHandler 创建SAML协议处理器
func NewSAMLProtocolHandler(db *gorm.DB) *SAMLProtocolHandler {
	return &SAMLProtocolHandler{db: db}
}

// ValidateConfig 验证SAML配置
func (h *SAMLProtocolHandler) ValidateConfig(config map[string]interface{}) error {
	// 验证ACS URL
	acsURL, ok := config["acsUrl"].(string)
	if !ok || acsURL == "" {
		return errors.New("ACS URL 是必需的")
	}

	// 可选：验证URL格式
	if err := h.validateURLFormat(acsURL); err != nil {
		return fmt.Errorf("ACS URL格式错误: %w", err)
	}

	// 验证SLO URL（如果提供）
	if slourl, ok := config["sloUrl"].(string); ok && slourl != "" {
		if err := h.validateURLFormat(slourl); err != nil {
			return fmt.Errorf("SLO URL格式错误: %w", err)
		}
	}

	return nil
}

// PrepareConfig 准备SAML配置
func (h *SAMLProtocolHandler) PrepareConfig(uuid string, config map[string]interface{}) (map[string]interface{}, error) {
	// 使用传入的UUID作为EntityID
	config["entityId"] = uuid

	// 设置默认的NameID格式（如果未设置）
	if _, ok := config["nameIdFormat"]; !ok {
		config["nameIdFormat"] = "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified"
	}

	// 设置默认的签名方法（如果未设置）
	if _, ok := config["signatureMethod"]; !ok {
		config["signatureMethod"] = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	}

	return config, nil
}

// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
func (h *SAMLProtocolHandler) EncryptSensitiveFields(config map[string]interface{}) error {
	// SAML协议暂时没有需要加密的敏感字段
	// 证书（Certificate）是公钥，不需要加密
	return nil
}

// CreateApplication 创建SAML应用
func (h *SAMLProtocolHandler) CreateApplication(app *model.Application) error {
	// SAML协议在创建时不需要额外处理
	return nil
}

// UpdateApplication 更新SAML应用
func (h *SAMLProtocolHandler) UpdateApplication(app *model.Application, updates map[string]interface{}) error {
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

// DeleteApplication 删除SAML应用
func (h *SAMLProtocolHandler) DeleteApplication(id uint) error {
	// TODO: 在删除应用前，需要清理相关的SAML会话数据
	return nil
}

// GetProtocolDetails 获取SAML协议详细信息
func (h *SAMLProtocolHandler) GetProtocolDetails(app *model.Application) (interface{}, error) {
	config := app.ProtocolConfig

	// 构造SAML配置响应
	details := map[string]interface{}{
		"entityId":          config["entityId"],
		"acsUrl":            config["acsUrl"],
		"sloUrl":            config["sloUrl"],
		"certificate":       config["certificate"],
		"signatureMethod":   config["signatureMethod"],
		"nameIdFormat":      config["nameIdFormat"],
		"attributeMappings": config["attributeMappings"],
	}

	return details, nil
}

// 私有方法

// validateURLFormat 验证URL格式
func (h *SAMLProtocolHandler) validateURLFormat(url string) error {
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
