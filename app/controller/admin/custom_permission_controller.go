package admin

import (
	"errors"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

type CustomPermissionController struct{ BaseController }

func (cc CustomPermissionController) result(c httpInterfaces.RouterContextInterface, data interface{}, err error) {
	if err == nil {
		cc.Success(c, data)
		return
	}
	code, message := constants.ErrCodeInternal, "自定义权限操作失败"
	if errors.Is(err, service.ErrCustomPermissionInvalid) {
		code, message = constants.ErrCodeBadRequest, err.Error()
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		code, message = constants.ErrCodeNotFound, "权限、授权或对象不存在"
	}
	cc.Error(c, code, message)
}
func (cc CustomPermissionController) id(c httpInterfaces.RouterContextInterface) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		cc.Error(c, constants.ErrCodeBadRequest, "无效的 ID")
		return 0, false
	}
	return uint(id), true
}
func (cc CustomPermissionController) actor(c httpInterfaces.RouterContextInterface) (uint, bool) {
	id, ok := c.Get("user_id").(uint)
	if !ok || id == 0 {
		cc.Error(c, constants.ErrCodeUnauthorized, "请先登录")
		return 0, false
	}
	return id, true
}
func (cc CustomPermissionController) List(c httpInterfaces.RouterContextInterface) {
	var q dto.CustomPermissionQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	data, err := service.NewCustomPermissionService(helper.GetDatabase()).ListPermissions(q)
	cc.result(c, data, err)
}
func (cc CustomPermissionController) Get(c httpInterfaces.RouterContextInterface) {
	id, ok := cc.id(c)
	if !ok {
		return
	}
	data, err := service.NewCustomPermissionService(helper.GetDatabase()).GetPermission(id)
	cc.result(c, data, err)
}
func (cc CustomPermissionController) Create(c httpInterfaces.RouterContextInterface) {
	cc.save(c, false)
}
func (cc CustomPermissionController) Update(c httpInterfaces.RouterContextInterface) {
	cc.save(c, true)
}
func (cc CustomPermissionController) save(c httpInterfaces.RouterContextInterface, update bool) {
	actor, ok := cc.actor(c)
	if !ok {
		return
	}
	var id uint
	if update {
		id, ok = cc.id(c)
		if !ok {
			return
		}
	}
	var req dto.CustomPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	data, err := service.NewCustomPermissionService(helper.GetDatabase()).SavePermission(id, req, actor)
	cc.result(c, data, err)
}
func (cc CustomPermissionController) Delete(c httpInterfaces.RouterContextInterface) {
	id, ok := cc.id(c)
	if !ok {
		return
	}
	cc.result(c, nil, service.NewCustomPermissionService(helper.GetDatabase()).DeletePermission(id))
}
func (cc CustomPermissionController) ListGrants(c httpInterfaces.RouterContextInterface) {
	var q dto.CustomPermissionQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	data, err := service.NewCustomPermissionService(helper.GetDatabase()).ListGrants(q)
	cc.result(c, data, err)
}
func (cc CustomPermissionController) CreateGrant(c httpInterfaces.RouterContextInterface) {
	cc.saveGrant(c, false)
}
func (cc CustomPermissionController) UpdateGrant(c httpInterfaces.RouterContextInterface) {
	cc.saveGrant(c, true)
}
func (cc CustomPermissionController) saveGrant(c httpInterfaces.RouterContextInterface, update bool) {
	actor, ok := cc.actor(c)
	if !ok {
		return
	}
	var id uint
	if update {
		id, ok = cc.id(c)
		if !ok {
			return
		}
	}
	var req dto.CustomPermissionGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	data, err := service.NewCustomPermissionService(helper.GetDatabase()).SaveGrant(id, req, actor)
	cc.result(c, data, err)
}
func (cc CustomPermissionController) RevokeGrant(c httpInterfaces.RouterContextInterface) {
	actor, ok := cc.actor(c)
	if !ok {
		return
	}
	id, ok := cc.id(c)
	if !ok {
		return
	}
	cc.result(c, nil, service.NewCustomPermissionService(helper.GetDatabase()).RevokeGrant(id, actor))
}
