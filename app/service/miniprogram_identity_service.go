package service

import (
	"context"
	"errors"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"gorm.io/gorm"
)

// MiniProgramIdentity is returned only to an authenticated, allowlisted application.
// It never contains a WeChat session key or an IAM session token.
type MiniProgramIdentity struct {
	State  string                `json:"state"`
	Claims *dto.UserInfoResponse `json:"claims,omitempty"`
}

func (s *ExternalAuthService) ResolveMiniProgramIdentity(ctx context.Context, identifier, code, clientID, clientSecret string) (*MiniProgramIdentity, error) {
	apps := NewApplicationService(s.db)
	app, err := apps.ValidateApplication(clientID, clientSecret)
	if err != nil {
		return nil, errors.New("应用认证失败")
	}
	if app.ProtocolType != string(model.ProtocolTypeOIDC) || app.ProtocolConfig["clientType"] == "public" || clientSecret == "" {
		return nil, errors.New("身份交换仅允许机密 OIDC 应用")
	}
	connection, adapter, err := s.enabledAdapter(identifier)
	if err != nil {
		return nil, err
	}
	if connection.ProviderType != "wechat_miniprogram" {
		return nil, errors.New("连接不是微信小程序")
	}
	codeAdapter, ok := adapter.(protocol.CodeAuthenticationAdapter)
	if !ok {
		return nil, errors.New("连接不支持微信登录")
	}
	profile, err := codeAdapter.LoginWithCode(ctx, code)
	if err != nil {
		return nil, errors.New("微信登录凭证无效，请重试")
	}
	principal, err := s.findPrincipal(connection.ID, profile)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &MiniProgramIdentity{State: "needs_bind"}, nil
	}
	if err != nil {
		return nil, err
	}
	if principal == nil {
		return &MiniProgramIdentity{State: "needs_bind"}, nil
	}
	if principal.Status != model.PrincipalStatusActive {
		return nil, errors.New("微信身份已停用")
	}
	if principal.UserID == nil {
		return &MiniProgramIdentity{State: "needs_bind"}, nil
	}
	user, err := s.userDAO.GetByID(*principal.UserID)
	if err != nil || user == nil || !user.CanLogin() {
		return nil, errors.New("账户不可登录")
	}
	if !apps.CheckUserAccess(app.ID, user.ID) {
		return nil, errors.New("没有访问此应用的权限")
	}
	if err := s.refreshPrincipal(principal, profile, true); err != nil {
		return nil, err
	}
	claims := NewOIDCUserInfoService(s.db).BuildUserInfoResponse(user, []string{"openid", "profile", "phone"})
	return &MiniProgramIdentity{State: "authenticated", Claims: claims}, nil
}
