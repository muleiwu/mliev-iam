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

// PermissionController 管理员权限控制器
type PermissionController struct {
	BaseController
}

// CreatePermission 创建权限
// POST /api/admin/permissions
func (pc PermissionController) CreatePermission(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.CreatePermission(&req)
	if err != nil {
		// 根据错误信息选择合适的错误码
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "权限名称已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "权限代码已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "父权限不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "必须指定") {
			errCode = constants.ErrCodeBadRequest
		}

		pc.Error(c, errCode, errMsg)
		return
	}

	pc.SuccessWithMessage(c, "权限创建成功", response)
}

// GetPermission 获取权限详情
// GET /api/admin/permissions/:id
func (pc PermissionController) GetPermission(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "权限ID格式错误")
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.GetPermissionByID(uint(id))
	if err != nil {
		errCode := constants.ErrCodeNotFound
		if !strings.Contains(err.Error(), "权限不存在") {
			errCode = constants.ErrCodeInternal
		}
		pc.Error(c, errCode, err.Error())
		return
	}

	pc.Success(c, response)
}

// UpdatePermission 更新权限信息
// PUT /api/admin/permissions/:id
func (pc PermissionController) UpdatePermission(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "权限ID格式错误")
		return
	}

	var req dto.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.UpdatePermission(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "权限不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "权限名称已存在") {
			errCode = constants.ErrCodeBadRequest
		}

		pc.Error(c, errCode, errMsg)
		return
	}

	pc.SuccessWithMessage(c, "权限更新成功", response)
}

// DeletePermission 删除权限
// DELETE /api/admin/permissions/:id
func (pc PermissionController) DeletePermission(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "权限ID格式错误")
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	err = permissionService.DeletePermission(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "权限不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "存在子权限") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "有") && strings.Contains(errMsg, "个角色使用此权限") {
			errCode = constants.ErrCodeBadRequest
		}

		pc.Error(c, errCode, errMsg)
		return
	}

	pc.SuccessWithMessage(c, "权限删除成功", nil)
}

// GetPermissionList 获取权限列表
// GET /api/admin/permissions
func (pc PermissionController) GetPermissionList(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.PermissionQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "查询参数错误: "+err.Error())
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.GetPermissionList(&req)
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, response)
}

// GetPermissionTree 获取权限树形结构
// GET /api/admin/permissions/tree
func (pc PermissionController) GetPermissionTree(c *gin.Context, helper interfaces.HelperInterface) {
	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.GetPermissionTree()
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, response)
}

// GetMenuTree 获取菜单树形结构
// GET /api/admin/permissions/menu-tree
func (pc PermissionController) GetMenuTree(c *gin.Context, helper interfaces.HelperInterface) {
	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.GetMenuTree()
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, response)
}

// UpdatePermissionStatus 更新权限状态
// PUT /api/admin/permissions/:id/status
func (pc PermissionController) UpdatePermissionStatus(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "权限ID格式错误")
		return
	}

	var req dto.PermissionStatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	err = permissionService.UpdatePermissionStatus(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "权限不存在") {
			errCode = constants.ErrCodeNotFound
		}

		pc.Error(c, errCode, err.Error())
		return
	}

	pc.SuccessWithMessage(c, "权限状态更新成功", nil)
}

// BatchOperation 批量操作权限
// POST /api/admin/permissions/batch
func (pc PermissionController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.BatchPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	permissionService := service.NewPermissionService(helper.GetDatabase())
	err := permissionService.BatchOperation(&req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "有角色关联") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "不支持的操作类型") {
			errCode = constants.ErrCodeBadRequest
		}

		pc.Error(c, errCode, errMsg)
		return
	}

	pc.SuccessWithMessage(c, "批量操作成功", nil)
}

// GetPermissionStats 获取权限统计信息
// GET /api/admin/permissions/stats
func (pc PermissionController) GetPermissionStats(c *gin.Context, helper interfaces.HelperInterface) {
	permissionService := service.NewPermissionService(helper.GetDatabase())
	response, err := permissionService.GetPermissionStats()
	if err != nil {
		pc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	pc.Success(c, response)
}
