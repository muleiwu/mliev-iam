package controller

import (
	"errors"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

// resolveExistingIAMUser reads only server-controlled HttpOnly cookies. A
// refresh token is the canonical SSO identity; user_access_token is a fallback
// for personal-center sessions that have no OIDC refresh token.
func resolveExistingIAMUser(ctx httpInterfaces.RouterContextInterface) (*dto.UserResponse, error) {
	userService := service.NewUserService(helper.GetDatabase())

	if refreshToken, err := ctx.Cookie("refresh_token"); err == nil && refreshToken != "" {
		token, tokenErr := dao.NewOIDCTokenDAO(helper.GetDatabase()).GetValidByRefreshToken(refreshToken)
		switch {
		case tokenErr == nil && token != nil:
			userModel, userErr := userService.GetUserModelByID(token.UserID)
			if userErr != nil {
				if errors.Is(userErr, gorm.ErrRecordNotFound) {
					return nil, nil
				}
				return nil, userErr
			}
			if userModel == nil || !userModel.CanLogin() {
				return nil, nil
			}
			return userService.GetUserByID(token.UserID)
		case tokenErr != nil && !errors.Is(tokenErr, gorm.ErrRecordNotFound):
			return nil, tokenErr
		}
	}

	userToken, err := ctx.Cookie(middleware.UserCookieName)
	if err != nil || userToken == "" {
		return nil, nil
	}
	claims, err := middleware.ValidateUserSessionToken(userToken)
	if err != nil || claims == nil {
		return nil, nil
	}
	return userService.GetUserByID(claims.UserID)
}

func sameOIDCBrowserUser(ctx httpInterfaces.RouterContextInterface, expectedUserID uint) (*dto.UserResponse, bool, error) {
	user, err := resolveExistingIAMUser(ctx)
	if err != nil {
		return nil, false, err
	}
	return user, user != nil && user.ID == expectedUserID, nil
}
