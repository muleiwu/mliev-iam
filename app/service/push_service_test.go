package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	mlievpush "github.com/mliev-sdk/push-go"
	"github.com/muleiwu/golog"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func pushTestDetail() mlievpush.ChannelDetailData {
	return mlievpush.ChannelDetailData{
		ChannelData:       mlievpush.ChannelData{ID: 42, Type: "sms", Name: "验证码", TemplateName: "验证码模板", Readiness: mlievpush.ChannelReadiness{State: "ready"}},
		Template:          &mlievpush.ChannelTemplate{ID: 7, Content: "{{otp}}，{{minutes}} 分钟有效，{{brand}}", Variables: []string{"otp", "minutes", "brand"}},
		SignatureRequired: true, SignatureNames: []string{"登录验证"},
	}
}

func pushTestConfig(baseURL string) dto.PushSettings {
	return dto.PushSettings{
		BaseURL: baseURL, AppID: "test-app", AppSecret: "test-secret", SMSChannelID: 42, SMSSignature: "登录验证",
		SMSTemplateBindings: map[string]dto.PushTemplateBinding{"otp": {Source: "code"}, "minutes": {Source: "expire_minutes"}, "brand": {Source: "constant", Value: "IAM"}},
	}
}

func pushReply(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}))
}

func TestPushCatalogQueriesAndNullableTemplates(t *testing.T) {
	detail := pushTestDetail()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		for _, header := range []string{"X-App-Id", "X-Timestamp", "X-Nonce", "X-Signature"} {
			require.NotEmpty(t, r.Header.Get(header))
		}
		if r.URL.Path == "/api/v1/channels" {
			require.Equal(t, "sms", r.URL.Query().Get("type"))
			require.Equal(t, "2", r.URL.Query().Get("page"))
			require.Equal(t, "100", r.URL.Query().Get("page_size"))
			blocked := detail.ChannelData
			blocked.Readiness = mlievpush.ChannelReadiness{State: "blocked", BlockerCodes: []string{"template_missing"}}
			pushReply(t, w, mlievpush.ListChannelsData{Items: []mlievpush.ChannelData{blocked}, Total: 101, Page: 2, Size: 100})
			return
		}
		require.Equal(t, "/api/v1/channels/42", r.URL.Path)
		pushReply(t, w, detail)
	}))
	defer server.Close()
	push, err := NewPushService(pushTestConfig(server.URL + "/").Credentials())
	require.NoError(t, err)
	page, err := push.ListChannels(context.Background(), dto.PushChannelsQueryRequest{Type: "sms", Page: 2, PageSize: 100})
	require.NoError(t, err)
	require.EqualValues(t, 101, page.Total)
	require.Equal(t, "blocked", page.Items[0].Readiness.State)
	for _, mode := range []string{"missing", "invalid", "empty"} {
		switch mode {
		case "missing":
			detail.Template = nil
		case "invalid":
			detail.Template = &mlievpush.ChannelTemplate{Variables: nil}
		case "empty":
			detail.Template = &mlievpush.ChannelTemplate{Variables: []string{}}
		}
		got, err := push.GetChannel(context.Background(), 42)
		require.NoError(t, err)
		payload, err := json.Marshal(got)
		require.NoError(t, err)
		require.Contains(t, string(payload), "signatureRequired")
		require.NotContains(t, string(payload), "test-secret")
		switch mode {
		case "missing":
			require.Contains(t, string(payload), `"template":null`)
		case "invalid":
			require.Contains(t, string(payload), `"variables":null`)
		case "empty":
			require.Contains(t, string(payload), `"variables":[]`)
		}
	}
}

func TestPushTemplateParams(t *testing.T) {
	legacy, err := BuildPushTemplateParams(nil, "012345", 7*time.Minute)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"code": "012345", "expire_time": "7"}, legacy)
	custom, err := BuildPushTemplateParams(pushTestConfig("").SMSTemplateBindings, "012345", verificationCodeExpiry)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"otp": "012345", "minutes": "5", "brand": "IAM"}, custom)
	for _, bindings := range []map[string]dto.PushTemplateBinding{
		{}, {"code": {Source: "constant"}}, {"code": {Source: "unsupported"}}, {"": {Source: "code"}},
	} {
		_, err := BuildPushTemplateParams(bindings, "123456", verificationCodeExpiry)
		require.Error(t, err)
	}
}

