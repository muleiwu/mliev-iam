package admin

import (
	"fmt"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type IdentityConnectionController struct{ controller.BaseResponse }

func (cc IdentityConnectionController) Create(c httpInterfaces.RouterContextInterface) {
	var req dto.CreateIdentityConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	req.Config = ensureAuthenticationCallback(c, req.Identifier, req.Config)
	response, err := service.NewIdentityConnectionService(helper.GetDatabase()).Create(&req)
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "身份连接创建成功", cc.withCallbacks(c, response))
}

func (cc IdentityConnectionController) ListPrincipalConflicts(c httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	connectionID, _ := strconv.ParseUint(c.Query("connectionId"), 10, 64)
	items, total, err := service.NewExternalPrincipalResolver(helper.GetDatabase()).ListConflicts(uint(connectionID), c.Query("status"), page, pageSize)
	if err != nil {
		cc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	cc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

type resolvePrincipalConflictRequest struct {
	CanonicalPrincipalID uint   `json:"canonicalPrincipalId" binding:"required"`
	TargetUserID         uint   `json:"targetUserId"`
	Note                 string `json:"note"`
}

func (cc IdentityConnectionController) ResolvePrincipalConflict(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的冲突 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		cc.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	var req resolvePrincipalConflictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := service.NewExternalPrincipalResolver(helper.GetDatabase()).ResolveConflict(id, req.CanonicalPrincipalID, req.TargetUserID, actorID, req.Note); err != nil {
		cc.Error(c, constants.ErrCodeConflict, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "principal 冲突已解决", nil)
}

func (cc IdentityConnectionController) RejectPrincipalConflict(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的冲突 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		cc.Error(c, constants.ErrCodeUnauthorized, "未登录")
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := service.NewExternalPrincipalResolver(helper.GetDatabase()).RejectConflict(id, actorID, req.Note); err != nil {
		cc.Error(c, constants.ErrCodeConflict, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "principal 冲突已拒绝", nil)
}

func (cc IdentityConnectionController) Get(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	response, err := service.NewIdentityConnectionService(helper.GetDatabase()).Get(id)
	if err != nil {
		cc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	cc.Success(c, cc.withCallbacks(c, response))
}

func (cc IdentityConnectionController) List(c httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	items, total, err := service.NewIdentityConnectionService(helper.GetDatabase()).List(
		page, pageSize, c.Query("category"), c.Query("providerType"), c.Query("capability"), c.Query("status"),
	)
	if err != nil {
		cc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	for _, item := range items {
		cc.withCallbacks(c, item)
	}
	cc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (cc IdentityConnectionController) Update(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	var req dto.UpdateIdentityConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.Config != nil {
		current, loadErr := service.NewIdentityConnectionService(helper.GetDatabase()).Get(id)
		if loadErr != nil {
			cc.Error(c, constants.ErrCodeBadRequest, loadErr.Error())
			return
		}
		identifier := current.Identifier
		if req.Identifier != nil {
			identifier = *req.Identifier
		}
		req.Config = ensureAuthenticationCallback(c, identifier, req.Config)
	}
	response, err := service.NewIdentityConnectionService(helper.GetDatabase()).Update(id, &req)
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "身份连接更新成功", cc.withCallbacks(c, response))
}

func (cc IdentityConnectionController) Delete(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	if err := service.NewIdentityConnectionService(helper.GetDatabase()).Delete(id); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "身份连接已删除", nil)
}

func (cc IdentityConnectionController) Batch(c httpInterfaces.RouterContextInterface) {
	var req dto.IdentityConnectionBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := service.NewIdentityConnectionService(helper.GetDatabase()).Batch(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "批量操作成功", nil)
}

func (cc IdentityConnectionController) Stats(c httpInterfaces.RouterContextInterface) {
	stats, err := service.NewIdentityConnectionService(helper.GetDatabase()).Stats()
	if err != nil {
		cc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	cc.Success(c, stats)
}

func (cc IdentityConnectionController) Test(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	results, err := service.NewIdentityConnectionService(helper.GetDatabase()).TestCapabilities(c.Request().Context(), id)
	message := "连接能力校验完成"
	if err != nil {
		message = "部分连接能力校验失败，请查看各能力结果"
	}
	cc.SuccessWithMessage(c, message, results)
}

func ensureAuthenticationCallback(c httpInterfaces.RouterContextInterface, identifier string, config map[string]any) map[string]any {
	result := make(map[string]any, len(config)+1)
	for key, value := range config {
		result[key] = value
	}
	result["redirectUrl"] = utils.GetBaseURL(c, fmt.Sprintf("/external-auth/%s/callback", identifier))
	return result
}

func (cc IdentityConnectionController) withCallbacks(c httpInterfaces.RouterContextInterface, response *dto.IdentityConnectionResponse) *dto.IdentityConnectionResponse {
	if response == nil {
		return nil
	}
	response.AuthenticationCallbackURL = utils.GetBaseURL(c, fmt.Sprintf("/external-auth/%s/callback", response.Identifier))
	response.EventsCallbackURL = utils.GetBaseURL(c, fmt.Sprintf("/external-events/%s", response.Identifier))
	return response
}

func (cc IdentityConnectionController) DiscoverOIDC(c httpInterfaces.RouterContextInterface) {
	var req dto.OIDCDiscoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	doc, err := protocol.NewOIDCDiscoveryCache(helper.GetRedis()).FetchAndCache(c.Request().Context(), req.Issuer)
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.Success(c, dto.OIDCDiscoverResponse{
		Issuer: doc.Issuer, AuthorizationEndpoint: doc.AuthorizationEndpoint,
		TokenEndpoint: doc.TokenEndpoint, UserinfoEndpoint: doc.UserinfoEndpoint,
		JwksURI: doc.JwksURI, ScopesSupported: doc.ScopesSupported,
	})
}
