package captcha

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestTencentEncryptionOfficialCBCVector(t *testing.T) {
	// Tencent's published CBC example uses a 24-hour TTL; production uses 300s.
	encrypted, err := encryptTencentAppID("123456789", "1234567891011121314151516", time.Unix(1710144972, 0), 86400, strings.NewReader("0123456789012345"))
	require.NoError(t, err)
	require.Equal(t, "MDEyMzQ1Njc4OTAxMjM0NWvZ11atw+1uzYmoIyt5rAQVPyMK9ZDavskPw5hcayeT", encrypted)
}

func decryptTencentParams(t *testing.T, encrypted, secret string) (string, []byte) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(raw), 32)
	require.Zero(t, (len(raw)-16)%16)
	key := []byte(strings.Repeat(secret, 32))[:32]
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	plain := make([]byte, len(raw)-16)
	cipher.NewCBCDecrypter(block, raw[:16]).CryptBlocks(plain, raw[16:])
	padding := int(plain[len(plain)-1])
	require.True(t, padding >= 1 && padding <= 16)
	require.Equal(t, bytes.Repeat([]byte{byte(padding)}, padding), plain[len(plain)-padding:])
	return string(plain[:len(plain)-padding]), raw[:16]
}

func TestTencentClientParamsUseFreshIVAndPrivateKey(t *testing.T) {
	now := time.Unix(1792700000, 0)
	for _, secret := range []string{"a", "private-app-secret", strings.Repeat("x", 32)} {
		first, err := encryptedTencentClientParams("123456", secret, now)
		require.NoError(t, err)
		second, err := encryptedTencentClientParams("123456", secret, now)
		require.NoError(t, err)
		plain, iv := decryptTencentParams(t, first["aidEncrypted"].(string), secret)
		_, nextIV := decryptTencentParams(t, second["aidEncrypted"].(string), secret)
		require.Equal(t, "123456&1792700000&300", plain)
		require.NotEqual(t, iv, nextIV)
		require.Equal(t, "cbc", first["aidEncryptedType"])
		require.Len(t, first, 3)
	}
}

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) { return 0, errors.New("private entropy details") }

func TestTencentEncryptionRejectsInvalidInputsAndEntropyFailure(t *testing.T) {
	for _, secret := range []string{"", strings.Repeat("x", 33)} {
		encrypted, err := encryptTencentAppID("123456", secret, time.Now(), 300, rand.Reader)
		require.Error(t, err)
		require.Empty(t, encrypted)
	}
	for _, id := range []string{"", "0", "abc", "123&456"} {
		_, err := encryptTencentAppID(id, "secret", time.Now(), 300, rand.Reader)
		require.Error(t, err)
	}
	for _, entropy := range []interface{ Read([]byte) (int, error) }{failingEntropy{}, strings.NewReader("too short")} {
		encrypted, err := encryptTencentAppID("123456", "secret", time.Now(), 300, entropy)
		require.ErrorContains(t, err, "生成验证码加密参数失败")
		require.NotContains(t, err.Error(), "private")
		require.Empty(t, encrypted)
	}
}

func TestTencentWebEncryptionIsEnabledByDefault(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{Name: "web", Provider: TencentProviderType, Enabled: true,
		Params: datatypes.JSONMap{"secretId": "private-id", "secretKey": "private-cloud-key", "appSecretKey": "private-app-key", "captchaAppId": "123456"}}
	require.NoError(t, db.Create(config).Error)
	createCaptchaSceneBinding(t, db, config.ID)
	manager := NewManager(db)
	first, err := manager.GetClientParamsForScene("login")
	require.NoError(t, err)
	second, err := manager.GetClientParamsForScene("login")
	require.NoError(t, err)
	require.True(t, first.Enabled)
	require.Equal(t, 300, first.ExpiresIn)
	require.Equal(t, "cbc", first.Params["aidEncryptedType"])
	require.NotEqual(t, first.Params["aidEncrypted"], second.Params["aidEncrypted"])
	encoded, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-")
	require.NotContains(t, string(encoded), "Secret")
	config.Params["appSecretKey"] = strings.Repeat("x", 33)
	require.NoError(t, db.Save(config).Error)
	result, err := manager.GetClientParamsForScene("login")
	require.ErrorContains(t, err, "生成验证码参数失败")
	require.Nil(t, result)
}

func TestMiniCaptchaEncryptionRetainsOptIn(t *testing.T) {
	config := &model.CaptchaConfig{Params: datatypes.JSONMap{"secretId": "private-id", "secretKey": "private-cloud-key", "appSecretKey": "private-app-key", "captchaAppId": "123456"}}
	p := &TencentMiniProgramProvider{}
	for _, enabled := range []any{nil, false, "false", true, "true"} {
		config.Params["aidEncryptionEnabled"] = enabled
		params, err := p.GetClientParams(config)
		require.NoError(t, err)
		if enabled == true || enabled == "true" {
			require.NotEmpty(t, params["aidEncrypted"])
		} else {
			require.NotContains(t, params, "aidEncrypted")
		}
	}
}
