package captcha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func geetestTestConfig() *model.CaptchaConfig {
	return &model.CaptchaConfig{
		Provider: GeetestProviderType,
		Params: datatypes.JSONMap{
			"captchaId":  "test-captcha-id",
			"captchaKey": "test-captcha-key",
		},
	}
}

func geetestTestData() map[string]string {
	return map[string]string{
		"lot_number":     "lot-123",
		"captcha_output": "output-123",
		"pass_token":     "pass-123",
		"gen_time":       "1700000000",
	}
}

func TestGeetestProviderDoesNotExposeCaptchaKey(t *testing.T) {
	params := (&GeetestCaptchaProvider{}).GetClientParams(geetestTestConfig())
	require.Equal(t, "test-captcha-id", params["captchaId"])
	require.NotContains(t, params, "captchaKey")

	raw, err := json.Marshal(params)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "test-captcha-key")
}

func TestGeetestProviderValidateSuccess(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "test-captcha-id", r.URL.Query().Get("captcha_id"))
		require.NoError(t, r.ParseForm())
		received = r.PostForm
		// 官方成功响应原始结构：status + result + reason + captcha_args
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"result": "success",
			"reason": "",
			"captcha_args": map[string]any{
				"used_type":  "slide",
				"user_ip":    "127.0.0.1",
				"lot_number": "lot-123",
				"scene":      "login",
				"referer":    "https://example.test/login",
			},
		})
	}))
	defer server.Close()

	provider := &GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL + "/validate"}
	result, err := provider.Verify(geetestTestConfig(), geetestTestData(), "")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "lot-123", received.Get("lot_number"))
	require.Equal(t, "output-123", received.Get("captcha_output"))
	require.Equal(t, "pass-123", received.Get("pass_token"))
	require.Equal(t, "1700000000", received.Get("gen_time"))
	require.Equal(t, geetestSignToken("test-captcha-key", "lot-123"), received.Get("sign_token"))
}

func TestGeetestProviderRejectsFailAndErrorResponses(t *testing.T) {
	t.Run("fail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// 官方失败响应同样是 status=success，但 result=fail
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":       "success",
				"result":       "fail",
				"reason":       "pass_token expire",
				"captcha_args": map[string]any{},
			})
		}))
		defer server.Close()

		result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL + "/validate"}).Verify(geetestTestConfig(), geetestTestData(), "")
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Equal(t, "验证已过期，请重新验证", result.Message)
	})

	t.Run("error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "code": "-50305", "msg": "lot_number expired"})
		}))
		defer server.Close()

		_, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL + "/validate"}).Verify(geetestTestConfig(), geetestTestData(), "")
		require.ErrorContains(t, err, "-50305")
	})
}

// status 是服务调用状态，result 才是校验结论；两者必须同时为 success。
// 评审指出的 {"status":"error","result":"success"} 组合绝不能放行。
func TestGeetestProviderRequiresStatusAndResultSuccess(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "error status with success result", body: `{"status":"error","result":"success","code":"-50005","msg":"illegal gen_time"}`},
		{name: "missing status", body: `{"result":"success"}`},
		{name: "unknown status", body: `{"status":"unknown","result":"success"}`},
		{name: "empty status with success result", body: `{"status":"","result":"success"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			provider := &GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}
			result, err := provider.Verify(geetestTestConfig(), geetestTestData(), "")
			if err != nil {
				require.Error(t, err)
				return
			}
			require.NotNil(t, result)
			require.False(t, result.Success)
		})
	}
}

func TestGeetestProviderOnlyAcceptsExactSuccessPair(t *testing.T) {
	require.True(t, geetestPassed("success", "success"))
	require.True(t, geetestPassed(" SUCCESS ", "Success"))
	require.False(t, geetestPassed("error", "success"))
	require.False(t, geetestPassed("", "success"))
	require.False(t, geetestPassed("success", "fail"))
}

// bypass 标记可能出现在 status/result/reason 任一字段，且文案会变。
func TestGeetestBypassDetectionCoversAllFields(t *testing.T) {
	require.True(t, geetestBypassed("bypass status", "success", ""))
	require.True(t, geetestBypassed("success", "BYPASS_SUCCESS", ""))
	require.True(t, geetestBypassed("success", "success", "captcha bypass enabled"))
	require.False(t, geetestBypassed("success", "success", ""))
	require.False(t, geetestBypassed("error", "fail", "pass_token expire"))
}

// 极验配置只接受 captchaId / captchaKey / allowBypass，不再暴露 apiServer。
func TestGeetestProviderSendsCaptchaIdInFormAndQuery(t *testing.T) {
	var seenForm, seenQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/validate", r.URL.Path)
		require.NoError(t, r.ParseForm())
		seenQuery = r.URL.Query().Get("captcha_id")
		seenForm = r.PostForm.Get("captcha_id")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "result": "success"})
	}))
	defer server.Close()

	result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL + "/validate"}).Verify(geetestTestConfig(), geetestTestData(), "")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "test-captcha-id", seenQuery)
	require.Equal(t, "test-captcha-id", seenForm)
}

func TestGeetestProviderRejectsIncompleteData(t *testing.T) {
	_, err := (&GeetestCaptchaProvider{}).Verify(geetestTestConfig(), map[string]string{"lot_number": "lot-123"}, "")
	require.Error(t, err)
}

func TestGeetestProviderRejectsMalformedHTTPResponses(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{name: "http non-200", statusCode: http.StatusBadGateway, body: `{}`, want: "HTTP 502"},
		{name: "invalid json", statusCode: http.StatusOK, body: `not-json`, want: "响应异常"},
		{name: "missing result", statusCode: http.StatusOK, body: `{}`, want: "验证失败"},
		{name: "wrong result", statusCode: http.StatusOK, body: `{"result":"unknown"}`, want: "验证失败"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL + "/validate"}).Verify(geetestTestConfig(), geetestTestData(), "")
			if err != nil {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NotNil(t, result)
			require.False(t, result.Success)
			require.Contains(t, result.Message, tc.want)
		})
	}
}

func TestGeetestProviderRejectsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"result":"success"}`))
	}))
	defer server.Close()

	provider := &GeetestCaptchaProvider{
		client:      &http.Client{Timeout: 30 * time.Millisecond},
		validateURL: server.URL,
	}
	_, err := provider.Verify(geetestTestConfig(), geetestTestData(), "")
	require.ErrorContains(t, err, "暂不可用")
}

func TestGeetestProviderRejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		config *model.CaptchaConfig
	}{
		{name: "nil config", config: nil},
		{name: "missing key", config: &model.CaptchaConfig{Params: datatypes.JSONMap{"captchaId": "test-captcha-id"}}},
		{name: "missing id", config: &model.CaptchaConfig{Params: datatypes.JSONMap{"captchaKey": "test-captcha-key"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&GeetestCaptchaProvider{}).Verify(tc.config, geetestTestData(), "")
			require.Error(t, err)
		})
	}
}

func TestGeetestSignTokenIsHMACSHA256Hex(t *testing.T) {
	// Known vector: HMAC-SHA256("key", "message")
	require.Equal(t, "6e9ef29b75fffc5b7abae527d58fdadb2fe42e7219011976917343065f58ed4a", geetestSignToken("key", "message"))
}

func TestGeetestProviderIsRegistered(t *testing.T) {
	manager := NewManager(nil)
	provider, err := manager.GetProvider(GeetestProviderType)
	require.NoError(t, err)
	require.Equal(t, GeetestProviderType, provider.GetProviderType())

	infos := manager.GetSupportedProviders()
	found := false
	for _, info := range infos {
		if info.Provider == GeetestProviderType {
			found = true
			require.NotEmpty(t, info.Fields)
			break
		}
	}
	require.True(t, found)
}

// TestGeetestProviderLiveValidateEnvironment is an opt-in environment test.
// It uses a real browser-generated proof, which expires quickly, so it only
// runs when GEETEST_INTEGRATION=1 and all proof values are supplied.
func TestGeetestProviderLiveValidateEnvironment(t *testing.T) {
	if os.Getenv("GEETEST_INTEGRATION") != "1" {
		t.Skip("set GEETEST_INTEGRATION=1 and provide a fresh browser proof to run")
	}

	captchaID := strings.TrimSpace(os.Getenv("GEETEST_CAPTCHA_ID"))
	captchaKey := strings.TrimSpace(os.Getenv("GEETEST_CAPTCHA_KEY"))
	lotNumber := strings.TrimSpace(os.Getenv("GEETEST_LOT_NUMBER"))
	captchaOutput := strings.TrimSpace(os.Getenv("GEETEST_CAPTCHA_OUTPUT"))
	passToken := strings.TrimSpace(os.Getenv("GEETEST_PASS_TOKEN"))
	genTime := strings.TrimSpace(os.Getenv("GEETEST_GEN_TIME"))
	require.NotEmpty(t, captchaID, "GEETEST_CAPTCHA_ID is required")
	require.NotEmpty(t, captchaKey, "GEETEST_CAPTCHA_KEY is required")
	require.NotEmpty(t, lotNumber, "GEETEST_LOT_NUMBER is required")
	require.NotEmpty(t, captchaOutput, "GEETEST_CAPTCHA_OUTPUT is required")
	require.NotEmpty(t, passToken, "GEETEST_PASS_TOKEN is required")
	require.NotEmpty(t, genTime, "GEETEST_GEN_TIME is required")

	config := &model.CaptchaConfig{
		Provider: GeetestProviderType,
		Params: datatypes.JSONMap{
			"captchaId":  captchaID,
			"captchaKey": captchaKey,
		},
	}
	result, err := (&GeetestCaptchaProvider{}).Verify(config, map[string]string{
		"lot_number":     lotNumber,
		"captcha_output": captchaOutput,
		"pass_token":     passToken,
		"gen_time":       genTime,
	}, "")
	require.NoError(t, err)
	require.True(t, result.Success, result.Message)
}
