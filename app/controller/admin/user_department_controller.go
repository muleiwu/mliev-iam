package admin

import (
	"errors"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// UserDepartmentController 管理用户与组织部门的归属关系。
type UserDepartmentController struct {
	BaseController
}

// Create 创建用户部门关系。
// POST /api/admin/user-departments
func (udc UserDepartmentController) Create(c httpInterfaces.RouterContextInterface) {
	var req dto.UserDepartmentCreateDto
	if err := c.ShouldBindJSON(&req); err != nil {
		udc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	response, err := service.NewUserDepartmentService(helper.GetDatabase()).Create(&req)
	if err != nil {
		udc.writeError(c, err)
		return
	}
	udc.SuccessWithMessage(c, "用户部门添加成功", response)
}

// Delete 删除用户部门关系。
// DELETE /api/admin/user-departments/:userId/:departmentId
func (udc UserDepartmentController) Delete(c httpInterfaces.RouterContextInterface) {
	userID, ok := udc.parseID(c, "userId", "用户ID")
	if !ok {
		return
	}
	departmentID, ok := udc.parseID(c, "departmentId", "部门ID")
	if !ok {
		return
	}

	if err := service.NewUserDepartmentService(helper.GetDatabase()).Delete(userID, departmentID); err != nil {
		udc.writeError(c, err)
		return
	}
	udc.SuccessWithMessage(c, "用户部门删除成功", nil)
}

// SetPrimary 设置用户主部门。
// PUT /api/admin/users/:id/departments/:departmentId/primary
func (udc UserDepartmentController) SetPrimary(c httpInterfaces.RouterContextInterface) {
	userID, ok := udc.parseID(c, "id", "用户ID")
	if !ok {
		return
	}
	departmentID, ok := udc.parseID(c, "departmentId", "部门ID")
	if !ok {
		return
	}

	response, err := service.NewUserDepartmentService(helper.GetDatabase()).SetPrimary(userID, departmentID)
	if err != nil {
		udc.writeError(c, err)
		return
	}
	udc.SuccessWithMessage(c, "主部门设置成功", response)
}

func (udc UserDepartmentController) parseID(
	c httpInterfaces.RouterContextInterface,
	param string,
	label string,
) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(param), 10, 32)
	if err != nil || id == 0 {
		udc.Error(c, constants.ErrCodeBadRequest, label+"格式错误")
		return 0, false
	}
	return uint(id), true
}

func (udc UserDepartmentController) writeError(c httpInterfaces.RouterContextInterface, err error) {
	code := constants.ErrCodeInternal
	switch {
	case errors.Is(err, service.ErrUserDepartmentConflict):
		code = constants.ErrCodeConflict
	case errors.Is(err, service.ErrUserDepartmentNotFound),
		errors.Is(err, service.ErrUserDepartmentUserNotFound),
		errors.Is(err, service.ErrUserDepartmentDepartmentNotFound):
		code = constants.ErrCodeNotFound
	case errors.Is(err, service.ErrUserDepartmentDepartmentDisabled):
		code = constants.ErrCodeBadRequest
	}
	udc.Error(c, code, err.Error())
}
