package event

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type WeComAdapter struct {
	token        string
	corpID       string
	aesKey       []byte
	maxClockSkew time.Duration
}

func NewWeComAdapter(credentials map[string]any) (*WeComAdapter, error) {
	value := eventCredentialString(credentials)
	key, err := decodeCallbackAESKey(value("encodingAesKey"))
	if err != nil {
		return nil, fmt.Errorf("企业微信事件凭据错误: %w", err)
	}
	adapter := &WeComAdapter{token: value("token"), corpID: value("corpId"), aesKey: key, maxClockSkew: 10 * time.Minute}
	if adapter.token == "" || adapter.corpID == "" {
		return nil, fmt.Errorf("企业微信事件凭据必须包含 token、encodingAesKey 和 corpId")
	}
	return adapter, nil
}

func (a *WeComAdapter) VerifyAndParse(_ context.Context, headers http.Header, body []byte) (*DirectoryEvent, error) {
	signature := headers.Get("X-Callback-Signature")
	timestamp := headers.Get("X-Callback-Timestamp")
	nonce := headers.Get("X-Callback-Nonce")
	if err := verifyCallbackTimestamp(timestamp, false, a.maxClockSkew); err != nil {
		return nil, fmt.Errorf("企业微信%s", err.Error())
	}
	echo := headers.Get("X-Callback-Echo")
	if echo != "" {
		if err := verifyCallbackSignature(signature, a.token, timestamp, nonce, echo); err != nil {
			return nil, fmt.Errorf("企业微信%s", err.Error())
		}
		challenge, err := decryptCallbackMessage(echo, a.aesKey, a.corpID)
		if err != nil {
			return nil, fmt.Errorf("企业微信 URL 验证解密失败: %w", err)
		}
		return &DirectoryEvent{
			EventType: "url_verification", Challenge: string(challenge), Payload: map[string]any{"type": "url_verification"},
			CallbackResponse: &CallbackResponse{Body: challenge, ContentType: "text/plain; charset=utf-8"},
		}, nil
	}
	if len(body) == 0 || len(body) > 4<<20 {
		return nil, fmt.Errorf("企业微信事件请求体为空或过大")
	}
	var envelope struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil || envelope.Encrypt == "" {
		return nil, fmt.Errorf("解析企业微信事件外层 XML 失败")
	}
	if err := verifyCallbackSignature(signature, a.token, timestamp, nonce, envelope.Encrypt); err != nil {
		return nil, fmt.Errorf("企业微信%s", err.Error())
	}
	plain, err := decryptCallbackMessage(envelope.Encrypt, a.aesKey, a.corpID)
	if err != nil {
		return nil, fmt.Errorf("企业微信事件解密失败: %w", err)
	}
	var source weComEventXML
	if err := xml.Unmarshal(plain, &source); err != nil {
		return nil, fmt.Errorf("解析企业微信事件 XML 失败: %w", err)
	}
	eventType := strings.TrimSpace(source.ChangeType)
	if eventType == "" {
		eventType = strings.TrimSpace(source.Event)
	}
	if eventType == "" {
		return nil, fmt.Errorf("企业微信事件缺少 Event 或 ChangeType")
	}
	eventID := strings.TrimSpace(source.EventID)
	if eventID == "" {
		eventID = strings.TrimSpace(source.MsgID)
	}
	if eventID == "" {
		digest := sha256.Sum256([]byte(envelope.Encrypt))
		eventID = hex.EncodeToString(digest[:])
	}
	objectType, objectID := weComEventObject(source)
	payload := map[string]any{
		"event": source.Event, "changeType": source.ChangeType, "userId": source.UserID,
		"newUserId": source.NewUserID, "departmentId": source.DepartmentID,
		"fromUserName": source.FromUserName, "createTime": source.CreateTime,
	}
	return &DirectoryEvent{
		ProviderEventID: eventID, EventType: eventType, ObjectType: objectType,
		ExternalObjectID: objectID, Payload: payload,
		CallbackResponse: &CallbackResponse{Body: []byte("success"), ContentType: "text/plain; charset=utf-8"},
	}, nil
}

type weComEventXML struct {
	FromUserName string `xml:"FromUserName"`
	CreateTime   string `xml:"CreateTime"`
	Event        string `xml:"Event"`
	ChangeType   string `xml:"ChangeType"`
	UserID       string `xml:"UserID"`
	NewUserID    string `xml:"NewUserID"`
	DepartmentID string `xml:"Id"`
	EventID      string `xml:"EventID"`
	MsgID        string `xml:"MsgId"`
}

func weComEventObject(source weComEventXML) (string, string) {
	eventType := strings.ToLower(source.ChangeType + ":" + source.Event)
	if strings.Contains(eventType, "party") || strings.Contains(eventType, "department") {
		if source.DepartmentID == "" {
			return "org_unit", ""
		}
		return "org_unit", source.DepartmentID
	}
	userID := source.NewUserID
	if userID == "" {
		userID = source.UserID
	}
	return "user", userID
}

func verifyCallbackTimestamp(value string, milliseconds bool, skew time.Duration) error {
	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("事件回调时间戳非法")
	}
	if milliseconds || unix > 100000000000 {
		unix /= 1000
	}
	delta := time.Since(time.Unix(unix, 0))
	if delta > skew || delta < -skew {
		return fmt.Errorf("事件回调时间戳超出允许偏差")
	}
	return nil
}

func eventCredentialString(credentials map[string]any) func(string) string {
	return func(key string) string {
		value, _ := credentials[key].(string)
		return strings.TrimSpace(value)
	}
}