func TestPushValidateChannel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*mlievpush.ChannelDetailData, *dto.PushSettings)
		valid  bool
	}{
		{"ready", func(_ *mlievpush.ChannelDetailData, _ *dto.PushSettings) {}, true},
		{"degraded", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Readiness.State = "degraded" }, true},
		{"blocked", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Readiness.State = "blocked" }, false},
		{"wrong type", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Type = "email" }, false},
		{"wrong id", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.ID = 9 }, false},
		{"missing template", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Template = nil }, false},
		{"invalid variables", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Template.Variables = nil }, false},
		{"no variables", func(d *mlievpush.ChannelDetailData, _ *dto.PushSettings) { d.Template.Variables = []string{} }, false},
		{"missing signature", func(_ *mlievpush.ChannelDetailData, c *dto.PushSettings) { c.SMSSignature = "" }, false},
		{"unknown signature", func(_ *mlievpush.ChannelDetailData, c *dto.PushSettings) { c.SMSSignature = "not-an-alias" }, false},
		{"optional signature", func(d *mlievpush.ChannelDetailData, c *dto.PushSettings) {
			d.SignatureRequired = false
			c.SMSSignature = ""
		}, true},
		{"missing binding", func(_ *mlievpush.ChannelDetailData, c *dto.PushSettings) { delete(c.SMSTemplateBindings, "brand") }, false},
		{"code absent from template", func(d *mlievpush.ChannelDetailData, c *dto.PushSettings) { d.Template.Variables = []string{"brand"} }, false},
		{"legacy", func(d *mlievpush.ChannelDetailData, c *dto.PushSettings) {
			d.Template.Variables = []string{"code", "expire_time"}
			c.SMSTemplateBindings = nil
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail := pushTestDetail()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { pushReply(t, w, detail) }))
			defer server.Close()
			config := pushTestConfig(server.URL)
			tc.change(&detail, &config)
			push, err := NewPushService(config.Credentials())
			require.NoError(t, err)
			err = push.ValidateChannel(context.Background(), config, "sms")
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPushErrorsAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			<-r.Context().Done()
			return
		}
		_, _ = fmt.Fprint(w, `{"code":20003,"message":"invalid test-secret for test-app"}`)
	}))
	defer server.Close()
	push, err := NewPushService(pushTestConfig(server.URL).Credentials(), mlievpush.WithTimeout(30*time.Millisecond))
	require.NoError(t, err)
	_, err = push.ListChannels(context.Background(), dto.PushChannelsQueryRequest{Type: "sms", Page: 1})
	require.ErrorContains(t, err, "20003")
	require.NotContains(t, err.Error(), "test-secret")
	require.NotContains(t, err.Error(), "test-app")
	_, err = push.ListChannels(context.Background(), dto.PushChannelsQueryRequest{Type: "sms", Page: 2})
	var pushError *PushError
	require.ErrorAs(t, err, &pushError)
	require.Equal(t, 504, pushError.Status)
	for _, req := range []dto.PushChannelsQueryRequest{{Type: "push"}, {Type: "sms", Page: -1}, {Type: "email", PageSize: 101}} {
		_, err := push.ListChannels(context.Background(), req)
		require.ErrorAs(t, err, &pushError)
		require.Equal(t, 400, pushError.Status)
	}
}

func TestPushSettingsPersistenceAndLegacyCompatibility(t *testing.T) {
	svc, db := newRegisterFieldsTestService(t)
	defaults, err := svc.GetPushSettings()
	require.NoError(t, err)
	require.Zero(t, defaults.SMSChannelID)
	require.Zero(t, defaults.EmailChannelID)
	legacy := `{"baseUrl":"https://old.example","appId":"old-app","appSecret":"old-secret","smsChannelId":17,"smsSignature":"old-alias"}`
	require.NoError(t, db.Create(&model.Setting{Key: PushSettingsConfigKey, Value: legacy}).Error)
	got, err := svc.GetPushSettings()
	require.NoError(t, err)
	require.Equal(t, 17, got.SMSChannelID)
	require.Nil(t, got.SMSTemplateBindings)
	detail := pushTestDetail()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { pushReply(t, w, detail) }))
	defer server.Close()
	config := pushTestConfig(server.URL)
	req := dto.UpdatePushSettingsRequest(config)
	require.NoError(t, svc.UpdatePushSettings(&req))
	got, err = svc.GetPushSettings()
	require.NoError(t, err)
	require.Equal(t, config, *got)
	detail.Readiness.State = "blocked"
	req.SMSSignature = "changed"
	require.Error(t, svc.UpdatePushSettings(&req))
	got, err = svc.GetPushSettings()
	require.NoError(t, err)
	require.Equal(t, config, *got, "failed validation must not replace the live configuration")
	req.SMSChannelID = 0
	require.NoError(t, svc.UpdatePushSettings(&req), "disabled channels do not require a reachable catalog")
}

// The hook executes real go-redis commands in memory, so these tests need no Redis server.
type pushRedisHook struct{ values map[string]string }

