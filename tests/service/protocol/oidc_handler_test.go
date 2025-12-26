package protocol

import (
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOIDCProtocolHandler_PrepareConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	handler := protocol.NewOIDCProtocolHandler(db)

	config := map[string]any{
		"redirectUris":  []any{"http://localhost:3000/callback"},
		"grantTypes":    []any{"authorization_code"},
		"responseTypes": []any{"code"},
		"scopes":        []any{"openid", "profile"},
		"clientType":    "confidential",
	}

	testUUID := "test-uuid-12345"
	preparedConfig, err := handler.PrepareConfig(testUUID, config)
	assert.NoError(t, err)

	// 验证生成了 clientId
	assert.NotEmpty(t, preparedConfig["clientId"])

	// 验证生成了明文的 clientSecret
	clientSecret, ok := preparedConfig["clientSecret"].(string)
	assert.True(t, ok)
	assert.NotEmpty(t, clientSecret)

	// 验证明文 clientSecret 不是哈希值（哈希值是64个字符的十六进制）
	// 而生成的明文应该是 base64 编码的，长度不同
	assert.NotEqual(t, 64, len(clientSecret))
}

func TestOIDCProtocolHandler_EncryptSensitiveFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	handler := protocol.NewOIDCProtocolHandler(db)

	plainSecret := "test-plain-secret"
	config := map[string]any{
		"clientId":     "test-client-id",
		"clientSecret": plainSecret,
	}

	// 加密前保存明文
	originalSecret := config["clientSecret"].(string)

	// 执行加密
	err = handler.EncryptSensitiveFields(config)
	assert.NoError(t, err)

	// 验证 clientSecret 已被哈希
	hashedSecret, ok := config["clientSecret"].(string)
	assert.True(t, ok)
	assert.NotEmpty(t, hashedSecret)

	// 验证哈希后的值与原始值不同
	assert.NotEqual(t, originalSecret, hashedSecret)

	// 验证哈希值是 bcrypt 格式 (以 $2a$ 或 $2b$ 开头，长度约60字符)
	assert.True(t, len(hashedSecret) > 50 && len(hashedSecret) < 80)
	assert.True(t, strings.HasPrefix(hashedSecret, "$2a$") || strings.HasPrefix(hashedSecret, "$2b$"))
}

func TestOIDCProtocolHandler_EncryptSensitiveFields_Idempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	handler := protocol.NewOIDCProtocolHandler(db)

	config := map[string]any{
		"clientId":     "test-client-id",
		"clientSecret": "test-plain-secret",
	}

	// 第一次加密
	err = handler.EncryptSensitiveFields(config)
	assert.NoError(t, err)
	firstHash := config["clientSecret"].(string)

	// 第二次加密（加密已经哈希的值）
	err = handler.EncryptSensitiveFields(config)
	assert.NoError(t, err)
	secondHash := config["clientSecret"].(string)

	// 注意：bcrypt 每次生成不同哈希（因为盐是随机的）
	// 这个测试是为了验证即使对哈希值再次哈希也不会出错
	assert.NotEmpty(t, secondHash)
	assert.NotEqual(t, firstHash, secondHash)
}

func TestOIDCProtocolHandler_PrepareAndEncrypt_Integration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	handler := protocol.NewOIDCProtocolHandler(db)

	config := map[string]any{
		"redirectUris":  []any{"http://localhost:3000/callback"},
		"grantTypes":    []any{"authorization_code"},
		"responseTypes": []any{"code"},
		"scopes":        []any{"openid", "profile"},
		"clientType":    "confidential",
	}

	// 1. 准备配置（生成明文凭据）
	testUUID := "test-uuid-67890"
	preparedConfig, err := handler.PrepareConfig(testUUID, config)
	assert.NoError(t, err)

	// 2. 保存明文副本（模拟创建应用流程）
	plainSecret := preparedConfig["clientSecret"].(string)
	assert.NotEmpty(t, plainSecret)

	// 3. 加密敏感字段
	err = handler.EncryptSensitiveFields(preparedConfig)
	assert.NoError(t, err)

	// 4. 验证配置中的 clientSecret 已被哈希
	hashedSecret := preparedConfig["clientSecret"].(string)
	assert.NotEqual(t, plainSecret, hashedSecret)
	// 验证哈希值是 bcrypt 格式 (以 $2a$ 或 $2b$ 开头，长度约60字符)
	assert.True(t, len(hashedSecret) > 50 && len(hashedSecret) < 80)
	assert.True(t, strings.HasPrefix(hashedSecret, "$2a$") || strings.HasPrefix(hashedSecret, "$2b$"))

	// 5. 验证可以通过验证函数验证明文密钥
	isValid := handler.VerifyClientSecret(plainSecret, hashedSecret)
	assert.True(t, isValid)
}
