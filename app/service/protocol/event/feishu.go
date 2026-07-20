package event

import (
	"context"
	"crypto/aes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type FeishuAdapter struct {
	verificationToken string
	encryptKey        string
	maxClockSkew      time.Duration
}

func NewFeishuAdapter(credentials map[string]any) (*FeishuAdapter, error) {
	value := func(key string) string {
		text, _ := credentials[key].(string)
		return strings.TrimSpace(text)
	}
	adapter := &FeishuAdapter{
		verificationToken: value("verificationToken"), encryptKey: value("encryptKey"),
		maxClockSkew: 5 * time.Minute,
	}
	if adapter.verificationToken == "" {
		return nil, fmt.Errorf("飞书事件凭据必须包含 verificationToken")
	}
	return adapter, nil
}

func (a *FeishuAdapter) VerifyAndParse(_ context.Context, headers http.Header, body []byte) (*DirectoryEvent, error) {
	if len(body) == 0 || len(body) > 4<<20 {
		return nil, fmt.Errorf("飞书事件请求体为空或过大")
	}
	if a.encryptKey != "" {
		if err := a.verifySignature(headers, body); err != nil {
			return nil, err
		}
	}
	payloadBytes := body
	var encryptedEnvelope struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &encryptedEnvelope); err != nil {
		return nil, fmt.Errorf("解析飞书事件外层失败: %w", err)
	}
	if encryptedEnvelope.Encrypt != "" {
		if a.encryptKey == "" {
			return nil, fmt.Errorf("收到加密事件但未配置 encryptKey")
		}
		plain, err := decryptFeishuPayload(encryptedEnvelope.Encrypt, a.encryptKey)
		if err != nil {
			return nil, err
		}
		payloadBytes = plain
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("解析飞书事件失败: %w", err)
	}
	header, _ := payload["header"].(map[string]any)
	token := firstString(payload, "token")
	if token == "" {
		token = firstString(header, "token")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(a.verificationToken)) != 1 {
		return nil, fmt.Errorf("飞书事件 verification token 无效")
	}
	challenge := firstString(payload, "challenge")
	if challenge != "" {
		return &DirectoryEvent{Challenge: challenge, EventType: "url_verification", Payload: payload}, nil
	}
	eventID := firstString(header, "event_id")
	if eventID == "" {
		eventID = firstString(payload, "uuid")
	}
	eventType := firstString(header, "event_type")
	if eventType == "" {
		eventType = firstString(payload, "type")
	}
	if eventID == "" || eventType == "" {
		return nil, fmt.Errorf("飞书事件缺少 event_id 或 event_type")
	}
	eventPayload, _ := payload["event"].(map[string]any)
	objectType, objectID := feishuEventObject(eventType, eventPayload)
	return &DirectoryEvent{
		ProviderEventID: eventID, EventType: eventType, ObjectType: objectType,
		ExternalObjectID: objectID, Payload: payload,
	}, nil
}

func (a *FeishuAdapter) verifySignature(headers http.Header, body []byte) error {
	timestamp := headers.Get("X-Lark-Request-Timestamp")
	nonce := headers.Get("X-Lark-Request-Nonce")
	signature := headers.Get("X-Lark-Signature")
	if signature == "" {
		signature = headers.Get("X-Lark-Request-Signature")
	}
	if timestamp == "" || nonce == "" || signature == "" {
		return fmt.Errorf("飞书事件签名头不完整")
	}
	var unix int64
	if _, err := fmt.Sscan(timestamp, &unix); err != nil {
		return fmt.Errorf("飞书事件时间戳非法")
	}
	requestTime := time.Unix(unix, 0)
	if delta := time.Since(requestTime); delta > a.maxClockSkew || delta < -a.maxClockSkew {
		return fmt.Errorf("飞书事件时间戳超出允许偏差")
	}
	digest := sha256.Sum256([]byte(timestamp + nonce + a.encryptKey + string(body)))
	expected := hex.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return fmt.Errorf("飞书事件签名无效")
	}
	return nil
}

func decryptFeishuPayload(encoded, encryptKey string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("飞书事件密文不是有效 Base64: %w", err)
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("飞书事件密文长度非法")
	}
	plain := make([]byte, len(ciphertext))
	previous := key[:aes.BlockSize]
	for offset := 0; offset < len(ciphertext); offset += aes.BlockSize {
		block.Decrypt(plain[offset:offset+aes.BlockSize], ciphertext[offset:offset+aes.BlockSize])
		for i := 0; i < aes.BlockSize; i++ {
			plain[offset+i] ^= previous[i]
		}
		previous = ciphertext[offset : offset+aes.BlockSize]
	}
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return nil, fmt.Errorf("飞书事件 PKCS7 padding 非法")
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("飞书事件 PKCS7 padding 非法")
		}
	}
	return plain[:len(plain)-padding], nil
}

func feishuEventObject(eventType string, payload map[string]any) (string, string) {
	objectType := "unknown"
	if strings.Contains(eventType, "user") || strings.Contains(eventType, "employee") {
		objectType = "user"
	} else if strings.Contains(eventType, "department") {
		objectType = "org_unit"
	}
	candidates := []string{"user_id", "open_department_id", "department_id", "object_id"}
	if value := recursiveString(payload, candidates, 0); value != "" {
		return objectType, value
	}
	return objectType, ""
}

func recursiveString(value any, keys []string, depth int) string {
	if depth > 5 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text := firstString(typed, key); text != "" {
				return text
			}
		}
		for _, child := range typed {
			if text := recursiveString(child, keys, depth+1); text != "" {
				return text
			}
		}
	case []any:
		for _, child := range typed {
			if text := recursiveString(child, keys, depth+1); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstString(object map[string]any, key string) string {
	if object == nil {
		return ""
	}
	text, _ := object[key].(string)
	return text
}