func (h *pushRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *pushRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *pushRedisHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		key := fmt.Sprint(args[1])
		switch cmd.Name() {
		case "exists":
			var count int64
			if _, ok := h.values[key]; ok {
				count = 1
			}
			cmd.(*redis.IntCmd).SetVal(count)
		case "set":
			h.values[key] = fmt.Sprint(args[2])
			cmd.(*redis.StatusCmd).SetVal("OK")
		case "del":
			delete(h.values, key)
			cmd.(*redis.IntCmd).SetVal(1)
		case "get":
			value, ok := h.values[key]
			if !ok {
				cmd.SetErr(redis.Nil)
				return redis.Nil
			}
			cmd.(*redis.StringCmd).SetVal(value)
		default:
			return fmt.Errorf("unexpected Redis command: %s", cmd.Name())
		}
		return nil
	}
}

func TestVerificationAndTestPushShareParameters(t *testing.T) {
	container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop())))
	for _, kind := range []string{"sms", "email"} {
		t.Run(kind, func(t *testing.T) {
			var messages []mlievpush.SendMessageRequest
			detail := pushTestDetail()
			detail.Type = kind
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					pushReply(t, w, detail)
					return
				}
				var req mlievpush.SendMessageRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				messages = append(messages, req)
				pushReply(t, w, mlievpush.SendMessageData{TaskID: "test-task", Status: "pending"})
			}))
			defer server.Close()
			config := pushTestConfig(server.URL)
			receiver, verificationType := "13800138000", VerificationTypePhone
			if kind == "email" {
				config.EmailChannelID, config.EmailSignature, config.EmailTemplateBindings = config.SMSChannelID, config.SMSSignature, config.SMSTemplateBindings
				config.SMSChannelID = 0
				receiver, verificationType = "user@example.com", VerificationTypeEmail
			}
			hook := &pushRedisHook{values: map[string]string{}}
			redisClient := redis.NewClient(&redis.Options{Addr: "unused"})
			redisClient.AddHook(hook)
			defer redisClient.Close()
			verification := NewVerificationCodeService(redisClient, &VerificationCodeConfig{
				BaseURL: config.BaseURL, AppID: config.AppID, AppSecret: config.AppSecret,
				SMSChannelID: config.SMSChannelID, SMSSignature: config.SMSSignature, SMSTemplateBindings: config.SMSTemplateBindings,
				EmailChannelID: config.EmailChannelID, EmailSignature: config.EmailSignature, EmailTemplateBindings: config.EmailTemplateBindings,
			})
			require.NoError(t, verification.SendCode(context.Background(), receiver, verificationType, "session", "trace"))
			require.Len(t, hook.values, 2)
			storedCode := hook.values[verification.getCodeKey(receiver, verificationType, "session")]
			require.Equal(t, storedCode, messages[0].TemplateParams["otp"])
			push, err := NewPushService(config.Credentials())
			require.NoError(t, err)
			result, err := push.Test(context.Background(), dto.PushTestRequest{Config: config, Type: kind, Receiver: receiver})
			require.NoError(t, err)
			require.Equal(t, "pending", result.Status)
			require.Len(t, hook.values, 2, "test messages must not touch login-code Redis")
			require.Len(t, messages, 2)
			for _, message := range messages {
				require.Regexp(t, `^\d{6}$`, message.TemplateParams["otp"])
				require.Equal(t, "5", message.TemplateParams["minutes"])
				require.Equal(t, "IAM", message.TemplateParams["brand"])
				require.Equal(t, "登录验证", message.SignatureName)
			}
			err = verification.VerifyCode(context.Background(), receiver, verificationType, "another-session", messages[1].TemplateParams["otp"])
			require.ErrorContains(t, err, "不存在")
			err = verification.SendCode(context.Background(), receiver, verificationType, "session", "trace")
			require.ErrorContains(t, err, "频繁")
		})
	}
}

func TestVerificationPushFailureCleansRedisAndLegacyParameters(t *testing.T) {
	container.Register(container.NewSimpleProvider(reflect.TypeFor[gsr.Logger](), golog.NewLoggerWithZap(zap.NewNop())))
	var params map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mlievpush.SendMessageRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		params = request.TemplateParams
		_, _ = fmt.Fprint(w, `{"code":30004,"message":"channel disabled"}`)
	}))
	defer server.Close()
	hook := &pushRedisHook{values: map[string]string{}}
	client := redis.NewClient(&redis.Options{Addr: "unused"})
	client.AddHook(hook)
	defer client.Close()
	verification := NewVerificationCodeService(client, &VerificationCodeConfig{BaseURL: server.URL, AppID: "test-app", AppSecret: "test-secret", SMSChannelID: 42})
	err := verification.SendCode(context.Background(), "13800138000", VerificationTypePhone, "session", "trace")
	require.Error(t, err)
	require.Empty(t, hook.values)
	require.Len(t, params, 2)
	require.Equal(t, "5", params["expire_time"])
	require.Regexp(t, `^\d{6}$`, params["code"])
	require.False(t, strings.Contains(err.Error(), "test-secret"))
}
