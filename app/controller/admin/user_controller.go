package admin

import (
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// UserController 管理员用户控制器
type UserController struct {
	BaseController
}

// CreateUser 创建用户（管理员操作）
// POST /api/admin/users
func (auc UserController) CreateUser(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) GetUser(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) UpdateUser(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) DeleteUser(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) GetUserList(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) UpdateUserStatus(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) ResetPassword(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
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
func (auc UserController) GetUsersByDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	departmentIdStr := c.Param("id")
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

// GetUserDepartments 获取用户的部门列表（管理员操作）
// GET /api/admin/users/:id/departments
func (auc UserController) GetUserDepartments(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserDepartments(uint(userID))
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}

// GetActiveUsersCount 获取活跃用户数量统计（管理员操作）
// GET /api/admin/users/stats/active-count
func (auc UserController) GetActiveUsersCount(c *gin.Context, helper interfaces.HelperInterface) {
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

// AssignUserRole 为用户分配角色
// POST /api/admin/users/:id/roles
func (auc UserController) AssignUserRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	roleIDStr := c.Param("roleId")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	var req dto.AssignUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 从JWT中获取当前管理员ID（假设已经在中间件中设置）
	adminID := uint(1) // TODO: 从上下文中获取实际的管理员ID

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.AssignUserRole(uint(userID), uint(roleID), adminID, &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		} else if strings.Contains(errMsg, "角色不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "已分配该角色") {
			errCode = constants.ErrCodeBadRequest
		}

		auc.Error(c, errCode, errMsg)
		return
	}

	auc.SuccessWithMessage(c, "角色分配成功", nil)
}

// RemoveUserRole 移除用户角色
// DELETE /api/admin/users/:id/roles/:roleId
func (auc UserController) RemoveUserRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	roleIDStr := c.Param("roleId")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.RemoveUserRole(uint(userID), uint(roleID))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "不存在") {
			errCode = constants.ErrCodeNotFound
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "角色移除成功", nil)
}

// GetUserRoles 获取用户的角色列表
// GET /api/admin/users/:id/roles
func (auc UserController) GetUserRoles(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserRoles(uint(userID))
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}

// GetUserWithRoles 获取用户及其角色信息
// GET /api/admin/users/:id/with-roles
func (auc UserController) GetUserWithRoles(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserWithRoles(uint(userID))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.Success(c, response)
}

// BatchAssignUserRoles 批量分配角色给用户
// POST /api/admin/users/:id/roles/batch
func (auc UserController) BatchAssignUserRoles(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	var req dto.BatchAssignUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 从JWT中获取当前管理员ID
	adminID := uint(1) // TODO: 从上下文中获取实际的管理员ID

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.BatchAssignUserRoles(uint(userID), req.RoleIDs, adminID)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		} else if strings.Contains(errMsg, "角色") && strings.Contains(errMsg, "不存在") {
			errCode = constants.ErrCodeBadRequest
		}

		auc.Error(c, errCode, errMsg)
		return
	}

	auc.SuccessWithMessage(c, "批量角色分配成功", nil)
}

// BatchRemoveUserRoles 批量移除用户角色
// DELETE /api/admin/users/:id/roles/batch
func (auc UserController) BatchRemoveUserRoles(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	var req dto.BatchRemoveUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.BatchRemoveUserRoles(uint(userID), req.RoleIDs)
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "用户不存在") {
			errCode = constants.ErrCodeUserNotFound
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "批量角色移除成功", nil)
}

// SetUserPrimaryRole 设置用户主要角色
// PUT /api/admin/users/:id/roles/:roleId/primary
func (auc UserController) SetUserPrimaryRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	roleIDStr := c.Param("roleId")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	err = userService.SetUserPrimaryRole(uint(userID), uint(roleID))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "不存在") {
			errCode = constants.ErrCodeNotFound
		}
		auc.Error(c, errCode, err.Error())
		return
	}

	auc.SuccessWithMessage(c, "主要角色设置成功", nil)
}

// GetUserPermissions 获取用户的权限列表
// GET /api/admin/users/:id/permissions
func (auc UserController) GetUserPermissions(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserPermissions(uint(userID))
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}

// GetUserMenuTree 获取用户菜单树
// GET /api/admin/users/:id/menu-tree
func (auc UserController) GetUserMenuTree(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	userID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		auc.Error(c, constants.ErrCodeBadRequest, "用户ID格式错误")
		return
	}

	userService := service.NewUserService(helper.GetDatabase())
	response, err := userService.GetUserMenuTree(uint(userID))
	if err != nil {
		auc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	auc.Success(c, response)
}
