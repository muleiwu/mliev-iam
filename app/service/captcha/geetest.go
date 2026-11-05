package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/muleiwu/gsr"
)

const (
	// GeetestProviderType 极验验证 4.0 提供商类型
	GeetestProviderType = "geetest"
	// GeetestValidateURL 极验验证 4.0 服务端校验地址
	GeetestValidateURL = "https://gcaptcha4.geetest.com/validate"
)

// GeetestCaptchaProvider 极验验证 4.0 提供商实现
type GeetestCaptchaProvider struct {
	client      *http.Client
	validateURL string
	logf        func(format string, args ...any)
}

// GetProviderType 返回提供商类型
func (*GeetestCaptchaProvider) GetProviderType() string {
	return GeetestProviderType
}

// GetProviderInfo 获取极验验证提供商信息
func (*GeetestCaptchaProvider) GetProviderInfo() dto.CaptchaProviderInfo {
	return dto.CaptchaProviderInfo{
		Provider: GeetestProviderType,
		Label:    "极验验证 4.0",
		Fields: []dto.CaptchaProviderField{
			{
				Key:         "captchaId",
				Label:       "Captcha ID",
				Type:        "text",
				Required:    true,
				Placeholder: "请输入极验后台的 Captcha ID",
			},
			{
				Key:         "captchaKey",
				Label:       "Captcha Key",
				Type:        "password",
				Required:    true,
				Placeholder: "请输入极验后台的 Captcha Key",
			},
			{
				Key:         "allowBypass",
				Label:       "接受极验测试绕过",
				Type:        "boolean",
				Required:    false,
				Placeholder: "开启后极验返回 bypass status 时视为通过，仅建议测试环境使用",
			},
		},
	}
}

// GetClientParams 返回 Web 初始化参数。captchaKey 绝不能返回给客户端。
func (*GeetestCaptchaProvider) GetClientParams(config *model.CaptchaConfig) map[string]interface{} {
	params := map[string]interface{}{
		"captchaId": "",
		"product":   "bind",
	}
	if config != nil && config.Params != nil {
		if value, ok := config.Params["captchaId"].(string); ok {
			params["captchaId"] = value
		}
	}
	return params
}

// Verify 调用极验 /validate 完成服务端二次校验。
func (p *GeetestCaptchaProvider) Verify(config *model.CaptchaConfig, data map[string]string, _ string) (*VerifyResult, error) {
	if p.logf == nil {
		if logger := geetestLogger(); logger != nil {
			p.logf = func(format string, args ...any) { logger.Info(fmt.Sprintf(format, args...)) }
		}
	}
	captchaID, captchaKey, err := geetestConfigParams(config)
	if err != nil {
		return nil, err
	}

	lotNumber := strings.TrimSpace(data["lot_number"])
	captchaOutput := strings.TrimSpace(data["captcha_output"])
	passToken := strings.TrimSpace(data["pass_token"])
	genTime := strings.TrimSpace(data["gen_time"])
	if lotNumber == "" || captchaOutput == "" || passToken == "" || genTime == "" {
		return &VerifyResult{Success: false, Message: "验证码参数不完整，请重新验证"}, errors.New("极验验证参数不完整")
	}
	if len(lotNumber) > 128 || len(captchaOutput) > 8192 || len(passToken) > 8192 || len(genTime) > 32 {
		return &VerifyResult{Success: false, Message: "验证码参数无效，请重新验证"}, errors.New("极验验证参数长度无效")
	}
	if _, err := strconv.ParseInt(genTime, 10, 64); err != nil {
		return &VerifyResult{Success: false, Message: "验证码参数无效，请重新验证"}, errors.New("极验 gen_time 无效")
	}

	signature := geetestSignToken(captchaKey, lotNumber)
	form := url.Values{
		"captcha_id":     {captchaID},
		"lot_number":     {lotNumber},
		"captcha_output": {captchaOutput},
		"pass_token":     {passToken},
		"gen_time":       {genTime},
		"sign_token":     {signature},
	}

	endpoint := strings.TrimSpace(p.validateURL)
	if endpoint == "" {
		endpoint = GeetestValidateURL
	}
	requestURL := strings.TrimRight(endpoint, "/") + "?captcha_id=" + url.QueryEscape(captchaID)
	request, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("验证码服务暂不可用，请稍后重试")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("验证码服务暂不可用，请稍后重试")
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, errors.New("验证码服务暂不可用，请稍后重试")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("验证码服务暂不可用，请稍后重试（HTTP %d）", response.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
		Code   string `json:"code"`
		Msg    string `json:"msg"`
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("验证码服务响应异常，请稍后重试")
	}
	// 记录原始判定字段，便于排查 bypass、签名或凭证问题（不含 captchaKey）。
	status := strings.TrimSpace(result.Status)
	verdict := strings.TrimSpace(result.Result)
	if p.logf != nil {
		p.logf("[geetest] validate captchaId=%s status=%s result=%s reason=%s code=%s msg=%s",
			captchaID, status, verdict, result.Reason, result.Code, result.Msg)
	}
	// 极验后台可把 Captcha ID 设为 bypass；此时校验结果不代表用户真实完成验证。
	// 只认官方字段的精确取值（status 或 result 为 bypass），不对 reason 做子串匹配：
	// reason 是对方案文案，改一句措辞就会让判定翻转，既可能误拦正常用户，
	// 也可能放过真正的放行响应，判定必须由结构化字段决定。
	//
	// bypass 必须先于 error 判定：放行状态下极验同样可能返回带 error 的组合，
	// 若先因 status=error 直接返回，管理员打开 allowBypass 后仍会被拒绝，开关形同虚设。
	if geetestBypassed(status, verdict) {
		if !geetestBypassAllowed(config) {
			return nil, errors.New("验证码服务处于测试绕过状态，请联系管理员配置生产 Captcha ID")
		}
		// 管理员显式开启后才接受。bypass 响应的 result 往往是 fail，
		// 不能继续走下面的严格判定，否则开关形同虚设。
		return &VerifyResult{Success: true, Message: "验证成功（极验测试绕过）"}, nil
	}
	if strings.EqualFold(status, "error") {
		return nil, fmt.Errorf("验证码服务暂不可用，请稍后重试（%s）", geetestErrorCode(result.Code))
	}
	// status 表示极验服务调用本身是否成功，result 才是本次校验结论。
	// 只有两者同时为 success 才算通过；{"status":"error","result":"success"}
	// 这类组合绝不能被当成通过，否则验证码可被完全跳过。
	if !geetestPassed(status, verdict) {
		return &VerifyResult{Success: false, Message: geetestFailureMessage(result.Reason)}, nil
	}
	return &VerifyResult{Success: true, Message: "验证成功"}, nil
}

