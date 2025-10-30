package protocol

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// OAuth2ProtocolHandler OAuth2协议处理器
type OAuth2ProtocolHandler struct {
	db *gorm.DB
}

// NewOAuth2ProtocolHandler 创建OAuth2协议处理器
func NewOAuth2ProtocolHandler(db *gorm.DB) *OAuth2ProtocolHandler {
	return &OAuth2ProtocolHandler{db: db}
}

// ValidateConfig 验证OAuth2配置
func (h *OAuth2ProtocolHandler) ValidateConfig(config map[string]interface{}) error {
	// 验证重定向URI
	redirectUris, ok := config["redirectUris"].([]interface{})
	if !ok || len(redirectUris) == 0 {
		return errors.New("至少需要一个重定向URI")
	}

	// 验证重定向URI格式
	for _, uri := range redirectUris {
		uriStr, ok := uri.(string)
		if !ok {
			return errors.New("重定向URI格式错误")
		}
		if err := h.validateRedirectURIFormat(uriStr); err != nil {
			return fmt.Errorf("重定向URI格式错误 (%s): %w", uriStr, err)
		}
	}

	// 验证授权类型
	grantTypes, ok := config["grantTypes"].([]interface{})
	if !ok || len(grantTypes) == 0 {
		return errors.New("至少需要一个授权类型")
	}

	// 验证客户端类型
	clientType, ok := config["clientType"].(string)
	if !ok || (clientType != "confidential" && clientType != "public") {
		return errors.New("客户端类型必须是 confidential 或 public")
	}

	return nil
}

// PrepareConfig 准备OAuth2配置（生成ClientID和ClientSecret）
// 注意：此方法返回的配置包含明文的ClientSecret，仅用于创建时返回给前端
func (h *OAuth2ProtocolHandler) PrepareConfig(uuid string, config map[string]interface{}) (map[string]interface{}, error) {
	// 使用传入的UUID作为ClientID
	config["clientId"] = uuid

	// 生成ClientSecret（明文）
	clientSecret := h.generateClientSecret()
	config["clientSecret"] = clientSecret

	// 设置默认的Token有效期（如果未设置）
	if _, ok := config["tokenLifetime"]; !ok {
		config["tokenLifetime"] = 3600
	}

	return config, nil
}

// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
func (h *OAuth2ProtocolHandler) EncryptSensitiveFields(config map[string]interface{}) error {
	// 对ClientSecret进行哈希处理
	if clientSecret, ok := config["clientSecret"].(string); ok && clientSecret != "" {
		config["clientSecret"] = h.hashClientSecret(clientSecret)
	}

	return nil
}

// CreateApplication 创建OAuth2应用
func (h *OAuth2ProtocolHandler) CreateApplication(app *model.Application) error {
	// OAuth2协议在创建时不需要额外处理
	return nil
}

// UpdateApplication 更新OAuth2应用
func (h *OAuth2ProtocolHandler) UpdateApplication(app *model.Application, updates map[string]interface{}) error {
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

// DeleteApplication 删除OAuth2应用
func (h *OAuth2ProtocolHandler) DeleteApplication(id uint) error {
	// TODO: 在删除应用前，需要撤销所有相关的令牌和授权码
	return nil
}

// GetProtocolDetails 获取OAuth2协议详细信息
func (h *OAuth2ProtocolHandler) GetProtocolDetails(app *model.Application) (interface{}, error) {
	config := app.ProtocolConfig

	// 构造OAuth2配置响应（不包含ClientSecret）
	details := map[string]interface{}{
		"clientId":      config["clientId"],
		"redirectUris":  config["redirectUris"],
		"grantTypes":    config["grantTypes"],
		"clientType":    config["clientType"],
		"tokenLifetime": config["tokenLifetime"],
	}

	return details, nil
}

// ValidateClient 验证OAuth2客户端凭据
func (h *OAuth2ProtocolHandler) ValidateClient(app *model.Application, clientSecret string) error {
	if app.ProtocolType != string(model.ProtocolTypeOAuth2) {
		return errors.New("不是OAuth2应用")
	}

	if !app.IsActive {
		return errors.New("应用已被禁用")
	}

	config := app.ProtocolConfig

	// 获取客户端类型
	clientType, ok := config["clientType"].(string)
	if !ok {
		return errors.New("客户端类型配置错误")
	}

	// 对于公开客户端，不需要验证客户端密钥
	if clientType == "public" {
		return nil
	}

	// 对于机密客户端，需要验证客户端密钥
	if clientSecret == "" {
		return errors.New("机密客户端需要提供客户端密钥")
	}

	storedSecret, ok := config["clientSecret"].(string)
	if !ok {
		return errors.New("客户端密钥配置错误")
	}

	if !h.verifyClientSecret(clientSecret, storedSecret) {
		return errors.New("客户端密钥错误")
	}

	return nil
}

// 私有方法

// generateClientSecret 生成客户端密钥
func (h *OAuth2ProtocolHandler) generateClientSecret() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

// hashClientSecret 哈希客户端密钥
func (h *OAuth2ProtocolHandler) hashClientSecret(secret string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		// bcrypt 失败的概率极低，这里简单处理
		panic(fmt.Sprintf("bcrypt hash failed: %v", err))
	}
	return string(hash)
}

// verifyClientSecret 验证客户端密钥
func (h *OAuth2ProtocolHandler) verifyClientSecret(secret, hashedSecret string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedSecret), []byte(secret))
	return err == nil
}

// validateRedirectURIFormat 验证重定向URI格式
func (h *OAuth2ProtocolHandler) validateRedirectURIFormat(uri string) error {
	if uri == "" {
		return errors.New("URI不能为空")
	}

	// 简单的URL格式验证
	if len(uri) < 7 {
		return errors.New("URI格式无效")
	}

	// 检查是否包含scheme
	hasValidScheme := false
	validSchemes := []string{"http://", "https://"}
	for _, scheme := range validSchemes {
		if len(uri) >= len(scheme) && uri[:len(scheme)] == scheme {
			hasValidScheme = true
			break
		}
	}

	// 也支持自定义scheme格式 (例如: myapp://)
	if !hasValidScheme {
		// 检查是否匹配自定义scheme格式 scheme://
		schemeEndIdx := -1
		for i := 0; i < len(uri) && i < 50; i++ { // 限制scheme长度
			if i+3 < len(uri) && uri[i:i+3] == "://" {
				schemeEndIdx = i
				break
			}
		}
		if schemeEndIdx > 0 {
			hasValidScheme = true
		}
	}

	if !hasValidScheme {
		return errors.New("URI必须包含有效的scheme (http://, https://, 或自定义scheme://)")
	}

	return nil
}
