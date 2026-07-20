package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
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
	response, err := service.NewIdentityConnectionService(helper.GetDatabase()).Create(&req)
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "身份连接创建成功", response)
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
	cc.Success(c, response)
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
	response, err := service.NewIdentityConnectionService(helper.GetDatabase()).Update(id, &req)
	if err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "身份连接更新成功", response)
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
	if err := service.NewIdentityConnectionService(helper.GetDatabase()).Test(id); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	cc.SuccessWithMessage(c, "连接配置校验通过", nil)
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