// geetestPassed 仅当 status 与 result 同时为 success 时才判定通过。
func geetestPassed(status, result string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "success") &&
		strings.EqualFold(strings.TrimSpace(result), "success")
}

// geetestBypassed 检测极验是否放弃校验（测试绕过）。
// 只对官方结构化字段做精确匹配：status 或 result 等于 bypass。
// 不对 reason 做子串匹配，避免判定被对方文案措辞左右。
func geetestBypassed(status, result string) bool {
	for _, field := range []string{status, result} {
		if strings.EqualFold(strings.TrimSpace(field), "bypass") {
			return true
		}
	}
	return false
}

// geetestLogger 在容器就绪时返回全局 logger；单元测试等无容器环境返回 nil。
func geetestLogger() (logger gsr.Logger) {
	defer func() {
		if recover() != nil {
			logger = nil
		}
	}()
	return helper.GetLogger()
}

func geetestBypassAllowed(config *model.CaptchaConfig) bool {
	if config == nil || config.Params == nil {
		return false
	}
	// 后台开关提交的是布尔值，同时兼容历史字符串配置。
	switch value := config.Params["allowBypass"].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func geetestConfigParams(config *model.CaptchaConfig) (string, string, error) {
	if config == nil || config.Params == nil {
		return "", "", errors.New("极验验证配置参数为空")
	}
	captchaID, _ := config.Params["captchaId"].(string)
	captchaKey, _ := config.Params["captchaKey"].(string)
	captchaID = strings.TrimSpace(captchaID)
	captchaKey = strings.TrimSpace(captchaKey)
	if captchaID == "" || captchaKey == "" {
		return "", "", errors.New("极验验证配置不完整")
	}
	return captchaID, captchaKey, nil
}

func geetestSignToken(captchaKey, lotNumber string) string {
	mac := hmac.New(sha256.New, []byte(captchaKey))
	_, _ = mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}

func geetestErrorCode(code string) string {
	if code == "" {
		return "unknown"
	}
	return code
}

func geetestFailureMessage(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if strings.Contains(reason, "expire") || strings.Contains(reason, "invalid") {
		return "验证已过期，请重新验证"
	}
	return "验证失败，请重新验证"
}
