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

// RoleController 管理员角色控制器
type RoleController struct {
	BaseController
}

// CreateRole 创建角色
// POST /api/admin/roles
func (rc RoleController) CreateRole(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.CreateRole(&req)
	if err != nil {
		// 根据错误信息选择合适的错误码
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "角色名称已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "角色代码已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "父角色不存在") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "角色创建成功", response)
}

// GetRole 获取角色详情
// GET /api/admin/roles/:id
func (rc RoleController) GetRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.GetRoleByID(uint(id))
	if err != nil {
		errCode := constants.ErrCodeNotFound
		if !strings.Contains(err.Error(), "角色不存在") {
			errCode = constants.ErrCodeInternal
		}
		rc.Error(c, errCode, err.Error())
		return
	}

	rc.Success(c, response)
}

// UpdateRole 更新角色信息
// PUT /api/admin/roles/:id
func (rc RoleController) UpdateRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.UpdateRole(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "角色不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "系统角色不允许修改") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "角色名称已存在") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "角色更新成功", response)
}

// DeleteRole 删除角色
// DELETE /api/admin/roles/:id
func (rc RoleController) DeleteRole(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	err = roleService.DeleteRole(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "角色不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "系统角色不允许删除") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "存在子角色") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "有") && strings.Contains(errMsg, "个用户使用此角色") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "角色删除成功", nil)
}

// GetRoleList 获取角色列表
// GET /api/admin/roles
func (rc RoleController) GetRoleList(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.RoleQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "查询参数错误: "+err.Error())
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.GetRoleList(&req)
	if err != nil {
		rc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	rc.Success(c, response)
}

// GetRoleTree 获取角色树形结构
// GET /api/admin/roles/tree
func (rc RoleController) GetRoleTree(c *gin.Context, helper interfaces.HelperInterface) {
	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.GetRoleTree()
	if err != nil {
		rc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	rc.Success(c, response)
}

// UpdateRoleStatus 更新角色状态
// PUT /api/admin/roles/:id/status
func (rc RoleController) UpdateRoleStatus(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	var req dto.RoleStatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	err = roleService.UpdateRoleStatus(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "角色不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "系统角色不允许修改状态") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "角色状态更新成功", nil)
}

// BatchOperation 批量操作角色
// POST /api/admin/roles/batch
func (rc RoleController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.BatchRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	err := roleService.BatchOperation(&req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "系统角色不允许批量操作") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "有用户关联") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "不支持的操作类型") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "批量操作成功", nil)
}

// AssignPermissions 为角色分配权限
// POST /api/admin/roles/:id/permissions
func (rc RoleController) AssignPermissions(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	var req dto.AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 获取当前登录用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		rc.Error(c, constants.ErrCodeUnauthorized, "未找到当前用户信息")
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	err = roleService.AssignPermissions(uint(id), &req, userID.(uint))
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "角色不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "权限") && strings.Contains(errMsg, "不存在") {
			errCode = constants.ErrCodeBadRequest
		}

		rc.Error(c, errCode, errMsg)
		return
	}

	rc.SuccessWithMessage(c, "权限分配成功", nil)
}

// GetRolePermissions 获取角色的权限列表
// GET /api/admin/roles/:id/permissions
func (rc RoleController) GetRolePermissions(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		rc.Error(c, constants.ErrCodeBadRequest, "角色ID格式错误")
		return
	}

	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.GetRolePermissions(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "角色不存在") {
			errCode = constants.ErrCodeNotFound
		}
		rc.Error(c, errCode, err.Error())
		return
	}

	rc.Success(c, response)
}

// GetRoleStats 获取角色统计信息
// GET /api/admin/roles/stats
func (rc RoleController) GetRoleStats(c *gin.Context, helper interfaces.HelperInterface) {
	roleService := service.NewRoleService(helper.GetDatabase())
	response, err := roleService.GetRoleStats()
	if err != nil {
		rc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	rc.Success(c, response)
}
