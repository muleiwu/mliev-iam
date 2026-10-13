package protocol

import (
	"cnb.cool/mliev/iam/mliev-iam/pkg/firstparty"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// OIDCProtocolHandler OIDC协议处理器
type OIDCProtocolHandler struct {
	db *gorm.DB
}

// NewOIDCProtocolHandler 创建OIDC协议处理器
func NewOIDCProtocolHandler(db *gorm.DB) *OIDCProtocolHandler {
	return &OIDCProtocolHandler{
		db: db,
	}
}

// ValidateConfig 验证OIDC配置
func (h *OIDCProtocolHandler) ValidateConfig(config map[string]interface{}) error {
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

	// 验证响应类型
	responseTypes, ok := config["responseTypes"].([]interface{})
	if !ok || len(responseTypes) == 0 {
		return errors.New("至少需要一个响应类型")
	}

	// 验证作用域
	scopes, ok := config["scopes"].([]interface{})
	if !ok || len(scopes) == 0 {
		return errors.New("至少需要一个作用域")
	}

	// 验证作用域必须包含openid
	hasOpenID := false
	for _, scope := range scopes {
		scopeStr, ok := scope.(string)
		if ok && scopeStr == "openid" {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		return errors.New("OIDC应用的作用域必须包含openid")
	}

	// 验证客户端类型
	clientType, ok := config["clientType"].(string)
	if !ok || (clientType != "confidential" && clientType != "public") {
		return errors.New("客户端类型必须是 confidential 或 public")
	}

	return h.validateAuthorizationConfig(config)
}

// PrepareConfig 准备OIDC配置（生成ClientID；机密客户端额外生成ClientSecret）
// 注意：机密客户端会返回明文ClientSecret，仅用于创建时返回给前端。
func (h *OIDCProtocolHandler) PrepareConfig(uuid string, config map[string]interface{}) (map[string]interface{}, error) {
	// 使用传入的UUID作为ClientID
	config["clientId"] = uuid

	clientType, _ := config["clientType"].(string)
	if clientType == "confidential" {
		// 生成ClientSecret（明文）
		clientSecret := h.generateClientSecret()
		config["clientSecret"] = clientSecret
	} else {
		// 公开客户端不能安全保存密钥，强制走 PKCE。
		delete(config, "clientSecret")
		config["requirePkce"] = true
	}

	// 设置默认的Token有效期（如果未设置）
	if _, ok := config["tokenLifetime"]; !ok {
		config["tokenLifetime"] = 3600
	}

	return config, nil
}

// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
// 此方法会直接修改传入的配置对象
func (h *OIDCProtocolHandler) EncryptSensitiveFields(config map[string]interface{}) error {
	if clientType, _ := config["clientType"].(string); clientType == "public" {
		delete(config, "clientSecret")
		return nil
	}

	// 对ClientSecret进行哈希处理
	if clientSecret, ok := config["clientSecret"].(string); ok && clientSecret != "" {
		config["clientSecret"] = h.hashClientSecret(clientSecret)
	}

	return nil
}

// CreateApplication 创建OIDC应用
func (h *OIDCProtocolHandler) CreateApplication(app *model.Application) error {
	// 1. 从配置中获取签名算法（可选）
	algorithm := "RS256" // 默认RS256
	if alg, ok := app.ProtocolConfig["signingAlgorithm"].(string); ok && alg != "" {
		algorithm = alg
	}

	// 2. 获取密钥大小
	keySize := h.getDefaultKeySize(algorithm)

	// 3. 为应用生成签名密钥（使用当前 db 连接，它可能已经在事务中）
	if err := h.generateKeyForApplicationWithDB(h.db, app.ID, algorithm, keySize); err != nil {
		return fmt.Errorf("生成签名密钥失败: %w", err)
	}

	return nil
}

// UpdateApplication 更新OIDC应用
func (h *OIDCProtocolHandler) UpdateApplication(app *model.Application, updates map[string]interface{}) error {
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

		if clientType, _ := currentConfig["clientType"].(string); clientType == "public" {
			// 从机密客户端切换为公开客户端时，清理历史密钥并强制 PKCE。
			delete(currentConfig, "clientSecret")
			currentConfig["requirePkce"] = true
		}

		app.ProtocolConfig = currentConfig
	}

	return nil
}

// DeleteApplication 删除OIDC应用
func (h *OIDCProtocolHandler) DeleteApplication(id uint) error {
	// 删除应用的所有密钥
	keyDAO := dao.NewApplicationOIDCKeyDAO(h.db)
	if err := keyDAO.DeleteByApplicationID(id); err != nil {
		return fmt.Errorf("删除应用密钥失败: %w", err)
	}

	// TODO: 在删除应用前，需要撤销所有相关的令牌和授权码
	// 这里可以添加清理逻辑
	return nil
}

// GetProtocolDetails 获取OIDC协议详细信息
func (h *OIDCProtocolHandler) GetProtocolDetails(app *model.Application) (interface{}, error) {
	config := app.ProtocolConfig

	// 构造OIDC配置响应（不包含ClientSecret）
	details := map[string]interface{}{
		"clientId":      config["clientId"],
		"redirectUris":  config["redirectUris"],
		"grantTypes":    config["grantTypes"],
		"responseTypes": config["responseTypes"],
		"scopes":        config["scopes"],
		"clientType":    config["clientType"],
		"requirePkce":   config["requirePkce"],
		"tokenLifetime": config["tokenLifetime"],
	}

	return details, nil
}

// ValidateClient 验证OIDC客户端凭据
func (h *OIDCProtocolHandler) ValidateClient(app *model.Application, clientSecret string) error {
	if app.ProtocolType != string(model.ProtocolTypeOIDC) {
		return errors.New("不是OIDC应用")
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
		if clientSecret != "" {
			return errors.New("公开客户端不能使用客户端密钥")
		}
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

	if !h.VerifyClientSecret(clientSecret, storedSecret) {
		return errors.New("客户端密钥错误")
	}

	return nil
}

// ValidateRedirectURI 验证重定向 URI（RFC 6749 §3.1.2 + OIDC Core §3.1.2.1）
//   - 必须是注册白名单内的 URI
//   - 禁止 fragment（#...）
//   - 比较方式为规范化后精确匹配：scheme + host + path + query（query 允许客户端附加则在注册时预先声明）
//   - RFC 8252 §7.3：http + loopback IP 字面量（127.0.0.1 / ::1）忽略端口比较，
//     以支持 native app 监听随机本地端口；localhost 域名不放宽
func (h *OIDCProtocolHandler) ValidateRedirectURI(app *model.Application, redirectURI string, trustedIssuer ...string) error {
	config := app.ProtocolConfig
	redirectUris, ok := config["redirectUris"].([]interface{})
	if !ok {
		return errors.New("重定向URI配置错误")
	}

	incoming, err := parseAndValidateRedirectURI(redirectURI)
	if err != nil {
		return err
	}
	if client, builtin := firstparty.Lookup(app.UUID); builtin && app.IsProtected {
		if incoming.Path != client.CallbackPath || incoming.RawQuery != "" {
			return errors.New("内置管理后台重定向URI无效")
		}
		if len(trustedIssuer) > 0 && trustedIssuer[0] != "" {
			issuer, err := url.Parse(trustedIssuer[0])
			if err != nil || issuer.Scheme == "" || issuer.Host == "" {
				return errors.New("可信颁发者地址无效")
			}
			if !strings.EqualFold(incoming.Scheme, issuer.Scheme) || !strings.EqualFold(incoming.Host, issuer.Host) {
				return errors.New("内置管理后台重定向URI必须与认证服务同源")
			}
		}
		return nil
	}
	incomingNorm := normalizeRedirectURI(incoming)

	for _, uri := range redirectUris {
		uriStr, ok := uri.(string)
		if !ok || uriStr == "" {
			continue
		}
		registered, err := url.Parse(uriStr)
		if err != nil {
			continue
		}
		if normalizeRedirectURI(registered) == incomingNorm {
			return nil
		}
		if matchLoopbackRedirectURI(registered, incoming) {
			return nil
		}
	}
	return errors.New("无效的重定向URI")
}

// matchLoopbackRedirectURI RFC 8252 §7.3：注册 URI 与传入 URI 均为 http + 同一
// loopback IP 字面量时忽略端口，其余部分（path、query）仍按规范化精确匹配
func matchLoopbackRedirectURI(registered, incoming *url.URL) bool {
	if strings.ToLower(registered.Scheme) != "http" || strings.ToLower(incoming.Scheme) != "http" {
		return false
	}
	regHost := strings.ToLower(registered.Hostname())
	if !isLoopbackIP(regHost) || regHost != strings.ToLower(incoming.Hostname()) {
		return false
	}
	return normalizeRedirectURIWithoutPort(registered) == normalizeRedirectURIWithoutPort(incoming)
}

// isLoopbackIP 仅认可 RFC 8252 §7.3 的 loopback IP 字面量，不包含 localhost 域名
func isLoopbackIP(hostname string) bool {
	return hostname == "127.0.0.1" || hostname == "::1"
}

// parseAndValidateRedirectURI 解析并检查 URI 的结构层面合规性
func parseAndValidateRedirectURI(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("重定向URI不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("重定向URI解析失败: %w", err)
	}
	if u.Scheme == "" {
		return nil, errors.New("重定向URI必须包含 scheme")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, errors.New("重定向URI禁止包含 fragment")
	}
	return u, nil
}

// normalizeRedirectURI 规范化用于比较：大小写化 scheme/host、去尾部多余斜杠
func normalizeRedirectURI(u *url.URL) string {
	return normalizeRedirectURIHost(u, strings.ToLower(u.Host))
}

// normalizeRedirectURIWithoutPort 同 normalizeRedirectURI，但 host 只取主机名（用于 loopback 忽略端口比较）
func normalizeRedirectURIWithoutPort(u *url.URL) string {
	return normalizeRedirectURIHost(u, strings.ToLower(u.Hostname()))
}

func normalizeRedirectURIHost(u *url.URL, host string) string {
	scheme := strings.ToLower(u.Scheme)
	path := u.EscapedPath()
	if path == "" && (scheme == "http" || scheme == "https") {
		path = "/"
	}
	// query 参与比较以保证严格匹配
	canonical := scheme + "://" + host + path
	if u.RawQuery != "" {
		canonical += "?" + u.RawQuery
	}
	return canonical
}

// ValidateGrantType 验证授权类型
func (h *OIDCProtocolHandler) ValidateGrantType(app *model.Application, grantType string) error {
	config := app.ProtocolConfig
	grantTypes, ok := config["grantTypes"].([]interface{})
	if !ok {
		return errors.New("授权类型配置错误")
	}

	for _, gt := range grantTypes {
		gtStr, ok := gt.(string)
		if ok && gtStr == grantType {
			return nil
		}
	}

	return fmt.Errorf("应用不支持授权类型: %s", grantType)
}

// ValidateResponseType 验证响应类型
func (h *OIDCProtocolHandler) ValidateResponseType(app *model.Application, responseType string) error {
	config := app.ProtocolConfig
	responseTypes, ok := config["responseTypes"].([]interface{})
	if !ok {
		return errors.New("响应类型配置错误")
	}

	for _, rt := range responseTypes {
		rtStr, ok := rt.(string)
		if ok && rtStr == responseType {
			return nil
		}
	}

	return fmt.Errorf("应用不支持响应类型: %s", responseType)
}

// ValidateScopes 验证作用域
func (h *OIDCProtocolHandler) ValidateScopes(app *model.Application, requestedScopes []string) error {
	c, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
	if err != nil {
		return err
	}
	scopes, err := model.NormalizeOIDCScopes(requestedScopes)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, scope := range c.Scopes {
		allowed[scope] = true
	}
	for _, scope := range scopes {
		if !allowed[scope] {
			return fmt.Errorf("应用不支持作用域: %s", scope)
		}
	}
	return nil
}

// IsPublicClient 检查是否为公开客户端
func (h *OIDCProtocolHandler) IsPublicClient(app *model.Application) bool {
	config := app.ProtocolConfig
	clientType, ok := config["clientType"].(string)
	return ok && clientType == "public"
}

// RequiresPKCE 检查是否需要PKCE
func (h *OIDCProtocolHandler) RequiresPKCE(app *model.Application) bool {
	config := app.ProtocolConfig
	requirePkce, ok := config["requirePkce"].(bool)
	return ok && requirePkce
}

// 私有方法

// generateClientID 生成客户端ID
func (h *OIDCProtocolHandler) generateClientID() string {
	return uuid.New().String()
}

// generateClientSecret 生成客户端密钥
func (h *OIDCProtocolHandler) generateClientSecret() string {
	v7, err := uuid.NewV7()
	if err != nil {
		v7 = uuid.New()
	}
	return v7.String()
}

// hashClientSecret 哈希客户端密钥
func (h *OIDCProtocolHandler) hashClientSecret(secret string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		// bcrypt 失败的概率极低，这里简单处理
		panic(fmt.Sprintf("bcrypt hash failed: %v", err))
	}
	return string(hash)
}

// VerifyClientSecret 验证客户端密钥
func (h *OIDCProtocolHandler) VerifyClientSecret(secret, hashedSecret string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedSecret), []byte(secret))
	return err == nil
}

