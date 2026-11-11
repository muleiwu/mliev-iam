package captcha

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

const captchaSessionTTLSeconds = 300

// encryptTencentAppID follows Tencent's aidEncrypted CBC wire format. The
// reader and clock are explicit so the official vector and entropy failures
// can be tested without replacing process-wide cryptographic randomness.
func encryptTencentAppID(appID, secret string, now time.Time, expiresIn int, random io.Reader) (string, error) {
	if expiresIn < 1 || expiresIn > 86400 {
		return "", errors.New("验证码加密有效期无效")
	}
	id, err := strconv.ParseUint(appID, 10, 64)
	if err != nil || id == 0 {
		return "", errors.New("验证码 AppId 无效")
	}
	if len(secret) == 0 || len(secret) > 32 {
		return "", errors.New("验证码加密密钥长度必须为1至32字节")
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = secret[i%len(secret)]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("验证码加密配置无效")
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(random, iv); err != nil {
		return "", errors.New("生成验证码加密参数失败，请重试")
	}
	plain := []byte(fmt.Sprintf("%s&%d&%d", appID, now.Unix(), expiresIn))
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < padding; i++ {
		plain = append(plain, byte(padding))
	}
	encrypted := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, plain)
	return base64.StdEncoding.EncodeToString(append(iv, encrypted...)), nil
}

func encryptedTencentClientParams(appID, secret string, now time.Time) (map[string]interface{}, error) {
	encrypted, err := encryptTencentAppID(appID, secret, now, captchaSessionTTLSeconds, rand.Reader)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"captchaAppId":     appID,
		"aidEncrypted":     encrypted,
		"aidEncryptedType": "cbc",
	}, nil
}
