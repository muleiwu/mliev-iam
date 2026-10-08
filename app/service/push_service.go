package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	mlievpush "github.com/mliev-sdk/push-go"
)

const verificationCodeExpiry = 5 * time.Minute

// PushError separates invalid configuration from upstream failures.
type PushError struct {
	Status  int
	Message string
}

func (e *PushError) Error() string { return e.Message }

func invalidPushConfig(message string) error {
	return &PushError{Status: 400, Message: message}
}

type PushService struct {
	client      *mlievpush.Client
	credentials dto.PushCredentials
}

func NewPushService(credentials dto.PushCredentials, opts ...mlievpush.ClientOption) (*PushService, error) {
	credentials.BaseURL = strings.TrimRight(strings.TrimSpace(credentials.BaseURL), "/")
	credentials.AppID = strings.TrimSpace(credentials.AppID)
	u, err := url.Parse(credentials.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, invalidPushConfig("请输入有效的 HTTP 或 HTTPS 推送服务地址")
	}
	if credentials.AppID == "" || strings.TrimSpace(credentials.AppSecret) == "" {
		return nil, invalidPushConfig("请填写推送应用 ID 和应用密钥")
	}
	return &PushService{
		client:      mlievpush.NewClient(credentials.BaseURL, credentials.AppID, credentials.AppSecret, opts...),
		credentials: credentials,
	}, nil
}

// upstreamError never exposes a request URL or credentials in network errors.
func (s *PushService) upstreamError(err error) error {
	var apiErr *mlievpush.APIError
	if errors.As(err, &apiErr) {
		message := strings.ReplaceAll(apiErr.Message, s.credentials.AppSecret, "***")
		message = strings.ReplaceAll(message, s.credentials.AppID, "***")
		return &PushError{Status: 502, Message: fmt.Sprintf("推送服务错误 [%d]：%s", apiErr.Code, message)}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return &PushError{Status: 504, Message: "推送服务请求超时，请重试"}
	}
	return &PushError{Status: 502, Message: "无法读取推送服务响应，请检查服务地址、网络及服务状态"}
}

func validPushType(messageType string) bool {
	return messageType == mlievpush.MessageTypeSMS || messageType == mlievpush.MessageTypeEmail
}

func convertPushChannel(channel mlievpush.ChannelData) dto.PushChannel {
	return dto.PushChannel{
		ID: channel.ID, Name: channel.Name, Type: channel.Type,
		MessageTemplateID: channel.MessageTemplateID, TemplateName: channel.TemplateName,
		Readiness: dto.PushChannelReadiness{State: channel.Readiness.State, BlockerCodes: channel.Readiness.BlockerCodes},
	}
}

