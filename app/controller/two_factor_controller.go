package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"github.com/google/uuid"
)

const twoFactorCookie = "iam_2fa_binding"

type TwoFactorController struct{ BaseResponse }
type twoFactorPending struct{ challenge *dto.TwoFactorChallenge }

func (e *twoFactorPending) Error() string { return service.ErrTwoFactorRequired.Error() }

func twoFactorService() *service.TwoFactorService {
	return service.NewTwoFactorService(helper.GetDatabase(), helper.GetRedis(), getVerificationCodeConfig())
}

func twoFactorBinding(c httpInterfaces.RouterContextInterface, create bool) string {
	raw, _ := c.Cookie(twoFactorCookie)
	if len(raw) == 64 {
		return raw
	}
	if !create {
		return ""
	}
	raw = strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	secure := c.Request().TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	if secure {
		c.SetSameSite(http.SameSiteNoneMode)
	} else {
		c.SetSameSite(http.SameSiteLaxMode)
	}
	c.SetCookie(twoFactorCookie, raw, 0, "/", "", secure, true)
	return raw
}

func primaryAuthenticationMethod(c httpInterfaces.RouterContextInterface) string {
	if v, ok := c.Get("primary_auth_method").(string); ok && v != "" {
		return v
	}
	path := c.Request().URL.Path
	if strings.Contains(path, "external") || strings.Contains(path, "miniprogram") {
		return "external"
	}
	if strings.Contains(path, "register") {
		return "registration"
	}
	return "password"
}

func beginTwoFactorLogin(c httpInterfaces.RouterContextInterface, user *dto.UserResponse, kind, oidcID string, remember bool) error {
	if proof, _ := c.Get("two_factor_proof").(*service.TwoFactorProof); proof != nil {
		return nil
	}
	// Avoid creating a binding cookie for users who have not enabled 2FA.
	model, err := service.NewUserService(helper.GetDatabase()).GetUserModelByID(user.ID)
	if err != nil {
		return err
	}
	if !model.TwoFactorEnabled {
		return nil
	}
	svc := twoFactorService()
	continuation, _ := c.Get("two_factor_continuation").(string)
	continuationCSRF, _ := c.Get("two_factor_continuation_csrf").(string)
	tr, err := svc.BeginLogin(c.Request().Context(), user.ID, kind, primaryAuthenticationMethod(c), oidcID, remember, twoFactorBinding(c, true), continuation, continuationCSRF)
	if err != nil {
		return err
	}
	if tr == nil {
		return nil
	}
	if primary, _ := c.Get("primary_passkey_proof").(*service.PasskeyProof); primary != nil {
		if err := svc.BindPrimaryPasskey(c.Request().Context(), tr, primary); err != nil {
			return err
		}
	}
	if oidcID != "" {
		sessions := service.NewOIDCSessionService(helper.GetRedis())
		session, err := getBoundOIDCSession(c, sessions, oidcID)
		if err != nil {
			return err
		}
		if session.PendingTwoFactorID != "" {
			_ = helper.GetRedis().Del(c.Request().Context(), "2fa:challenge:"+session.PendingTwoFactorID).Err()
		}
		session.PendingTwoFactorID = tr.ID
		session.IsAuthenticated = false
		session.HasLoginInteraction = false
		session.UserID = nil
		session.LoginSessionID = ""
		session.IsConsentGranted = false
		if err = sessions.UpdateSession(c.Request().Context(), session); err != nil {
			return err
		}
	}
	view, err := svc.View(c.Request().Context(), tr)
	if err != nil {
		return err
	}
	return &twoFactorPending{challenge: view}
}

func respondTwoFactorRequired(c httpInterfaces.RouterContextInterface, err error) bool {
	var pending *twoFactorPending
	if !errors.As(err, &pending) {
		return false
	}
	BaseResponse{}.Success(c, dto.TwoFactorRequired{RequiresTwoFactor: true, TwoFactorChallenge: pending.challenge})
	return true
}

func (tc TwoFactorController) transaction(c httpInterfaces.RouterContextInterface) (*service.TwoFactorTransaction, error) {
	tr, err := twoFactorService().Read(c.Request().Context(), c.Param("id"), twoFactorBinding(c, false))
	if err != nil {
		return nil, err
	}
	if tr.OIDCSessionID != "" {
		session, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), tr.OIDCSessionID)
		if err != nil || session.PendingTwoFactorID != tr.ID || session.IsAuthenticated {
			return nil, service.ErrTwoFactorExpired
		}
	}
	return tr, nil
}

