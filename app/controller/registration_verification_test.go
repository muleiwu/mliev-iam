package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	httpimpl "cnb.cool/mliev/open/go-web/pkg/server/http_server/impl"
	mlievpush "github.com/mliev-sdk/push-go"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type registrationCodeFixture struct {
	*accountAuthFixture
	jar      http.CookieJar
	messages chan mlievpush.SendMessageRequest
}

func newRegistrationCodeFixture(t *testing.T) *registrationCodeFixture {
	t.Helper()
	f := newAccountAuthFixture(t)
	messages := make(chan mlievpush.SendMessageRequest, 20)
	push := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message mlievpush.SendMessageRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&message))
		messages <- message
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"registration-test","status":"pending"}}`))
	}))
	t.Cleanup(push.Close)
	config, err := json.Marshal(map[string]any{"baseUrl": push.URL, "appId": "test-app", "appSecret": "test-secret", "emailChannelId": 42, "smsChannelId": 43})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&model.Setting{Key: service.PushSettingsConfigKey, Value: string(config)}).Error)
	router := httpimpl.NewRouter(f.engine, httpimpl.NewHttpDeps())
	router.POST("/api/oidc/send-register-verification-code", OIDCAuthFlowController{}.SendRegisterVerificationCode)
	router.POST("/api/oidc/register", OIDCAuthFlowController{}.Register)
	return &registrationCodeFixture{accountAuthFixture: f, jar: accountTestJar(t), messages: messages}
}

func (f *registrationCodeFixture) sendCode(t *testing.T, kind, receiver string) string {
	t.Helper()
	sent := accountTestData(t, f.request(t, f.jar, "POST", "/api/oidc/send-register-verification-code", map[string]any{"type": kind, "receiver": receiver}))
	require.Equal(t, true, sent["success"])
	select {
	case message := <-f.messages:
		require.Equal(t, receiver, message.Receiver)
		require.Regexp(t, `^\d{6}$`, message.TemplateParams["code"])
		return message.TemplateParams["code"]
	case <-time.After(time.Second):
		t.Fatal("verification message was not sent")
		return ""
	}
}

func (f *registrationCodeFixture) emailRequest(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"username": "new_registration_user", "password": "Correct-password123", "realName": "Test User", "email": "registration@example.test", "emailCode": f.sendCode(t, "email", "registration@example.test")}
}

func (f *registrationCodeFixture) register(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return f.request(t, f.jar, "POST", "/api/oidc/register", body)
}

func requireRegistrationFailure(t *testing.T, rec *httptest.ResponseRecorder, message string) {
	t.Helper()
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.NotZero(t, response.Code, rec.Body.String())
	require.Contains(t, response.Message, message)
}

func TestRegistrationVerificationRetryAfterUsernameConflict(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	require.NoError(t, f.db.Create(&model.User{Username: "taken_user", Status: model.UserStatusActive, IsActivated: true}).Error)
	body["username"] = "taken_user"
	requireRegistrationFailure(t, f.register(t, body), "用户名已存在")
	body["username"] = "corrected_user"
	result := accountTestData(t, f.register(t, body))
	require.Equal(t, true, result["success"], "correcting the username must not require another email code")
}

func TestRegistrationVerificationRetryAfterWrongContactCode(t *testing.T) {
	for _, kind := range []string{"email", "phone"} {
		t.Run(kind, func(t *testing.T) {
			f := newRegistrationCodeFixture(t)
			body := f.emailRequest(t)
			body["phone"] = "+8613800138009"
			body["phoneCode"] = f.sendCode(t, "phone", body["phone"].(string))
			correct := body[kind+"Code"].(string)
			wrong := "000000"
			if correct == wrong {
				wrong = "111111"
			}
			body[kind+"Code"] = wrong
			label := map[string]string{"email": "邮箱", "phone": "手机"}[kind]
			requireRegistrationFailure(t, f.register(t, body), label+"验证码错误或已过期")
			body[kind+"Code"] = correct
			result := accountTestData(t, f.register(t, body))
			require.Equal(t, true, result["success"])
			user, err := service.NewUserService(f.db).GetUserModelByID(uint(result["userId"].(float64)))
			require.NoError(t, err)
			require.NotNil(t, user.EmailVerifiedAt)
			require.NotNil(t, user.PhoneVerifiedAt)
		})
	}
}

func TestRegistrationVerificationRetryAfterDatabaseRollback(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	require.NoError(t, f.db.Exec("CREATE TRIGGER fail_registration_verification BEFORE UPDATE OF email_verified_at ON users BEGIN SELECT RAISE(ABORT, 'injected registration failure'); END").Error)
	requireRegistrationFailure(t, f.register(t, body), "injected registration failure")
	require.NoError(t, f.db.Exec("DROP TRIGGER fail_registration_verification").Error)
	result := accountTestData(t, f.register(t, body))
	require.Equal(t, true, result["success"], "a rolled-back user and contact verification must be retryable with the delivered code")
}

func TestRegistrationVerificationPreservesOriginalExpiry(t *testing.T) {
	for _, remaining := range []time.Duration{59 * time.Second, 61 * time.Second} {
		t.Run(remaining.String(), func(t *testing.T) {
			f := newRegistrationCodeFixture(t)
			body := f.emailRequest(t)
			require.NoError(t, f.db.Create(&model.User{Username: "taken_user", Status: model.UserStatusActive, IsActivated: true}).Error)
			body["username"] = "taken_user"
			f.clock.FastForward(4 * time.Minute)
			requireRegistrationFailure(t, f.register(t, body), "用户名已存在")
			body["username"] = "corrected_user"
			f.clock.FastForward(remaining)
			if remaining < time.Minute {
				require.Equal(t, true, accountTestData(t, f.register(t, body))["success"])
			} else {
				requireRegistrationFailure(t, f.register(t, body), "邮箱验证码错误或已过期")
			}
		})
	}
}

func TestRegistrationVerificationConsumesSuccessfulProof(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	result := accountTestData(t, f.register(t, body))
	require.Equal(t, true, result["success"])
	// Free the address through the normal administrator service. Replay must fail
	// because the proof was consumed, not just because of contact uniqueness.
	_, err := service.NewUserService(f.db).UpdateUser(uint(result["userId"].(float64)), &dto.UpdateUserRequest{Email: "changed@example.test"})
	require.NoError(t, err)
	body["username"] = "replay_user"
	requireRegistrationFailure(t, f.register(t, body), "邮箱验证码错误或已过期")
}

// Inject a boundary event after Redis accepts the first atomic operation and
// before the registration transaction starts; no application functions are mocked.
type registrationRedisHook struct {
	once  sync.Once
	after func()
}

func (h *registrationRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *registrationRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *registrationRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil && cmd.Name() == "eval" {
			h.once.Do(h.after)
		}
		return err
	}
}

func TestRegistrationVerificationResendSurvivesFailedAttempt(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	require.NoError(t, f.db.Create(&model.User{Username: "taken_user", Status: model.UserStatusActive, IsActivated: true}).Error)
	body["username"] = "taken_user"
	var replacement string
	f.redis.AddHook(&registrationRedisHook{after: func() {
		f.clock.FastForward(61 * time.Second)
		replacement = f.sendCode(t, "email", body["email"].(string))
	}})
	requireRegistrationFailure(t, f.register(t, body), "用户名已存在")
	body["username"], body["emailCode"] = "corrected_user", replacement
	require.Equal(t, true, accountTestData(t, f.register(t, body))["success"], "rollback must not replace the newly delivered code with an older one")
}

func TestRegistrationVerificationDoesNotRestoreExpiredProof(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	require.NoError(t, f.db.Create(&model.User{Username: "taken_user", Status: model.UserStatusActive, IsActivated: true}).Error)
	body["username"] = "taken_user"
	f.redis.AddHook(&registrationRedisHook{after: func() { f.clock.FastForward(6 * time.Minute) }})
	requireRegistrationFailure(t, f.register(t, body), "用户名已存在")
	body["username"] = "corrected_user"
	requireRegistrationFailure(t, f.register(t, body), "邮箱验证码错误或已过期")
}

func TestRegistrationVerificationCancelledRequestCanRetry(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	body := f.emailRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.redis.AddHook(&registrationRedisHook{after: cancel})
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", f.origin.String()+"/api/oidc/register", bytes.NewReader(payload)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	requireRegistrationFailure(t, rec, "context canceled")
	require.Equal(t, true, accountTestData(t, f.register(t, body))["success"])
}

func TestRegistrationVerificationConcurrentUseIsSingleUse(t *testing.T) {
	f := newRegistrationCodeFixture(t)
	base := f.emailRequest(t)
	responses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		body := make(map[string]any, len(base))
		for key, value := range base {
			body[key] = value
		}
		body["username"] = fmt.Sprintf("concurrent_user_%d", i)
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", f.origin.String()+"/api/oidc/register", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			f.engine.ServeHTTP(rec, req)
			responses <- rec
		}()
	}
	wg.Wait()
	close(responses)
	succeeded := 0
	for rec := range responses {
		if rec.Code == http.StatusOK {
			require.Equal(t, true, accountTestData(t, rec)["success"])
			succeeded++
		} else {
			requireRegistrationFailure(t, rec, "邮箱验证码错误或已过期")
		}
	}
	require.Equal(t, 1, succeeded)
}
