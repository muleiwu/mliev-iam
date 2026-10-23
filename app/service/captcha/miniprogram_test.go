package captcha

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func miniCaptchaFixture(t *testing.T) (*MiniProgramManager, *model.Application, *model.CaptchaConfig, *miniredis.Miniredis) {
	t.Helper()
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{Name: "native captcha", Provider: TencentMiniProgramProviderType, Enabled: true, Params: datatypes.JSONMap{"secretId": "private-cloud-id", "secretKey": "private-cloud-key", "captchaAppId": "123456", "appSecretKey": "private-captcha-secret"}}
	require.NoError(t, db.Create(config).Error)
	app := &model.Application{UUID: "native-client-a", ProtocolConfig: map[string]any{"miniProgram": model.MiniProgramConfig{CaptchaConfigID: &config.ID}}}
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { rdb.Close() })
	manager := NewMiniProgramManager(db, rdb)
	manager.provider.send = func(_ *model.CaptchaConfig, req *miniCaptchaRequest, res *miniCaptchaResponse) error {
		require.Equal(t, "DescribeCaptchaMiniResult", req.GetAction())
		require.Equal(t, "2019-07-22", req.GetVersion())
		require.Equal(t, "client-ticket", *req.Ticket)
		require.EqualValues(t, 9, *req.CaptchaType)
		require.Equal(t, "192.0.2.1", *req.UserIp)
		body, err := json.Marshal(req)
		require.NoError(t, err)
		require.NotContains(t, string(body), "Randstr")
		require.NoError(t, json.Unmarshal([]byte(`{"Response":{"CaptchaCode":1}}`), res))
		return nil
	}
	return manager, app, config, server
}
func captchaParamsAndTicket(t *testing.T, m *MiniProgramManager, app *model.Application) (*dto.CaptchaClientParamsResponse, *dto.CaptchaTicket) {
	t.Helper()
	params, err := m.Params(context.Background(), app, "transaction-a", dto.MiniProgramCaptchaParamsRequest{Fingerprint: "installation-a"}, "192.0.2.1")
	require.NoError(t, err)
	require.True(t, params.Enabled)
	require.Equal(t, TencentMiniProgramProviderType, params.Provider)
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-")
	return params, &dto.CaptchaTicket{Provider: params.Provider, SessionUuid: params.SessionUuid, Data: map[string]string{"fingerprint": "installation-a", "ticket": "client-ticket"}}
}
func TestMiniProgramCaptchaSuccessIsBoundAndSingleUse(t *testing.T) {
	m, app, _, _ := miniCaptchaFixture(t)
	_, ticket := captchaParamsAndTicket(t, m, app)
	require.NoError(t, m.Validate(context.Background(), app, "transaction-a", ticket, "192.0.2.1"))
	require.ErrorContains(t, m.Validate(context.Background(), app, "transaction-a", ticket, "192.0.2.1"), "已使用")
	_, secondTicket := captchaParamsAndTicket(t, m, app)
	require.ErrorContains(t, m.Validate(context.Background(), app, "transaction-a", secondTicket, "192.0.2.1"), "票据已使用", "a fresh IAM session must not make a consumed vendor ticket reusable")
}
func TestMiniProgramCaptchaBindsApplicationAndTransaction(t *testing.T) {
	for _, policy := range []string{"explicit", "inherited"} {
		t.Run(policy, func(t *testing.T) {
			m, owner, cfg, _ := miniCaptchaFixture(t)
			if policy == "inherited" {
				createCaptchaSceneBinding(t, m.db, cfg.ID)
				owner.ProtocolConfig = map[string]any{}
			}
			other := *owner
			other.UUID = "native-client-b"
			_, ticket := captchaParamsAndTicket(t, m, owner)
			calls := 0
			send := m.provider.send
			m.provider.send = func(config *model.CaptchaConfig, req *miniCaptchaRequest, res *miniCaptchaResponse) error {
				calls++
				return send(config, req, res)
			}
			ctx := context.Background()
			// Matching provider, config and fingerprint cannot substitute for the
			// owning application and exact login transaction.
			require.ErrorContains(t, m.Validate(ctx, &other, "transaction-a", ticket, "192.0.2.1"), "当前登录不匹配")
			require.ErrorContains(t, m.Validate(ctx, owner, "transaction-b", ticket, "192.0.2.1"), "当前登录不匹配")
			require.Zero(t, calls)
			require.NoError(t, m.Validate(ctx, owner, "transaction-a", ticket, "192.0.2.1"), "foreign attempts must not consume the owner's session")
			require.Equal(t, 1, calls)
			require.ErrorContains(t, m.Validate(ctx, owner, "transaction-a", ticket, "192.0.2.1"), "已使用")
			// A new session for another application/transaction must not make the
			// already consumed Tencent ticket valid again.
			params, err := m.Params(ctx, &other, "transaction-b", dto.MiniProgramCaptchaParamsRequest{Fingerprint: "installation-a"}, "192.0.2.1")
			require.NoError(t, err)
			retargeted := *ticket
			retargeted.SessionUuid = params.SessionUuid
			require.ErrorContains(t, m.Validate(ctx, &other, "transaction-b", &retargeted, "192.0.2.1"), "票据已使用")
			require.Equal(t, 1, calls)
		})
	}
}
func TestMiniProgramCaptchaRejectsForeignExpiredAndChangedProofs(t *testing.T) {
	for _, scenario := range []string{"client", "transaction", "fingerprint", "expired", "configuration", "disabled", "provider", "remote-failure", "network-failure"} {
		t.Run(scenario, func(t *testing.T) {
			m, app, cfg, server := miniCaptchaFixture(t)
			_, ticket := captchaParamsAndTicket(t, m, app)
			transaction := "transaction-a"
			switch scenario {
			case "client":
				app.UUID = "other-client"
			case "transaction":
				transaction = "other-transaction"
			case "fingerprint":
				ticket.Data["fingerprint"] = "other-installation"
			case "expired":
				server.FastForward(6 * time.Minute)
			case "configuration":
				cfg.Params["captchaAppId"] = "99999"
				require.NoError(t, m.db.Save(cfg).Error)
			case "disabled":
				require.NoError(t, m.db.Model(cfg).Update("enabled", false).Error)
			case "provider":
				ticket.Provider = TencentProviderType
			case "remote-failure":
				m.provider.send = func(_ *model.CaptchaConfig, _ *miniCaptchaRequest, res *miniCaptchaResponse) error {
					return json.Unmarshal([]byte(`{"Response":{"CaptchaCode":21,"CaptchaMsg":"private-ticket"}}`), res)
				}
			case "network-failure":
				m.provider.send = func(_ *model.CaptchaConfig, _ *miniCaptchaRequest, _ *miniCaptchaResponse) error {
					return errors.New("private-cloud-key")
				}
			}
			err := m.Validate(context.Background(), app, transaction, ticket, "192.0.2.1")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-")
			if scenario == "client" || scenario == "transaction" || scenario == "fingerprint" {
				app.UUID = "native-client-a"
				ticket.Data["fingerprint"] = "installation-a"
				require.NoError(t, m.Validate(context.Background(), app, "transaction-a", ticket, "192.0.2.1"), "an unrelated request must not consume the owner's session")
			}
		})
	}
}
func TestMiniProgramCaptchaInheritedWebPolicyDoesNotBecomeDisabled(t *testing.T) {
	m, app, cfg, _ := miniCaptchaFixture(t)
	cfg.Provider = TencentProviderType
	require.NoError(t, m.db.Save(cfg).Error)
	createCaptchaSceneBinding(t, m.db, cfg.ID)
	app.ProtocolConfig = map[string]any{}
	params, err := m.Params(context.Background(), app, "tx", dto.MiniProgramCaptchaParamsRequest{Fingerprint: "install"}, "192.0.2.1")
	require.NoError(t, err)
	require.True(t, params.Enabled)
	require.Equal(t, TencentProviderType, params.Provider)
	require.Error(t, m.Validate(context.Background(), app, "tx", nil, "192.0.2.1"))
	zero := uint(0)
	app.ProtocolConfig["miniProgram"] = model.MiniProgramConfig{CaptchaConfigID: &zero}
	params, err = m.Params(context.Background(), app, "tx", dto.MiniProgramCaptchaParamsRequest{Fingerprint: "install"}, "192.0.2.1")
	require.NoError(t, err)
	require.False(t, params.Enabled)
	require.NoError(t, m.Validate(context.Background(), app, "tx", nil, "192.0.2.1"))
}

