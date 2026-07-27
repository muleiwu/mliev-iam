package controller

import (
	"context"
	"fmt"
	"net/url"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

const (
	oidcAccessDeniedCode        = "access_denied"
	oidcAccessDeniedDescription = "您没有访问此应用的权限"
)

func buildOIDCErrorRedirectURL(redirectURI, code, description, state string) (string, error) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("解析重定向 URI 失败: %w", err)
	}

	query := target.Query()
	query.Set("error", code)
	if description != "" {
		query.Set("error_description", description)
	}
	if state != "" {
		query.Set("state", state)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func buildOIDCSuccessRedirectURL(redirectURI, code, state string) (string, error) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("解析重定向 URI 失败: %w", err)
	}

	query := target.Query()
	query.Set("code", code)
	if state != "" {
		query.Set("state", state)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func validateOIDCRedirectURI(app *model.Application, redirectURI string) error {
	return protocol.NewOIDCProtocolHandler(helper.GetDatabase()).ValidateRedirectURI(app, redirectURI)
}

func checkOIDCUserAccess(session *service.OIDCSession, userID uint) (*model.Application, *dto.AuthorizationFailure, error) {
	if session == nil {
		return nil, nil, fmt.Errorf("OIDC 会话为空")
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	app, err := appService.GetApplicationByClientID(session.ClientID)
	if err != nil {
		return nil, nil, fmt.Errorf("获取应用信息失败: %w", err)
	}
	if err := validateOIDCRedirectURI(app, session.RedirectURI); err != nil {
		return nil, nil, fmt.Errorf("无效的重定向 URI: %w", err)
	}
	if appService.CheckUserAccess(app.ID, userID) {
		return app, nil, nil
	}

	failure, err := newOIDCFailure(session, oidcAccessDeniedCode, oidcAccessDeniedDescription)
	if err != nil {
		return nil, nil, err
	}
	return app, failure, nil
}

func newOIDCFailure(session *service.OIDCSession, code, description string) (*dto.AuthorizationFailure, error) {
	if session == nil {
		return nil, fmt.Errorf("OIDC 会话为空")
	}
	redirectURL, err := buildOIDCErrorRedirectURL(
		session.RedirectURI,
		code,
		description,
		session.State,
	)
	if err != nil {
		return nil, err
	}
	return &dto.AuthorizationFailure{
		Code:        code,
		Description: description,
		RedirectURL: redirectURL,
	}, nil
}

func markOIDCSessionFailed(
	ctx context.Context,
	sessions *service.OIDCSessionService,
	sessionID string,
	failure *dto.AuthorizationFailure,
) error {
	if failure == nil {
		return nil
	}
	if err := sessions.MarkAsFailed(ctx, sessionID, failure); err != nil {
		return fmt.Errorf("记录授权失败状态: %w", err)
	}
	return nil
}
