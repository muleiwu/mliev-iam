package admin

import (
	"errors"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

// ApplicationController 应用管理控制器（管理员）
type ApplicationController struct {
	BaseController
}

// CreateApplication 创建应用
// POST /api/admin/applications
func (ac ApplicationController) CreateApplication(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 从认证中间件获取当前管理员用户ID
	adminUserID, exists := c.Get("user_id")
	if !exists {
		ac.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	userID, ok := adminUserID.(uint)
	if !ok {
		ac.Error(c, constants.ErrCodeInternal, "用户ID类型错误")
		return
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	response, err := appService.CreateApplication(&req, userID)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "不支持的协议类型") {
			errCode = constants.ErrCodeBadRequest
		} else if strings.Contains(errMsg, "请求参数错误") || strings.Contains(errMsg, "至少需要") || strings.Contains(errMsg, "必须包含") {
			errCode = constants.ErrCodeBadRequest
		}

		ac.Error(c, errCode, errMsg)
		return
	}

	ac.SuccessWithMessage(c, "应用创建成功", response)
}

// GetApplicationList 获取应用列表
// GET /api/admin/applications
func (ac ApplicationController) GetApplicationList(c *gin.Context, helper interfaces.HelperInterface) {
	// 解析分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	// 解析协议类型过滤参数
	var protocolType *string
	if pt := c.Query("protocolType"); pt != "" {
		protocolType = &pt
	}

	// 解析状态过滤参数
	var isActive *bool
	if statusStr := c.Query("isActive"); statusStr != "" {
		status, err := strconv.ParseBool(statusStr)
		if err == nil {
			isActive = &status
		}
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	apps, total, err := appService.ListApplications(offset, pageSize, protocolType, isActive)
	if err != nil {
		ac.Error(c, constants.ErrCodeInternal, "获取应用列表失败: "+err.Error())
		return
	}

	response := map[string]interface{}{
		"applications": apps,
		"pagination": map[string]interface{}{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	}

	ac.Success(c, response)
}

// GetApplication 获取应用详情
// GET /api/admin/applications/:id
func (ac ApplicationController) GetApplication(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "应用ID格式错误")
		return
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	response, err := appService.GetApplication(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if errors.Is(err, service.ErrApplicationNotFound) {
			errCode = constants.ErrCodeNotFound
		}
		ac.Error(c, errCode, err.Error())
		return
	}

	ac.Success(c, response)
}

// UpdateApplication 更新应用
// PUT /api/admin/applications/:id
func (ac ApplicationController) UpdateApplication(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "应用ID格式错误")
		return
	}

	var req dto.UpdateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	response, err := appService.UpdateApplication(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal

		if errors.Is(err, service.ErrApplicationNotFound) {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(err.Error(), "请求参数错误") || strings.Contains(err.Error(), "至少需要") || strings.Contains(err.Error(), "必须包含") {
			errCode = constants.ErrCodeBadRequest
		}

		ac.Error(c, errCode, err.Error())
		return
	}

	ac.SuccessWithMessage(c, "应用更新成功", response)
}

// DeleteApplication 删除应用
// DELETE /api/admin/applications/:id
func (ac ApplicationController) DeleteApplication(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "应用ID格式错误")
		return
	}

	appService := service.NewApplicationService(helper.GetDatabase())
	err = appService.DeleteApplication(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if errors.Is(err, service.ErrApplicationNotFound) {
			errCode = constants.ErrCodeNotFound
		}
		ac.Error(c, errCode, err.Error())
		return
	}

	ac.SuccessWithMessage(c, "应用删除成功", nil)
}

// GetStats 获取应用统计信息
// GET /api/admin/applications/stats
func (ac ApplicationController) GetStats(c *gin.Context, helper interfaces.HelperInterface) {
	appService := service.NewApplicationService(helper.GetDatabase())
	stats, err := appService.GetStats()
	if err != nil {
		ac.Error(c, constants.ErrCodeInternal, "获取统计信息失败: "+err.Error())
		return
	}

	ac.Success(c, stats)
}

// BatchOperation 批量操作应用
// POST /api/admin/applications/batch
func (ac ApplicationController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req struct {
		Action string `json:"action" binding:"required,oneof=enable disable delete"`
		IDs    []uint `json:"ids" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		ac.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	appService := service.NewApplicationService(helper.GetDatabase())

	switch req.Action {
	case "enable":
		// 批量启用应用
		for _, id := range req.IDs {
			updateReq := dto.UpdateApplicationRequest{
				IsActive: boolPtr(true),
			}
			_, err := appService.UpdateApplication(id, &updateReq)
			if err != nil {
				ac.Error(c, constants.ErrCodeInternal, "批量启用失败: "+err.Error())
				return
			}
		}
	case "disable":
		// 批量禁用应用
		for _, id := range req.IDs {
			updateReq := dto.UpdateApplicationRequest{
				IsActive: boolPtr(false),
			}
			_, err := appService.UpdateApplication(id, &updateReq)
			if err != nil {
				ac.Error(c, constants.ErrCodeInternal, "批量禁用失败: "+err.Error())
				return
			}
		}
	case "delete":
		// 批量删除应用
		for _, id := range req.IDs {
			err := appService.DeleteApplication(id)
			if err != nil {
				ac.Error(c, constants.ErrCodeInternal, "批量删除失败: "+err.Error())
				return
			}
		}
	}

	ac.SuccessWithMessage(c, "批量操作完成", nil)
}

// 辅助函数
func boolPtr(b bool) *bool {
	return &b
}