// validateRedirectURIFormat 验证重定向URI格式（注册时）
// 同时禁止 fragment，避免将非法 URI 写入白名单
func (h *OIDCProtocolHandler) validateRedirectURIFormat(uri string) error {
	_, err := parseAndValidateRedirectURI(uri)
	return err
}

// generateKeyForApplication 为应用生成密钥（使用新事务）
func (h *OIDCProtocolHandler) generateKeyForApplication(appID uint, algorithm string, keySize int) error {
	return h.generateKeyForApplicationWithDB(h.db, appID, algorithm, keySize)
}

// generateKeyForApplicationWithDB 为应用生成密钥（可传入事务）
func (h *OIDCProtocolHandler) generateKeyForApplicationWithDB(db *gorm.DB, appID uint, algorithm string, keySize int) error {
	// 1. 创建密钥工厂
	factory := jwt.NewKeyManagerFactory()

	// 2. 验证算法是否支持
	if !factory.IsAlgorithmSupported(algorithm) {
		return fmt.Errorf("不支持的算法: %s", algorithm)
	}

	// 3. 生成密钥ID
	keyID, err := uuid.NewV7()
	if err != nil {
		keyID = uuid.New() // 回退到v4
	}
	keyIDStr := keyID.String()

	// 4. 创建密钥管理器
	keyManager, err := factory.CreateKeyManager(algorithm, keyIDStr, keySize)
	if err != nil {
		return fmt.Errorf("创建密钥管理器失败: %w", err)
	}

	// 5. 生成密钥
	if err := keyManager.GenerateKey(); err != nil {
		return fmt.Errorf("生成密钥失败: %w", err)
	}

	// 6. 导出密钥
	privateKey, err := keyManager.ExportPrivateKey()
	if err != nil {
		return fmt.Errorf("导出私钥失败: %w", err)
	}

	publicKey, _ := keyManager.ExportPublicKey() // 对称加密可能返回空

	// 7. 创建密钥记录
	key := &model.ApplicationOIDCKey{
		ApplicationID: appID,
		KeyID:         keyIDStr,
		Algorithm:     keyManager.GetAlgorithm(),
		KeyType:       string(keyManager.GetKeyType()),
		PrivateKey:    privateKey, // TODO: 加密存储
		PublicKey:     publicKey,
		KeySize:       keyManager.GetKeySize(),
		IsPrimary:     true, // 默认设为主密钥
	}

	// 8. 取消该应用下所有密钥的主密钥状态
	if err := db.Model(&model.ApplicationOIDCKey{}).
		Where("application_id = ?", appID).
		Update("is_primary", false).Error; err != nil {
		return fmt.Errorf("更新主密钥状态失败: %w", err)
	}

	// 9. 保存新密钥
	if err := db.Create(key).Error; err != nil {
		return fmt.Errorf("保存密钥失败: %w", err)
	}

	return nil
}

// getDefaultKeySize 获取默认密钥大小
func (h *OIDCProtocolHandler) getDefaultKeySize(algorithm string) int {
	switch algorithm {
	case "RS256", "RS384":
		return 2048
	case "RS512":
		return 4096
	case "HS256":
		return 256
	case "HS384":
		return 384
	case "HS512":
		return 512
	default:
		return 2048
	}
}

// ResetClientSecret 重置客户端密钥
func (h *OIDCProtocolHandler) ResetClientSecret(appID uint) (string, error) {
	appDAO := dao.NewApplicationDAO(h.db)
	app, err := appDAO.GetByID(appID)
	if err != nil {
		return "", err
	}
	if clientType, _ := app.ProtocolConfig["clientType"].(string); clientType == "public" {
		return "", errors.New("公开客户端没有客户端密钥")
	}

	// 生成新的客户端密钥（明文）
	newClientSecret := h.generateClientSecret()

	// 加密密钥
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(newClientSecret), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("加密客户端密钥失败: %w", err)
	}

	// 更新数据库
	err = appDAO.UpdateClientSecret(appID, string(hashedSecret))
	if err != nil {
		return "", fmt.Errorf("更新客户端密钥失败: %w", err)
	}

	return newClientSecret, nil
}