func (s *PushService) ListChannels(ctx context.Context, req dto.PushChannelsQueryRequest) (*dto.PushChannelsPage, error) {
	if !validPushType(req.Type) || req.Page < 0 || req.PageSize < 0 || req.PageSize > 100 {
		return nil, invalidPushConfig("通道类型须为 sms 或 email，页码须为正数，每页最多 100 条")
	}
	page, err := s.client.ListChannels(ctx, &mlievpush.ListChannelsRequest{Type: req.Type, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		return nil, s.upstreamError(err)
	}
	result := &dto.PushChannelsPage{Items: make([]dto.PushChannel, 0, len(page.Items)), Total: page.Total, Page: page.Page, Size: page.Size}
	for _, channel := range page.Items {
		result.Items = append(result.Items, convertPushChannel(channel))
	}
	return result, nil
}

func (s *PushService) GetChannel(ctx context.Context, id int) (*dto.PushChannelDetail, error) {
	if id <= 0 {
		return nil, invalidPushConfig("通道 ID 须为正数")
	}
	detail, err := s.client.GetChannel(ctx, id)
	if err != nil {
		return nil, s.upstreamError(err)
	}
	result := &dto.PushChannelDetail{
		PushChannel:       convertPushChannel(detail.ChannelData),
		SignatureRequired: detail.SignatureRequired, SignatureNames: detail.SignatureNames,
	}
	if detail.Template != nil {
		result.Template = &dto.PushChannelTemplate{
			ID: detail.Template.ID, TemplateName: detail.Template.TemplateName,
			ContentType: detail.Template.ContentType, Content: detail.Template.Content,
			Variables: detail.Template.Variables, Description: detail.Template.Description,
		}
	}
	return result, nil
}

func legacyPushBindings() map[string]dto.PushTemplateBinding {
	return map[string]dto.PushTemplateBinding{"code": {Source: "code"}, "expire_time": {Source: "expire_minutes"}}
}

// BuildPushTemplateParams is shared by login codes and isolated test messages.
// nil bindings preserve the sending contract of configurations saved before v1.2.
func BuildPushTemplateParams(bindings map[string]dto.PushTemplateBinding, code string, expiry time.Duration) (map[string]string, error) {
	if bindings == nil {
		bindings = legacyPushBindings()
	}
	params := make(map[string]string, len(bindings))
	hasCode := false
	for name, binding := range bindings {
		if strings.TrimSpace(name) == "" {
			return nil, invalidPushConfig("模板变量名不能为空")
		}
		switch binding.Source {
		case "code":
			params[name] = code
			hasCode = true
		case "expire_minutes":
			params[name] = strconv.FormatInt(int64(expiry/time.Minute), 10)
		case "constant":
			params[name] = binding.Value
		default:
			return nil, invalidPushConfig(fmt.Sprintf("模板变量 %s 尚未绑定有效数据来源", name))
		}
	}
	if !hasCode {
		return nil, invalidPushConfig("验证码模板至少需要一个变量绑定验证码")
	}
	return params, nil
}

func pushChannelSettings(config dto.PushSettings, messageType string) (int, string, map[string]dto.PushTemplateBinding) {
	if messageType == mlievpush.MessageTypeSMS {
		return config.SMSChannelID, config.SMSSignature, config.SMSTemplateBindings
	}
	return config.EmailChannelID, config.EmailSignature, config.EmailTemplateBindings
}

func (s *PushService) ValidateChannel(ctx context.Context, config dto.PushSettings, messageType string) error {
	if !validPushType(messageType) {
		return invalidPushConfig("消息类型须为 sms 或 email")
	}
	id, signature, bindings := pushChannelSettings(config, messageType)
	detail, err := s.GetChannel(ctx, id)
	if err != nil {
		return err
	}
	if detail.ID != id || detail.Type != messageType {
		return invalidPushConfig("所选通道与消息类型不匹配")
	}
	if detail.Readiness.State != mlievpush.ChannelReadinessReady && detail.Readiness.State != mlievpush.ChannelReadinessDegraded {
		return invalidPushConfig("所选通道不可用：" + strings.Join(detail.Readiness.BlockerCodes, "、"))
	}
	if detail.Template == nil || detail.Template.Variables == nil {
		return invalidPushConfig("所选通道的模板缺失或变量配置无效")
	}
	if (detail.SignatureRequired && signature == "") || (signature != "" && !slices.Contains(detail.SignatureNames, signature)) {
		return invalidPushConfig("请选择通道提供的短信签名或邮件标题")
	}
	if _, err := BuildPushTemplateParams(bindings, "000000", verificationCodeExpiry); err != nil {
		return err
	}
	if bindings == nil {
		bindings = legacyPushBindings()
	}
	hasCode := false
	for _, name := range detail.Template.Variables {
		binding, ok := bindings[name]
		if !ok {
			return invalidPushConfig(fmt.Sprintf("模板变量 %s 尚未绑定数据来源", name))
		}
		hasCode = hasCode || binding.Source == "code"
	}
	if !hasCode {
		return invalidPushConfig("模板实际使用的变量中至少一个须绑定验证码")
	}
	return nil
}

func (s *PushService) SendCode(ctx context.Context, id int, signature, receiver string, bindings map[string]dto.PushTemplateBinding, code string, expiry time.Duration) (*dto.PushTestResponse, error) {
	params, err := BuildPushTemplateParams(bindings, code, expiry)
	if err != nil {
		return nil, err
	}
	data, err := s.client.SendMessage(ctx, &mlievpush.SendMessageRequest{ChannelID: id, SignatureName: signature, Receiver: receiver, TemplateParams: params})
	if err != nil {
		return nil, s.upstreamError(err)
	}
	return &dto.PushTestResponse{TaskID: data.TaskID, Status: data.Status, CreatedAt: data.CreatedAt}, nil
}

// Test sends a standalone message without reading or writing login-code Redis keys.
func (s *PushService) Test(ctx context.Context, req dto.PushTestRequest) (*dto.PushTestResponse, error) {
	if !validPushType(req.Type) {
		return nil, invalidPushConfig("消息类型须为 sms 或 email")
	}
	receiver := strings.TrimSpace(req.Receiver)
	if req.Type == mlievpush.MessageTypeSMS {
		if _, err := utils.NormalizePhoneToE164(receiver, "CN"); err != nil {
			return nil, invalidPushConfig("请输入有效的测试手机号")
		}
	} else if address, err := mail.ParseAddress(receiver); err != nil || address.Address != receiver {
		return nil, invalidPushConfig("请输入有效的测试邮箱")
	}
	if err := s.ValidateChannel(ctx, req.Config, req.Type); err != nil {
		return nil, err
	}
	code, err := generateVerificationCode()
	if err != nil {
		return nil, err
	}
	id, signature, bindings := pushChannelSettings(req.Config, req.Type)
	return s.SendCode(ctx, id, signature, receiver, bindings, code, verificationCodeExpiry)
}
