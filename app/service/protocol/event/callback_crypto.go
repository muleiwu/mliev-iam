package event

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

func decodeCallbackAESKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if len(encoded) != 43 {
		return nil, fmt.Errorf("EncodingAESKey 必须为 43 个字符")
	}
	key, err := base64.StdEncoding.DecodeString(encoded + "=")
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("EncodingAESKey 不是有效的 32 字节 Base64 密钥")
	}
	return key, nil
}

func callbackSHA1Signature(values ...string) string {
	sort.Strings(values)
	digest := sha1.Sum([]byte(strings.Join(values, "")))
	return fmt.Sprintf("%x", digest[:])
}

func verifyCallbackSignature(actual string, values ...string) error {
	expected := callbackSHA1Signature(values...)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(actual))), []byte(expected)) != 1 {
		return fmt.Errorf("事件回调签名无效")
	}
	return nil
}

func decryptCallbackMessage(encoded string, key []byte, receiverID string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("事件密文不是有效 Base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("事件密文长度非法")
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, ciphertext)
	plain, err = callbackPKCS7Unpad(plain)
	if err != nil {
		return nil, err
	}
	if len(plain) < 20 {
		return nil, fmt.Errorf("事件明文长度非法")
	}
	messageLength := int(binary.BigEndian.Uint32(plain[16:20]))
	if messageLength < 0 || 20+messageLength > len(plain) {
		return nil, fmt.Errorf("事件明文消息长度非法")
	}
	message := plain[20 : 20+messageLength]
	receiver := string(plain[20+messageLength:])
	if receiverID != "" && subtle.ConstantTimeCompare([]byte(receiver), []byte(receiverID)) != 1 {
		return nil, fmt.Errorf("事件回调接收方标识不匹配")
	}
	return message, nil
}

func encryptCallbackMessage(message []byte, key []byte, receiverID string) (string, error) {
	randomPrefix := make([]byte, 16)
	if _, err := rand.Read(randomPrefix); err != nil {
		return "", err
	}
	plain := make([]byte, 0, 20+len(message)+len(receiverID)+32)
	plain = append(plain, randomPrefix...)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(message)))
	plain = append(plain, length...)
	plain = append(plain, message...)
	plain = append(plain, receiverID...)
	plain = callbackPKCS7Pad(plain)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(ciphertext, plain)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// WeCom and legacy DingTalk callback envelopes use a 32-byte PKCS#7 block.
func callbackPKCS7Pad(value []byte) []byte {
	padding := 32 - len(value)%32
	if padding == 0 {
		padding = 32
	}
	return append(value, bytesRepeat(byte(padding), padding)...)
}

func callbackPKCS7Unpad(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("事件明文为空")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > 32 || padding > len(value) {
		return nil, fmt.Errorf("事件 PKCS7 padding 非法")
	}
	for _, item := range value[len(value)-padding:] {
		if int(item) != padding {
			return nil, fmt.Errorf("事件 PKCS7 padding 非法")
		}
	}
	return value[:len(value)-padding], nil
}

func bytesRepeat(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}
