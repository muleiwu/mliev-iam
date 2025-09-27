package admin

import (
	"strconv"
	"strings"

	"cnb.cool/mliev/examples/go-web/app/constants"
	"cnb.cool/mliev/examples/go-web/app/controller"
	"cnb.cool/mliev/examples/go-web/app/dto"
	"cnb.cool/mliev/examples/go-web/app/service"
	"cnb.cool/mliev/examples/go-web/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// AdminUserController 管理员用户控制器
type AdminUserController struct {
	controller.BaseResponse
}

// CreateUser 创建用户（管理员操作）
// POST /api/admin/users
func (auc AdminUserController) CreateUser(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.CreateUser(&req)
	if err != nil {
		// 根据错误信息选择合适的错误码
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户名已存在") {
			errCode = constants.ErrCodeUserAlreadyExists
		} else if strings.Contains(errMsg, "邮箱已存在") {
			errCode = constants.ErrCodeEmailAlreadyExists
		} else if strings.Contains(errMsg, "手机号已存在") {
			errCode = constants.ErrCodePhoneAlreadyExists
		}

		auc.Error(c, errCode, errMsg)
		return
	}

	auc.SuccessWithMessage(c, "用户创建成功", response)
}

// GetUser 获取用户详情（管理员操作）
// GET /api/admin/users/:id
func (auc AdminUserController) GetUser(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserByID(uint(id))
	if err != nil {
		errCode := constants.ErrCodeUserNotFound
		if !strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeInternal
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.Success(c, response)
}

// UpdateUser 更新用户信息（管理员操作）
// PUT /api/admin/users/:id
func (auc AdminUserController) UpdateUser(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.UpdateUser(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		} else if strings.Contains(errMsg, "邮箱已存在") {
			errCode = constants.ErrCodeEmailAlreadyExists
		} else if strings.Contains(errMsg, "手机号已存在") {
			errCode = constants.ErrCodePhoneAlreadyExists
		}

		auc.Error(c, errCode, errMsg)
		return
	}

	auc.SuccessWithMessage(c, "用户更新成功", response)
}

// DeleteUser 删除用户（管理员操作）
// DELETE /api/admin/users/:id
func (auc AdminUserController) DeleteUser(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.DeleteUser(uint(id))
	if err != nil {
		errCode := constants.ErrCodeUserNotFound
		if !strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeInternal
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "用户删除成功", nil)
}

// GetUserList 获取用户列表（管理员操作）
// GET /api/admin/users
func (auc AdminUserController) GetUserList(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.UserQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "查询参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserList(&req)
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}

// UpdateUserStatus 更新用户状态（管理员操作）
// PUT /api/admin/users/:id/status
func (auc AdminUserController) UpdateUserStatus(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	var req dto.UserStatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.UpdateUserStatus(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeUserNotFound
		if !strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeInternal
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "用户状态更新成功", nil)
}

// ResetPassword 重置用户密码（管理员操作）
// PUT /api/admin/users/:id/reset-password
func (auc AdminUserController) ResetPassword(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	var req struct {
		NewPassword string `json:"newPassword" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.ResetPassword(uint(id), req.NewPassword)
	if err != nil {
		errCode := constants.ErrCodeUserNotFound
		if !strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeInternal
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "密码重置成功", nil)
}

// BatchOperation 批量操作用户（管理员操作）
// POST /api/admin/users/batch
func (auc AdminUserController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.BatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err := userService.BatchOperation(&req)
	if err != nil {
		auc.Error(c, constants.ErrCodeBatchOperationFailed, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "批量操作成功", nil)
}

// GetUsersByDepartment 根据部门获取用户列表（管理员操作）
// GET /api/admin/departments/:departmentId/users
func (auc AdminUserController) GetUsersByDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	departmentIdStr := c.Param("departmentId")
	departmentId, err := strconv.ParseUint(departmentIdStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUsersByDepartment(uint(departmentId))
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}

// GetActiveUsersCount 获取活跃用户数量统计（管理员操作）
// GET /api/admin/users/stats/active-count
func (auc AdminUserController) GetActiveUsersCount(c *gin.Context, helper interfaces.HelperInterface) {
	userService := service.NewUserService(helper.GetDatabase())
	count, err := userService.GetActiveUsersCount()
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, gin.H{
		"activeUsersCount": count,
	})
}
