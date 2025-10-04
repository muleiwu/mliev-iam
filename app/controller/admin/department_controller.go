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

// DepartmentController 管理员部门控制器
type DepartmentController struct {
	BaseController
}

// CreateDepartment 创建部门（管理员操作）
// POST /api/admin/departments
func (adc DepartmentController) CreateDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.CreateDepartment(&req)
	if err != nil {
		// 根据错误信息选择合适的错误码
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "部门编码已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "部门名称已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "父部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "父部门未启用") {
			errCode = constants.ErrCodeBadRequest
		}

		adc.Error(c, errCode, errMsg)
		return
	}

	adc.SuccessWithMessage(c, "部门创建成功", response)
}

// GetDepartment 获取部门详情（管理员操作）
// GET /api/admin/departments/:id
func (adc DepartmentController) GetDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetDepartmentByID(uint(id))
	if err != nil {
		errCode := constants.ErrCodeBadRequest
		if strings.Contains(err.Error(), "部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else {
			errCode = constants.ErrCodeInternal
		}
		adc.Error(c, errCode, err.Error())
		return
	}

	adc.Success(c, response)
}

// UpdateDepartment 更新部门信息（管理员操作）
// PUT /api/admin/departments/:id
func (adc DepartmentController) UpdateDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	var req dto.UpdateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.UpdateDepartment(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "部门编码已存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "部门名称已存在") {
			errCode = constants.ErrCodeBadRequest
		}

		adc.Error(c, errCode, errMsg)
		return
	}

	adc.SuccessWithMessage(c, "部门更新成功", response)
}

// DeleteDepartment 删除部门（管理员操作）
// DELETE /api/admin/departments/:id
func (adc DepartmentController) DeleteDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err = departmentService.DeleteDepartment(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "存在子部门") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "存在关联用户") {
			errCode = constants.ErrCodeBadRequest
		}

		adc.Error(c, errCode, errMsg)
		return
	}

	adc.SuccessWithMessage(c, "部门删除成功", nil)
}

// GetDepartmentList 获取部门列表（管理员操作）
// GET /api/admin/departments
func (adc DepartmentController) GetDepartmentList(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.DepartmentQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "查询参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetDepartmentList(&req)
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	adc.Success(c, response)
}

// GetDepartmentTree 获取部门树形结构（管理员操作）
// GET /api/admin/departments/tree
func (adc DepartmentController) GetDepartmentTree(c *gin.Context, helper interfaces.HelperInterface) {
	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetDepartmentTree()
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	adc.Success(c, response)
}

// UpdateDepartmentStatus 更新部门状态（管理员操作）
// PUT /api/admin/departments/:id/status
func (adc DepartmentController) UpdateDepartmentStatus(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	var req dto.DepartmentStatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err = departmentService.UpdateDepartmentStatus(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeBadRequest
		if !strings.Contains(err.Error(), "部门不存在") {
			errCode = constants.ErrCodeInternal
		}
		adc.Error(c, errCode, err.Error())
		return
	}

	adc.SuccessWithMessage(c, "部门状态更新成功", nil)
}

// BatchOperation 批量操作部门（管理员操作）
// POST /api/admin/departments/batch
func (adc DepartmentController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.BatchDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err := departmentService.BatchOperation(&req)
	if err != nil {
		adc.Error(c, constants.ErrCodeBatchOperationFailed, err.Error())
		return
	}

	adc.SuccessWithMessage(c, "批量操作成功", nil)
}

// MoveDepartment 移动部门（管理员操作）
// PUT /api/admin/departments/:id/move
func (adc DepartmentController) MoveDepartment(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	var req dto.MoveDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err = departmentService.MoveDepartment(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "目标父部门不存在") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "目标父部门未启用") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "不能移动到子部门下") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "目标位置已存在同名部门") {
			errCode = constants.ErrCodeBadRequest
		}

		adc.Error(c, errCode, errMsg)
		return
	}

	adc.SuccessWithMessage(c, "部门移动成功", nil)
}

// GetChildrenDepartments 获取子部门列表（管理员操作）
// GET /api/admin/departments/:id/children
func (adc DepartmentController) GetChildrenDepartments(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetChildrenDepartments(uint(id))
	if err != nil {
		errCode := constants.ErrCodeBadRequest
		if !strings.Contains(err.Error(), "部门不存在") {
			errCode = constants.ErrCodeInternal
		}
		adc.Error(c, errCode, err.Error())
		return
	}

	adc.Success(c, response)
}

// GetDescendantDepartments 获取所有后代部门（管理员操作）
// GET /api/admin/departments/:id/descendants
func (adc DepartmentController) GetDescendantDepartments(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "部门ID格式错误")
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetDescendantDepartments(uint(id))
	if err != nil {
		errCode := constants.ErrCodeBadRequest
		if !strings.Contains(err.Error(), "部门不存在") {
			errCode = constants.ErrCodeInternal
		}
		adc.Error(c, errCode, err.Error())
		return
	}

	adc.Success(c, response)
}

// GetRootDepartments 获取根部门列表（管理员操作）
// GET /api/admin/departments/roots
func (adc DepartmentController) GetRootDepartments(c *gin.Context, helper interfaces.HelperInterface) {
	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetRootDepartments()
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	adc.Success(c, response)
}

// GetDepartmentsByManager 根据负责人获取部门列表（管理员操作）
// GET /api/admin/managers/:managerId/departments
func (adc DepartmentController) GetDepartmentsByManager(c *gin.Context, helper interfaces.HelperInterface) {
	managerIdStr := c.Param("managerId")
	managerId, err := strconv.ParseUint(managerIdStr, 10, 32)
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "负责人ID格式错误")
		return
	}

	departmentService := service.NewDepartmentService(helper.GetDatabase())
	response, err := departmentService.GetDepartmentsByManager(uint(managerId))
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	adc.Success(c, response)
}

// GetActiveDepartmentsCount 获取启用部门数量统计（管理员操作）
// GET /api/admin/departments/stats/active-count
func (adc DepartmentController) GetActiveDepartmentsCount(c *gin.Context, helper interfaces.HelperInterface) {
	departmentService := service.NewDepartmentService(helper.GetDatabase())
	count, err := departmentService.GetActiveDepartmentsCount()
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}

	adc.Success(c, gin.H{
		"activeDepartmentsCount": count,
	})
}

// ValidateHierarchy 验证部门层级结构完整性（管理员操作）
// POST /api/admin/departments/validate-hierarchy
func (adc DepartmentController) ValidateHierarchy(c *gin.Context, helper interfaces.HelperInterface) {
	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err := departmentService.ValidateDepartmentHierarchy()
	if err != nil {
		adc.Error(c, constants.ErrCodeBadRequest, "层级结构验证失败: "+err.Error())
		return
	}

	adc.SuccessWithMessage(c, "部门层级结构验证通过", nil)
}

// RepairHierarchy 修复部门层级结构（管理员操作）
// POST /api/admin/departments/repair-hierarchy
func (adc DepartmentController) RepairHierarchy(c *gin.Context, helper interfaces.HelperInterface) {
	departmentService := service.NewDepartmentService(helper.GetDatabase())
	err := departmentService.RepairDepartmentHierarchy()
	if err != nil {
		adc.Error(c, constants.ErrCodeInternal, "修复部门层级结构失败: "+err.Error())
		return
	}

	adc.SuccessWithMessage(c, "部门层级结构修复成功", nil)
}
