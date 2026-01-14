package captcha

import (
	"fmt"
	"strconv"

	captcha "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/captcha/v20190722"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

const (
	// TencentProviderType 腾讯云验证码提供商类型
	TencentProviderType = "tencent"
	// TencentAPIHost 腾讯云验证码API域名
	TencentAPIHost = "captcha.tencentcloudapi.com"
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
func (t *TencentCaptchaProvider) Verify(config *model.CaptchaConfig, ticketData map[string]string, userIP string) (*VerifyResult, error) {
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

	// userIP 由调用方传入，如果为空则使用默认值
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
	// 创建腾讯云凭证
	credential := common.NewCredential(secretID, secretKey)

	// 配置客户端
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = TencentAPIHost

	// 创建验证码客户端
	client, err := captcha.NewClient(credential, "", cpf)
	if err != nil {
		return false, "创建客户端失败", err
	}

	// 将captchaAppID转换为uint64
	appIDInt, err := strconv.ParseUint(captchaAppID, 10, 64)
	if err != nil {
		return false, "验证码AppId格式错误", err
	}

	// 构建验证请求
	request := captcha.NewDescribeCaptchaResultRequest()
	request.CaptchaType = common.Uint64Ptr(9)
	request.CaptchaAppId = common.Uint64Ptr(appIDInt)
	request.AppSecretKey = common.StringPtr(appSecretKey)
	request.Randstr = common.StringPtr(randstr)
	request.Ticket = common.StringPtr(ticket)
	request.UserIp = common.StringPtr(userIP)

	// 发送验证请求
	response, err := client.DescribeCaptchaResult(request)
	if err != nil {
		return false, "验证码验证失败", err
	}

	// 检查响应是否为空
	if response == nil || response.Response == nil {
		return false, "验证响应为空", fmt.Errorf("验证响应为空")
	}

	// 验证码验证成功判断: CaptchaCode为1表示验证成功
	if response.Response.CaptchaCode == nil || *response.Response.CaptchaCode != 1 {
		msg := "验证码验证失败"
		if response.Response.CaptchaMsg != nil {
			msg = *response.Response.CaptchaMsg
		}
		return false, msg, nil
	}

	// EvilLevel检查：参考实现的安全检查，EvilLevel大于0表示可能存在安全风险
	if response.Response.EvilLevel != nil && *response.Response.EvilLevel > 0 {
		return false, "可能存在安全风险", nil
	}

	return true, "验证成功", nil
}