type captchaTestTransport func(*http.Request) (*http.Response, error)

func (f captchaTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMiniProgramCaptchaRejectsValidInheritedWebTicket(t *testing.T) {
	m, app, cfg, _ := miniCaptchaFixture(t)
	cfg.Provider = TencentProviderType
	require.NoError(t, m.db.Save(cfg).Error)
	createCaptchaSceneBinding(t, m.db, cfg.ID)
	app.ProtocolConfig = map[string]any{}
	// Only the external Tencent API is stubbed. Its successful result must not
	// let native callers skip the application/transaction-bound captcha session.
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	calls := 0
	http.DefaultTransport = captchaTestTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, TencentAPIHost, req.URL.Host)
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"Response":{"CaptchaCode":1}}`))}, nil
	})
	ticket := &dto.CaptchaTicket{Provider: TencentProviderType, Data: map[string]string{"ticket": "valid-web-ticket", "randstr": "valid-web-randstr"}}
	for _, clientID := range []string{app.UUID, "other-client"} {
		other := *app
		other.UUID = clientID
		for _, transaction := range []string{"transaction-a", "transaction-b"} {
			require.ErrorContains(t, m.Validate(context.Background(), &other, transaction, ticket, "192.0.2.1"), "小程序验证码类型不支持")
		}
	}
	require.Zero(t, calls, "incompatible tickets must be rejected before contacting Tencent")
	// The browser login endpoint retains its existing Web captcha support.
	require.NoError(t, NewManager(m.db).ValidateForScene("login", ticket, "192.0.2.1", m.redis))
	require.Equal(t, 1, calls)
}
func TestMiniProgramCaptchaAidEncryptionMatchesDocumentedFormat(t *testing.T) {
	_, _, cfg, _ := miniCaptchaFixture(t)
	cfg.Params["aidEncryptionEnabled"] = "true"
	now := time.Unix(1792700000, 0)
	params, err := miniCaptchaPublicParams(cfg, now)
	require.NoError(t, err)
	encoded := params["aidEncrypted"].(string)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	secret := cfg.Params["appSecretKey"].(string)
	key := []byte(strings.Repeat(secret, 2))[:32]
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	iv, encrypted := raw[:16], raw[16:]
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, encrypted)
	padding := int(plain[len(plain)-1])
	require.True(t, padding > 0 && padding <= 16)
	require.Equal(t, "123456&1792700000&300", string(plain[:len(plain)-padding]))
	again, err := miniCaptchaPublicParams(cfg, now)
	require.NoError(t, err)
	require.NotEqual(t, encoded, again["aidEncrypted"])
}
func TestMiniProgramCaptchaProviderIsListedWithoutChangingWebProvider(t *testing.T) {
	m, _, _, _ := miniCaptchaFixture(t)
	providers := NewManager(m.db).GetSupportedProviders()
	names := []string{}
	for _, p := range providers {
		names = append(names, p.Provider)
	}
	require.Contains(t, names, TencentProviderType)
	require.Contains(t, names, TencentMiniProgramProviderType)
}

func TestMiniProgramCaptchaConcurrentTicketConsumption(t *testing.T) {
	m, app, _, _ := miniCaptchaFixture(t)
	_, first := captchaParamsAndTicket(t, m, app)
	_, second := captchaParamsAndTicket(t, m, app)
	var calls atomic.Int32
	m.provider.send = func(_ *model.CaptchaConfig, _ *miniCaptchaRequest, result *miniCaptchaResponse) error {
		calls.Add(1)
		return json.Unmarshal([]byte(`{"Response":{"CaptchaCode":1}}`), result)
	}
	ready := make(chan struct{})
	results := make(chan error, 2)
	for _, ticket := range []*dto.CaptchaTicket{first, second} {
		go func(ticket *dto.CaptchaTicket) {
			<-ready
			results <- m.Validate(context.Background(), app, "transaction-a", ticket, "192.0.2.1")
		}(ticket)
	}
	close(ready)
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	require.EqualValues(t, 1, calls.Load())
}
