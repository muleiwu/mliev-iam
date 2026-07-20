package event

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestDingTalkAdapterVerifiesAndReturnsEncryptedSuccess(t *testing.T) {
	key := []byte("abcdef0123456789abcdef0123456789")
	encodedKey := base64.StdEncoding.EncodeToString(key)[:43]
	adapter, err := NewDingTalkAdapter(map[string]any{"eventToken": "event-token", "encodingAesKey": encodedKey, "corpId": "ding-corp"})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := fmt.Sprint(time.Now().UnixMilli())
	nonce := "nonce-ding"
	payload := []byte(`{"EventType":"user_modify_org","EventId":"evt-ding-1","UserId":"ada"}`)
	encrypted, err := encryptCallbackMessage(payload, key, "ding-corp")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"encrypt": encrypted})
	headers := http.Header{
		"X-Callback-Signature": []string{callbackSHA1Signature("event-token", timestamp, nonce, encrypted)},
		"X-Callback-Timestamp": []string{timestamp}, "X-Callback-Nonce": []string{nonce},
	}
	event, err := adapter.VerifyAndParse(context.Background(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.ProviderEventID != "evt-ding-1" || event.EventType != "user_modify_org" || event.ObjectType != "user" || event.ExternalObjectID != "ada" {
		t.Fatalf("钉钉事件标准化错误: %#v", event)
	}
	if event.CallbackResponse == nil {
		t.Fatal("钉钉事件必须返回加密 success")
	}
	var response map[string]string
	if err := json.Unmarshal(event.CallbackResponse.Body, &response); err != nil {
		t.Fatal(err)
	}
	if err := verifyCallbackSignature(response["msg_signature"], "event-token", timestamp, nonce, response["encrypt"]); err != nil {
		t.Fatal(err)
	}
	plain, err := decryptCallbackMessage(response["encrypt"], key, "ding-corp")
	if err != nil || string(plain) != "success" {
		t.Fatalf("钉钉确认响应无法解密: %q %v", plain, err)
	}
}

func TestDingTalkAdapterTreatsCheckURLAsChallenge(t *testing.T) {
	key := []byte("abcdef0123456789abcdef0123456789")
	encodedKey := base64.StdEncoding.EncodeToString(key)[:43]
	adapter, err := NewDingTalkAdapter(map[string]any{"eventToken": "event-token", "encodingAesKey": encodedKey, "corpId": "ding-corp"})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := fmt.Sprint(time.Now().UnixMilli())
	nonce := "challenge-nonce"
	encrypted, _ := encryptCallbackMessage([]byte(`{"EventType":"check_url"}`), key, "ding-corp")
	body, _ := json.Marshal(map[string]any{"encrypt": encrypted})
	headers := http.Header{
		"X-Callback-Signature": []string{callbackSHA1Signature("event-token", timestamp, nonce, encrypted)},
		"X-Callback-Timestamp": []string{timestamp}, "X-Callback-Nonce": []string{nonce},
	}
	event, err := adapter.VerifyAndParse(context.Background(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.Challenge != "success" || event.CallbackResponse == nil {
		t.Fatalf("钉钉 check_url 应仅返回验证响应: %#v", event)
	}
}
