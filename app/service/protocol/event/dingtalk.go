package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type DingTalkAdapter struct {
	token        string
	corpID       string
	aesKey       []byte
	maxClockSkew time.Duration
}

func NewDingTalkAdapter(credentials map[string]any) (*DingTalkAdapter, error) {
	value := eventCredentialString(credentials)
	key, err := decodeCallbackAESKey(value("encodingAesKey"))
	if err != nil {
		return nil, fmt.Errorf("钉钉事件凭据错误: %w", err)
	}
	adapter := &DingTalkAdapter{token: value("eventToken"), corpID: value("corpId"), aesKey: key, maxClockSkew: 10 * time.Minute}
	if adapter.token == "" || adapter.corpID == "" {
		return nil, fmt.Errorf("钉钉事件凭据必须包含 eventToken、encodingAesKey 和 corpId")
	}
	return adapter, nil
}

func (a *DingTalkAdapter) VerifyAndParse(_ context.Context, headers http.Header, body []byte) (*DirectoryEvent, error) {
	if len(body) == 0 || len(body) > 4<<20 {
		return nil, fmt.Errorf("钉钉事件请求体为空或过大")
	}
	signature := headers.Get("X-Callback-Signature")
	timestamp := headers.Get("X-Callback-Timestamp")
	nonce := headers.Get("X-Callback-Nonce")
	if err := verifyCallbackTimestamp(timestamp, true, a.maxClockSkew); err != nil {
		return nil, fmt.Errorf("钉钉%s", err.Error())
	}
	var envelope struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Encrypt == "" {
		return nil, fmt.Errorf("解析钉钉事件加密外层失败")
	}
	if err := verifyCallbackSignature(signature, a.token, timestamp, nonce, envelope.Encrypt); err != nil {
		return nil, fmt.Errorf("钉钉%s", err.Error())
	}
	plain, err := decryptCallbackMessage(envelope.Encrypt, a.aesKey, a.corpID)
	if err != nil {
		return nil, fmt.Errorf("钉钉事件解密失败: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("解析钉钉事件内容失败: %w", err)
	}
	eventType := mapString(payload, "EventType", "eventType", "event_type")
	if eventType == "" {
		return nil, fmt.Errorf("钉钉事件缺少 EventType")
	}
	response, err := a.successResponse(timestamp, nonce)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(eventType, "check_url") {
		return &DirectoryEvent{
			EventType: eventType, Challenge: "success", Payload: payload,
			CallbackResponse: &CallbackResponse{Body: response, ContentType: "application/json; charset=utf-8"},
		}, nil
	}
	eventID := mapString(payload, "EventId", "eventId", "event_id", "uuid")
	if eventID == "" {
		digest := sha256.Sum256([]byte(envelope.Encrypt))
		eventID = hex.EncodeToString(digest[:])
	}
	objectType, objectID := dingTalkEventObject(eventType, payload)
	return &DirectoryEvent{
		ProviderEventID: eventID, EventType: eventType, ObjectType: objectType,
		ExternalObjectID: objectID, Payload: payload,
		CallbackResponse: &CallbackResponse{Body: response, ContentType: "application/json; charset=utf-8"},
	}, nil
}

func (a *DingTalkAdapter) successResponse(timestamp, nonce string) ([]byte, error) {
	encrypted, err := encryptCallbackMessage([]byte("success"), a.aesKey, a.corpID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"msg_signature": callbackSHA1Signature(a.token, timestamp, nonce, encrypted),
		"timeStamp":     timestamp, "nonce": nonce, "encrypt": encrypted,
	})
}

func dingTalkEventObject(eventType string, payload map[string]any) (string, string) {
	lower := strings.ToLower(eventType)
	if strings.Contains(lower, "user") || strings.Contains(lower, "employee") {
		return "user", mapString(payload, "UserId", "userId", "userid", "user_id")
	}
	if strings.Contains(lower, "dept") || strings.Contains(lower, "org") {
		return "org_unit", mapString(payload, "DeptId", "deptId", "dept_id")
	}
	return "user", mapString(payload, "UserId", "userId", "userid", "user_id")
}

func mapString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if number, ok := payload[key].(json.Number); ok {
			return number.String()
		}
		if number, ok := payload[key].(float64); ok {
			return fmt.Sprintf("%.0f", number)
		}
	}
	return ""
}
