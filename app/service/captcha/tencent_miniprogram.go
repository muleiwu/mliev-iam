package captcha

import (
	"errors"
	"strconv"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	tencent "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/captcha/v20190722"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tchttp "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/http"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

const TencentMiniProgramProviderType = "tencent_miniprogram"

type TencentMiniProgramProvider struct {
	// Injectable signed transport for contract tests. Secrets never enter client parameters.
	send func(*model.CaptchaConfig, *miniCaptchaRequest, *miniCaptchaResponse) error
}

func (*TencentMiniProgramProvider) GetProviderType() string { return TencentMiniProgramProviderType }
func (*TencentMiniProgramProvider) GetProviderInfo() dto.CaptchaProviderInfo {
	info := (&TencentCaptchaProvider{}).GetProviderInfo()
	info.Provider = TencentMiniProgramProviderType
	info.Label = "腾讯云验证码（微信小程序）"
	info.Fields = append(info.Fields, dto.CaptchaProviderField{Key: "aidEncryptionEnabled", Label: "CaptchaAppId 加密校验", Type: "text", Placeholder: "true 开启，留空关闭"})
	return info
}
func (*TencentMiniProgramProvider) GetClientParams(config *model.CaptchaConfig) (map[string]interface{}, error) {
	return miniCaptchaPublicParams(config, time.Now())
}

// The pinned SDK predates this Action; embed its request/response bases and use
// the same signed Send transport without upgrading unrelated Tencent services.
type miniCaptchaRequest struct {
	*tchttp.BaseRequest
	CaptchaType  *uint64 `json:"CaptchaType,omitempty"`
	Ticket       *string `json:"Ticket,omitempty"`
	UserIp       *string `json:"UserIp,omitempty"`
	CaptchaAppId *uint64 `json:"CaptchaAppId,omitempty"`
	AppSecretKey *string `json:"AppSecretKey,omitempty"`
}
type miniCaptchaResponse struct {
	*tchttp.BaseResponse
	Response *struct {
		CaptchaCode *int64  `json:"CaptchaCode"`
		CaptchaMsg  *string `json:"CaptchaMsg"`
		RequestId   *string `json:"RequestId"`
	} `json:"Response"`
}

func (p *TencentMiniProgramProvider) Verify(config *model.CaptchaConfig, data map[string]string, ip string) (*VerifyResult, error) {
	secretID, secretKey, appSecret, appID, err := (&TencentCaptchaProvider{}).getConfigParams(config)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(appID, 10, 64)
	if err != nil || id == 0 {
		return nil, errors.New("小程序验证码 AppId 无效")
	}
	if data["ticket"] == "" || len(data["ticket"]) > 8192 || ip == "" {
		return nil, errors.New("小程序验证码票据或请求信息无效")
	}
	req := &miniCaptchaRequest{BaseRequest: &tchttp.BaseRequest{}, CaptchaType: common.Uint64Ptr(9), Ticket: common.StringPtr(data["ticket"]), UserIp: common.StringPtr(ip), CaptchaAppId: &id, AppSecretKey: &appSecret}
	req.Init().WithApiInfo("captcha", "2019-07-22", "DescribeCaptchaMiniResult")
	response := &miniCaptchaResponse{BaseResponse: &tchttp.BaseResponse{}}
	if p.send != nil {
		err = p.send(config, req, response)
	} else {
		cfg := profile.NewClientProfile()
		cfg.HttpProfile.Endpoint = TencentAPIHost
		cfg.HttpProfile.ReqTimeout = 10
		client, createErr := tencent.NewClient(common.NewCredential(secretID, secretKey), "", cfg)
		if createErr != nil {
			return nil, errors.New("小程序验证码服务不可用")
		}
		err = client.Send(req, response)
	}
	if err != nil {
		return nil, errors.New("小程序验证码校验请求失败，请重新验证")
	}
	if response.Response == nil || response.Response.CaptchaCode == nil || *response.Response.CaptchaCode != 1 {
		return &VerifyResult{Success: false, Message: "小程序验证码未通过，请重新验证"}, nil
	}
	return &VerifyResult{Success: true, Message: "验证成功"}, nil
}

func miniCaptchaPublicParams(config *model.CaptchaConfig, now time.Time) (map[string]interface{}, error) {
	_, _, secret, appID, err := (&TencentCaptchaProvider{}).getConfigParams(config)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(appID, 10, 64)
	if err != nil || id == 0 {
		return nil, errors.New("小程序验证码 AppId 无效")
	}
	result := map[string]interface{}{"captchaAppId": appID}
	enabled := config.Params["aidEncryptionEnabled"] == true || config.Params["aidEncryptionEnabled"] == "true"
	if !enabled {
		return result, nil
	}
	return encryptedTencentClientParams(appID, secret, now)
}
