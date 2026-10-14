package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type PasskeyController struct{ BaseResponse }

type passkeyVerificationRequest struct {
	TransactionID string          `json:"transactionId" binding:"required,len=43"`
	Credential    json.RawMessage `json:"credential" binding:"required"`
}

func passkeyService() (*service.PasskeyService, error) {
	return service.NewPasskeyService(helper.GetDatabase(), helper.GetRedis(), service.ReadPasskeyConfig(), twoFactorService())
}
func (pc PasskeyController) Config(c httpInterfaces.RouterContextInterface) {
	cfg := service.ReadPasskeyConfig()
	_, err := cfg.WebAuthn()
	pc.Success(c, map[string]any{"enabled": cfg.Enabled && err == nil})
}
func (pc PasskeyController) List(c httpInterfaces.RouterContextInterface) {
	uid, _ := middleware.GetCurrentUserID(c)
	items, err := dao.NewPasskeyDAO(helper.GetDatabase()).List(uid)
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, items)
}
func (pc PasskeyController) begin(c httpInterfaces.RouterContextInterface) (*service.PasskeyService, bool) {
	if !(UserController{}).requireJSONContentType(c) {
		return nil, false
	}
	svc, err := passkeyService()
	if err == nil {
		err = service.PasskeyRequestLimit(c.Request().Context(), helper.GetRedis(), twoFactorBinding(c, true), c.ClientIP())
	}
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return nil, false
	}
	return svc, true
}
func (pc PasskeyController) RegistrationOptions(c httpInterfaces.RouterContextInterface) {
	svc, ok := pc.begin(c)
	if !ok {
		return
	}
	var req struct {
		OperationToken string `json:"operationToken" binding:"required"`
		Name           string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	uid, _ := middleware.GetCurrentUserID(c)
	result, err := svc.BeginRegistration(c.Request().Context(), uid, middleware.GetCurrentSessionID(c), req.OperationToken, req.Name, twoFactorBinding(c, false), c.GetHeader("Origin"))
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, result)
}
func readPasskeyVerification(c httpInterfaces.RouterContextInterface, purpose string) (*service.PasskeyService, *service.PasskeyTransaction, *passkeyVerificationRequest, error) {
	c.Request().Body = http.MaxBytesReader(nil, c.Request().Body, 128<<10)
	var req passkeyVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, nil, nil, err
	}
	if len(req.Credential) > 65536 {
		return nil, nil, nil, errors.New("Passkey 响应过大")
	}
	svc, err := passkeyService()
	if err != nil {
		return nil, nil, nil, err
	}
	tr, err := svc.Read(c.Request().Context(), req.TransactionID, twoFactorBinding(c, false), c.GetHeader("Origin"), purpose)
	return svc, tr, &req, err
}
func (pc PasskeyController) RegistrationVerify(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	svc, tr, req, err := readPasskeyVerification(c, "registration")
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	uid, _ := middleware.GetCurrentUserID(c)
	proof, err := svc.FinishRegistration(c.Request().Context(), tr, uid, middleware.GetCurrentSessionID(c), req.Credential, c.ClientIP())
	if err == nil {
		err = renewTwoFactorSession(c, uid, proof)
	}
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, map[string]any{"twoFactorUpdated": true})
}
func (pc PasskeyController) LoginOptions(c httpInterfaces.RouterContextInterface) {
	svc, ok := pc.begin(c)
	if !ok {
		return
	}
	var req struct {
		SessionID  string `json:"sessionId"`
		RememberMe bool   `json:"rememberMe"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	if req.SessionID != "" {
		if _, err := getBoundOIDCSession(c, service.NewOIDCSessionService(helper.GetRedis()), req.SessionID); err != nil {
			(TwoFactorController{}).failure(c, err)
			return
		}
	}
	result, err := svc.BeginLogin(c.Request().Context(), twoFactorBinding(c, false), c.GetHeader("Origin"), req.SessionID, req.RememberMe)
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, result)
}
func (pc PasskeyController) LoginVerify(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	svc, tr, req, err := readPasskeyVerification(c, "login")
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	sessions := service.NewOIDCSessionService(helper.GetRedis())
	var oidc *service.OIDCSession
	if tr.OIDCSessionID != "" {
		oidc, err = getBoundOIDCSession(c, sessions, tr.OIDCSessionID)
		if err != nil {
			(TwoFactorController{}).failure(c, err)
			return
		}
	}
	proof, err := svc.FinishLogin(c.Request().Context(), tr, req.Credential)
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	user, err := service.NewUserService(helper.GetDatabase()).GetUserByID(proof.UserID())
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	c.Set("primary_auth_method", "passkey")
	c.Set("primary_passkey_proof", proof)
	if oidc != nil {
		result, err := (OIDCAuthFlowController{}).completeOIDCLogin(c, sessions, oidc, tr.OIDCSessionID, user, tr.RememberMe)
		if err != nil {
			(TwoFactorController{}).failure(c, err)
			return
		}
		pc.Success(c, result)
		return
	}
	if err = completeUserLogin(c, user, tr.RememberMe); err != nil {
		if !respondTwoFactorRequired(c, err) {
			(TwoFactorController{}).failure(c, err)
		}
		return
	}
	pc.Success(c, sanitizeSelfProfile(user))
}
func (pc PasskeyController) SecondOptions(c httpInterfaces.RouterContextInterface) {
	svc, ok := pc.begin(c)
	if !ok {
		return
	}
	tc := TwoFactorController{}
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	result, err := svc.BeginSecondFactor(c.Request().Context(), tr, twoFactorBinding(c, false), c.GetHeader("Origin"))
	if err != nil {
		tc.failure(c, err)
		return
	}
	pc.Success(c, result)
}
func (pc PasskeyController) SecondVerify(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	tc := TwoFactorController{}
	tr, err := tc.transaction(c)
	if err != nil {
		tc.failure(c, err)
		return
	}
	svc, ceremony, req, err := readPasskeyVerification(c, "second")
	if err != nil {
		tc.failure(c, err)
		return
	}
	proof, err := svc.FinishSecondFactor(c.Request().Context(), tr, ceremony, req.Credential, c.ClientIP())
	if err != nil {
		tc.failure(c, err)
		return
	}
	tc.complete(c, tr, proof)
}
func (pc PasskeyController) Rename(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	uid, _ := middleware.GetCurrentUserID(c)
	if err = service.RenamePasskey(helper.GetDatabase(), uid, uint(id), req.Name); err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, map[string]any{"updated": true})
}
func (pc PasskeyController) Delete(c httpInterfaces.RouterContextInterface) {
	if !(UserController{}).requireJSONContentType(c) {
		return
	}
	var req struct {
		OperationToken string `json:"operationToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	uid, _ := middleware.GetCurrentUserID(c)
	proof, err := twoFactorService().RemovePasskey(c.Request().Context(), uid, middleware.GetCurrentSessionID(c), req.OperationToken, uint(id), c.ClientIP())
	if err == nil {
		err = renewTwoFactorSession(c, uid, proof)
	}
	if err != nil {
		(TwoFactorController{}).failure(c, err)
		return
	}
	pc.Success(c, map[string]any{"twoFactorUpdated": true})
}
