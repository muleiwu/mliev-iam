package captcha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func geetestBypassServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":       "success",
			"reason":       "bypass status",
			"status":       "success",
			"captcha_args": map[string]any{},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

// 极验后台把 Captcha ID 设为 bypass 时，任意凭证都会返回 success。
// 默认必须拒绝，否则验证码可被完全跳过。
func TestGeetestProviderRejectsVendorBypassByDefault(t *testing.T) {
	server := geetestBypassServer(t)
	provider := &GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}

	_, err := provider.Verify(geetestTestConfig(), geetestTestData(), "")
	require.ErrorContains(t, err, "测试绕过")
}

func TestGeetestProviderAllowsVendorBypassOnlyWhenExplicitlyEnabled(t *testing.T) {
	server := geetestBypassServer(t)
	config := geetestTestConfig()
	config.Params["allowBypass"] = "true"

	result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(config, geetestTestData(), "")
	require.NoError(t, err)
	require.True(t, result.Success)
}

// 后台开关提交的是布尔值，必须真正生效（此前只认字符串，开关形同虚设）。
func TestGeetestBypassAcceptsBooleanSwitch(t *testing.T) {
	server := geetestBypassServer(t)
	config := geetestTestConfig()
	config.Params["allowBypass"] = true

	result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(config, geetestTestData(), "")
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestGeetestBypassRejectsFalseBooleanSwitch(t *testing.T) {
	server := geetestBypassServer(t)
	config := geetestTestConfig()
	config.Params["allowBypass"] = false

	_, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(config, geetestTestData(), "")
	require.ErrorContains(t, err, "测试绕过")
}