func (tc TwoFactorController) failure(c httpInterfaces.RouterContextInterface, err error) {
	code := 400
	if errors.Is(err, service.ErrTwoFactorLimited) {
		code = 429
	}
	tc.Error(c, code, err.Error())
}

func (tc TwoFactorController) Get(c httpInterfaces.RouterContextInterface) {
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	view, err := twoFactorService().View(c.Request().Context(), tr)
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, view)
}

func (tc TwoFactorController) Send(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		Method string `json:"method" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	retry, err := twoFactorService().SendCode(c.Request().Context(), tr, req.Method)
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, map[string]any{"retryAfter": retry})
}

func (tc TwoFactorController) Cancel(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	if err = twoFactorService().Cancel(c.Request().Context(), tr.ID, twoFactorBinding(c, false)); err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, map[string]any{"cancelled": true})
}

func (tc TwoFactorController) Verify(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		Method   string `json:"method" binding:"required"`
		Code     string `json:"code" binding:"max=80"`
		Password string `json:"password" binding:"max=1024"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	svc := twoFactorService()
	value := req.Code
	if req.Method == "password" {
		value = req.Password
	}
	proof, err := svc.Verify(c.Request().Context(), tr, req.Method, value, c.ClientIP())
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.complete(c, tr, proof)
}

func (tc TwoFactorController) complete(c httpInterfaces.RouterContextInterface, tr *service.TwoFactorTransaction, proof *service.TwoFactorProof) {
	svc := twoFactorService()
	switch tr.Purpose {
	case "manage":
		token, err := svc.Operation(c.Request().Context(), tr.UserID, tr.ParentSessionID, tr.Kind, proof)
		if err != nil {
			tc.failure(c, err)
			return
		}
		tc.Success(c, map[string]any{"operationToken": token, "expiresIn": 300})
		return
	case "enroll":
		codes, renewed, err := svc.ConfirmEnrollment(c.Request().Context(), tr, proof, c.ClientIP())
		if err != nil {
			tc.failure(c, err)
			return
		}
		if err := renewTwoFactorSession(c, tr.UserID, renewed); err != nil {
			tc.failure(c, err)
			return
		}
		tc.Success(c, map[string]any{"twoFactorUpdated": true, "recoveryCodes": codes})
		return
	case "login":
		if tr.ContinuationID != "" {
			if err := service.NewExternalAuthService(helper.GetDatabase(), helper.GetRedis()).CompleteTwoFactorBinding(c.Request().Context(), tr.ContinuationID, tr.ContinuationCSRF, proof); err != nil {
				tc.failure(c, err)
				return
			}
			clearContinuationCookie(c, tr.ContinuationCSRF)
		}
		user, err := service.NewUserService(helper.GetDatabase()).GetUserByID(tr.UserID)
		if err != nil {
			tc.failure(c, err)
			return
		}
		c.Set("two_factor_proof", proof)
		c.Set("two_factor_challenge_id", tr.ID)
		c.Set("primary_auth_method", tr.PrimaryMethod)
		if tr.OIDCSessionID != "" {
			sessions := service.NewOIDCSessionService(helper.GetRedis())
			session, err := getBoundOIDCSession(c, sessions, tr.OIDCSessionID)
			if err != nil {
				tc.failure(c, err)
				return
			}
			response, err := (OIDCAuthFlowController{}).completeOIDCLogin(c, sessions, session, tr.OIDCSessionID, user, oidcLoginOptions{RememberMe: tr.RememberMe})
			if err != nil {
				tc.failure(c, err)
				return
			}
			tc.Success(c, response)
			return
		}
		if err = completeUserLogin(c, user, tr.RememberMe); err != nil {
			tc.failure(c, err)
			return
		}
		if tr.Kind == "external" {
			token, _ := c.Get("login_response_token").(string)
			tc.Success(c, &dto.SocialAuthCallbackResponse{User: sanitizeSelfProfile(user), Token: token})
			return
		}
		tc.Success(c, sanitizeSelfProfile(user))
	default:
		tc.failure(c, service.ErrTwoFactorExpired)
	}
}

