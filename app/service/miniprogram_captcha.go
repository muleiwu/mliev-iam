package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"context"
	"errors"
	"gorm.io/gorm"
)

// The captcha package depends on service settings; injection keeps the native
// authentication service independent of provider implementations.
type MiniProgramCaptcha interface {
	Params(context.Context, *model.Application, string, dto.MiniProgramCaptchaParamsRequest, string) (*dto.CaptchaClientParamsResponse, error)
	Validate(context.Context, *model.Application, string, *dto.CaptchaTicket, string) error
}

func (s *MiniProgramService) SetCaptcha(c MiniProgramCaptcha) { s.captcha = c }
func ResolveMiniProgramCaptchaConfigID(db *gorm.DB, app *model.Application) (uint, error) {
	cfg, err := model.DecodeMiniProgramConfig(app.ProtocolConfig)
	if err != nil {
		return 0, err
	}
	if cfg.CaptchaConfigID != nil {
		return *cfg.CaptchaConfigID, nil
	}
	bindings, err := NewSettingService(db).GetCaptchaSceneBindings()
	if err != nil {
		return 0, err
	}
	return bindings.Login, nil
}
func (s *MiniProgramService) CaptchaParams(ctx context.Context, clientID, token string, req dto.MiniProgramCaptchaParamsRequest, ip string) (*dto.CaptchaClientParamsResponse, error) {
	var response *dto.CaptchaClientParamsResponse
	_, err := s.withTransaction(ctx, clientID, token, func(tr *miniTransaction, app *miniApplication) (*dto.MiniProgramResponse, error) {
		if !miniCanBind(tr) {
			return nil, ErrMiniTransaction
		}
		if s.captcha == nil {
			return nil, errors.New("验证码服务未初始化")
		}
		var err error
		response, err = s.captcha.Params(ctx, app.app, miniHash(clientID, token), req, ip)
		return nil, err
	})
	return response, err
}
func miniCanBind(tr *miniTransaction) bool {
	return tr.UserID == 0 && (tr.State == "bind_required" || tr.State == "phone_required" || tr.State == "profile_required")
}
