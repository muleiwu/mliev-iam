package captcha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// geetestBypassServer 模拟极验后台把 Captcha ID 设为 bypass 时返回的真实形态：
// status=bypass、result=fail。此前测试桩用的是 success/success，只靠 reason
// 文案区分，既不能证明 bypass 分支会命中，也无法发现「开关打开后仍被严格
// 判定拦下」的缺陷。
func geetestBypassServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "bypass",
			"result":       "fail",
			"reason":       "captcha bypass enabled",
			"captcha_args": map[string]any{},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

// 极验后台把 Captcha ID 设为 bypass 时，任意凭证都会返回 bypass。
// 默认必须拒绝，否则验证码可被完全跳过。
func TestGeetestProviderRejectsVendorBypassByDefault(t *testing.T) {
	server := geetestBypassServer(t)
	provider := &GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}

	_, err := provider.Verify(geetestTestConfig(), geetestTestData(), "")
	require.ErrorContains(t, err, "测试绕过")
}

// 后台开关打开后，bypass 响应必须直接视为通过。
// 该响应的 result 是 fail，若继续走严格判定，开关将形同虚设。
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

// result=bypass 同样属于放行标记，不能因为 status 是 success 就漏判。
func TestGeetestProviderDetectsBypassOnResultField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"result": "bypass",
			"reason": "bypass",
		})
	}))
	defer server.Close()

	_, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(geetestTestConfig(), geetestTestData(), "")
	require.ErrorContains(t, err, "测试绕过")
}

// 放行状态下极验也可能返回 status=error；bypass 判定必须先于 error 分支，
// 否则管理员打开 allowBypass 后仍会被「服务暂不可用」拒绝，开关形同虚设。
func TestGeetestProviderBypassTakesPrecedenceOverErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"result": "bypass",
			"code":   "-50305",
			"msg":    "bypass mode",
		})
	}))
	defer server.Close()

	t.Run("default rejects", func(t *testing.T) {
		_, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(geetestTestConfig(), geetestTestData(), "")
		require.ErrorContains(t, err, "测试绕过")
	})

	t.Run("enabled passes", func(t *testing.T) {
		config := geetestTestConfig()
		config.Params["allowBypass"] = true
		result, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(config, geetestTestData(), "")
		require.NoError(t, err)
		require.True(t, result.Success)
	})
}

// 非 bypass 的 error 响应仍然必须报服务异常，不能被当作通过或普通失败。
func TestGeetestProviderKeepsErrorBranchWithoutBypass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "result": "fail", "code": "-50005"})
	}))
	defer server.Close()

	config := geetestTestConfig()
	config.Params["allowBypass"] = true
	_, err := (&GeetestCaptchaProvider{client: server.Client(), validateURL: server.URL}).Verify(config, geetestTestData(), "")
	require.ErrorContains(t, err, "-50005")
}
