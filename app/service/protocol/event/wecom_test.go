package event

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWeComAdapterVerifiesURLAndParsesEncryptedContactEvent(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encodedKey := base64.StdEncoding.EncodeToString(key)[:43]
	adapter, err := NewWeComAdapter(map[string]any{"token": "verify-token", "encodingAesKey": encodedKey, "corpId": "corp-a"})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := fmt.Sprint(time.Now().Unix())
	nonce := "nonce-a"
	echo, err := encryptCallbackMessage([]byte("verified"), key, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	challengeHeaders := http.Header{
		"X-Callback-Signature": []string{callbackSHA1Signature("verify-token", timestamp, nonce, echo)},
		"X-Callback-Timestamp": []string{timestamp}, "X-Callback-Nonce": []string{nonce}, "X-Callback-Echo": []string{echo},
	}
	challenge, err := adapter.VerifyAndParse(context.Background(), challengeHeaders, nil)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Challenge != "verified" || challenge.CallbackResponse == nil || string(challenge.CallbackResponse.Body) != "verified" {
		t.Fatalf("企业微信 URL 验证响应错误: %#v", challenge)
	}

	plain := []byte(`<xml><FromUserName>corp-a</FromUserName><CreateTime>1710000000</CreateTime><Event>change_contact</Event><ChangeType>update_user</ChangeType><UserID>ada</UserID><EventID>evt-wecom-1</EventID></xml>`)
	encrypted, err := encryptCallbackMessage(plain, key, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`<xml><Encrypt><![CDATA[` + encrypted + `]]></Encrypt></xml>`)
	headers := http.Header{
		"X-Callback-Signature": []string{callbackSHA1Signature("verify-token", timestamp, nonce, encrypted)},
		"X-Callback-Timestamp": []string{timestamp}, "X-Callback-Nonce": []string{nonce},
	}
	event, err := adapter.VerifyAndParse(context.Background(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.ProviderEventID != "evt-wecom-1" || event.EventType != "update_user" || event.ObjectType != "user" || event.ExternalObjectID != "ada" {
		t.Fatalf("企业微信事件标准化错误: %#v", event)
	}
	if event.CallbackResponse == nil || string(event.CallbackResponse.Body) != "success" {
		t.Fatalf("企业微信事件确认响应错误: %#v", event.CallbackResponse)
	}

	badHeaders := headers.Clone()
	badHeaders.Set("X-Callback-Signature", "invalid")
	if _, err := adapter.VerifyAndParse(context.Background(), badHeaders, body); err == nil {
		t.Fatal("企业微信无效签名必须被拒绝")
	}
}

func TestWeComAdapterRequiresCallbackCredentials(t *testing.T) {
	if _, err := NewWeComAdapter(map[string]any{"token": "only-token"}); err == nil {
		t.Fatal("缺少 EncodingAESKey/corpId 时应拒绝创建企业微信事件适配器")
	}
}