func renewTwoFactorSession(c httpInterfaces.RouterContextInterface, userID uint, proof *service.TwoFactorProof) error {
	user, err := service.NewUserService(helper.GetDatabase()).GetUserByID(userID)
	if err != nil {
		return err
	}
	jwt, err := service.NewJWTAuthServiceWithError()
	if err != nil {
		return err
	}
	// A self-service configuration change rotates the browser's user session and
	// any administrator access attached to that same SID. It is not a new login.
	retainAdmin := false
	oldUser, _ := c.Cookie(middleware.UserCookieName)
	oldAdmin, _ := c.Cookie(middleware.AdminCookieName)
	if userClaims, e := jwt.GetJWTService().LogoutClaims(oldUser); e == nil && userClaims.UserID == userID {
		if adminClaims, e := jwt.GetJWTService().LogoutClaims(oldAdmin); e == nil && adminClaims.UserID == userID && adminClaims.SessionID == userClaims.SessionID {
			var old model.LoginSession
			if e := helper.GetDatabase().First(&old, "id = ? AND user_id = ?", userClaims.SessionID, userID).Error; e == nil && old.HasAdminAccess {
				retainAdmin, _ = service.IsSuperAdmin(helper.GetDatabase(), userID)
			}
		}
	}
	c.Set("two_factor_proof", proof)
	c.Set("primary_auth_method", "security_change")
	token, age, sid, err := prepareUserSessionCookie(c, user, false, "")
	if err != nil {
		return err
	}
	if err = finishUserSession(c, token, age, sid, userID); err != nil {
		return err
	}
	if retainAdmin {
		adminToken, err := jwt.GenerateAdminToken(user, sid)
		if err != nil {
			return err
		}
		if err = helper.GetDatabase().Model(&model.LoginSession{}).Where("id = ? AND user_id = ?", sid, userID).Update("has_admin_access", true).Error; err != nil {
			return err
		}
		secure := c.Request().TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(middleware.AdminCookieName, adminToken, jwt.GetConfig().GetAccessTokenHours()*3600, "/", "", secure, true)
	}
	return nil
}

func (tc TwoFactorController) Status(c httpInterfaces.RouterContextInterface) {
	userID, _ := middleware.GetCurrentUserID(c)
	status, err := twoFactorService().Status(userID)
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, status)
}

func (tc TwoFactorController) Reauthenticate(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	userID, _ := middleware.GetCurrentUserID(c)
	kind := "user"
	if strings.HasPrefix(c.Request().URL.Path, "/api/admin/") {
		kind = "admin"
	}
	token, tr, err := twoFactorService().Reauthenticate(c.Request().Context(), userID, middleware.GetCurrentSessionID(c), req.Password, twoFactorBinding(c, true), kind)
	if err != nil {
		tc.failure(c, err)
		return
	}
	if tr != nil {
		view, err := twoFactorService().View(c.Request().Context(), tr)
		if err != nil {
			tc.failure(c, err)
			return
		}
		tc.Success(c, dto.TwoFactorRequired{RequiresTwoFactor: true, TwoFactorChallenge: view})
		return
	}
	tc.Success(c, map[string]any{"operationToken": token, "expiresIn": 300})
}

func (tc TwoFactorController) Enroll(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		Method         string `json:"method" binding:"required"`
		OperationToken string `json:"operationToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	userID, _ := middleware.GetCurrentUserID(c)
	result, err := twoFactorService().Enroll(c.Request().Context(), userID, middleware.GetCurrentSessionID(c), req.OperationToken, req.Method, twoFactorBinding(c, true))
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, result)
}

func (tc TwoFactorController) Mutate(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		OperationToken string `json:"operationToken" binding:"required"`
		ConfirmDisable bool   `json:"confirmDisable"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	action := "remove"
	path := c.Request().URL.Path
	if strings.HasSuffix(path, "/enable") {
		action = "enable"
	}
	if strings.HasSuffix(path, "/disable") {
		action = "disable"
	}
	if strings.HasSuffix(path, "/regenerate") {
		action = "regenerate"
	}
	userID, _ := middleware.GetCurrentUserID(c)
	codes, proof, err := twoFactorService().Mutate(c.Request().Context(), userID, middleware.GetCurrentSessionID(c), req.OperationToken, action, c.Param("method"), c.ClientIP(), req.ConfirmDisable)
	if err != nil {
		tc.failure(c, err)
		return
	}
	if err = renewTwoFactorSession(c, userID, proof); err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, map[string]any{"twoFactorUpdated": true, "recoveryCodes": codes})
}

func (tc TwoFactorController) AdminReset(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		OperationToken string `json:"operationToken" binding:"required"`
		Reason         string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		tc.failure(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		tc.failure(c, err)
		return
	}
	actor, _ := middleware.GetCurrentUserID(c)
	if err = twoFactorService().Reset(c.Request().Context(), actor, uint(id), middleware.GetCurrentSessionID(c), req.OperationToken, req.Reason, c.ClientIP()); err != nil {
		tc.failure(c, err)
		return
	}
	tc.Success(c, map[string]any{"reset": true})
}
