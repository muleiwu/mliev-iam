package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const (
	// TencentProviderType 腾讯云验证码提供商类型
	TencentProviderType = "tencent"
	// TencentAPIHost 腾讯云验证码API域名
	TencentAPIHost = "captcha.tencentcloudapi.com"
	// TencentAPIVersion 腾讯云API版本
	TencentAPIVersion = "2019-07-22"
	// TencentAPIAction 腾讯云验证码验证接口
	TencentAPIAction = "DescribeCaptchaResult"
)

// TencentCaptchaProvider 腾讯云验证码提供商实现
type TencentCaptchaProvider struct{}

// GetProviderType 返回提供商类型
func (t *TencentCaptchaProvider) GetProviderType() string {
	return TencentProviderType
}

// GetProviderInfo 获取腾讯云验证码提供商信息
func (t *TencentCaptchaProvider) GetProviderInfo() dto.CaptchaProviderInfo {
	return dto.CaptchaProviderInfo{
		Provider: "tencent",
		Label:    "腾讯云验证码",
		Fields: []dto.CaptchaProviderField{
			{
				Key:         "secretId",
				Label:       "腾讯云SecretId",
				Type:        "text",
				Required:    true,
				Placeholder: "请输入腾讯云SecretId",
			},
			{
				Key:         "secretKey",
				Label:       "腾讯云SecretKey",
				Type:        "password",
				Required:    true,
				Placeholder: "请输入腾讯云SecretKey",
			},
			{
				Key:         "captchaAppId",
				Label:       "验证码AppId",
				Type:        "text",
				Required:    true,
				Placeholder: "请输入验证码AppId",
			},
			{
				Key:         "appSecretKey",
				Label:       "验证码AppSecretKey",
				Type:        "password",
				Required:    true,
				Placeholder: "请输入验证码AppSecretKey",
			},
		},
	}
}

// GetClientParams 获取前端初始化参数
func (t *TencentCaptchaProvider) GetClientParams(config *model.CaptchaConfig) map[string]interface{} {
	// 腾讯云前端只需要 CaptchaAppId
	captchaAppID := ""
	if config.Params != nil {
		if val, ok := config.Params["captchaAppId"]; ok {
			if str, ok := val.(string); ok {
				captchaAppID = str
			}
		}
	}

	return map[string]interface{}{
		"captchaAppId": captchaAppID,
	}
}

// Verify 验证腾讯云验证码ticket
func (t *TencentCaptchaProvider) Verify(config *model.CaptchaConfig, ticketData map[string]string) (*VerifyResult, error) {
	// 从配置中获取必要参数
	secretID, secretKey, appSecretKey, captchaAppID, err := t.getConfigParams(config)
	if err != nil {
		return &VerifyResult{Success: false, Message: err.Error()}, err
	}

	// 从ticketData中获取ticket和randstr
	ticket, ok := ticketData["ticket"]
	if !ok || ticket == "" {
		return &VerifyResult{Success: false, Message: "ticket不能为空"}, fmt.Errorf("ticket不能为空")
	}

	randstr, ok := ticketData["randstr"]
	if !ok || randstr == "" {
		return &VerifyResult{Success: false, Message: "randstr不能为空"}, fmt.Errorf("randstr不能为空")
	}

	userIP := ticketData["userIp"]
	if userIP == "" {
		userIP = "127.0.0.1" // 默认值
	}

	// 调用腾讯云API验证
	success, msg, err := t.callTencentAPI(secretID, secretKey, appSecretKey, captchaAppID, ticket, randstr, userIP)
	if err != nil {
		return &VerifyResult{Success: false, Message: fmt.Sprintf("验证码验证失败: %s", err.Error())}, err
	}

	return &VerifyResult{Success: success, Message: msg}, nil
}

// getConfigParams 从配置中提取参数
func (t *TencentCaptchaProvider) getConfigParams(config *model.CaptchaConfig) (secretID, secretKey, appSecretKey, captchaAppID string, err error) {
	if config.Params == nil {
		return "", "", "", "", fmt.Errorf("验证码配置参数为空")
	}

	secretID, _ = config.Params["secretId"].(string)
	secretKey, _ = config.Params["secretKey"].(string)
	appSecretKey, _ = config.Params["appSecretKey"].(string)
	captchaAppID, _ = config.Params["captchaAppId"].(string)

	if secretID == "" || secretKey == "" || appSecretKey == "" || captchaAppID == "" {
		return "", "", "", "", fmt.Errorf("腾讯云验证码配置不完整")
	}

	return secretID, secretKey, appSecretKey, captchaAppID, nil
}

// callTencentAPI 调用腾讯云API进行验证
func (t *TencentCaptchaProvider) callTencentAPI(secretID, secretKey, appSecretKey, captchaAppID, ticket, randstr, userIP string) (bool, string, error) {
	timestamp := time.Now().Unix()

	// 构建请求参数
	params := map[string]string{
		"CaptchaType":  "9",
		"Ticket":       ticket,
		"Randstr":      randstr,
		"UserIp":       userIP,
		"CaptchaAppId": captchaAppID,
		"AppSecretKey": appSecretKey,
	}

	// 构建规范请求字符串
	canonicalQueryString := t.buildCanonicalQueryString(params)
	canonicalHeaders := "content-type:application/x-www-form-urlencoded\nhost:" + TencentAPIHost + "\n"
	signedHeaders := "content-type;host"
	payload := canonicalQueryString
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s", canonicalHeaders, signedHeaders, t.sha256hex(payload))

	// 构建待签名字符串
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/captcha/tc3_request", date)
	hashedCanonicalRequest := t.sha256hex(canonicalRequest)
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s", timestamp, credentialScope, hashedCanonicalRequest)

	// 计算签名
	secretDate := t.hmacSHA256([]byte("TC3"+secretKey), []byte(date))
	secretService := t.hmacSHA256(secretDate, []byte("captcha"))
	secretSigning := t.hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(t.hmacSHA256(secretSigning, []byte(stringToSign)))

	// 构建Authorization
	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credentialScope, signedHeaders, signature)

	// 发送HTTP请求
	apiURL := "https://" + TencentAPIHost
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(canonicalQueryString))
	if err != nil {
		return false, "创建请求失败", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Host", TencentAPIHost)
	req.Header.Set("X-TC-Action", TencentAPIAction)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-TC-Version", TencentAPIVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "请求腾讯云API失败", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "读取响应失败", err
	}

	// 解析响应
	var result struct {
		Response struct {
			CaptchaCode int    `json:"CaptchaCode"`
			CaptchaMsg  string `json:"CaptchaMsg"`
			EvilLevel   int    `json:"EvilLevel"`
			RequestID   string `json:"RequestId"`
			Error       struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, "解析响应失败", err
	}

	// 检查是否有错误
	if result.Response.Error.Code != "" {
		return false, result.Response.Error.Message, fmt.Errorf("腾讯云API错误: %s", result.Response.Error.Message)
	}

	// CaptchaCode为1表示验证成功
	if result.Response.CaptchaCode == 1 {
		return true, "验证成功", nil
	}

	return false, result.Response.CaptchaMsg, nil
}

// buildCanonicalQueryString 构建规范查询字符串
func (t *TencentCaptchaProvider) buildCanonicalQueryString(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	return strings.Join(queryParts, "&")
}

// sha256hex 计算SHA256哈希并返回十六进制字符串
func (t *TencentCaptchaProvider) sha256hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// hmacSHA256 计算HMAC-SHA256
func (t *TencentCaptchaProvider) hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
