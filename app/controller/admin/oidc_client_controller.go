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

// OIDCClientController OIDC客户端管理控制器（管理员）
type OIDCClientController struct {
	BaseController
}

// CreateClient 创建OIDC客户端
// POST /api/admin/oidc/clients
func (occ OIDCClientController) CreateClient(c *gin.Context, helper interfaces.HelperInterface) {
	var req dto.CreateOIDCClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 获取当前管理员用户ID（这里应该从认证中间件获取）
	// 暂时使用查询参数模拟
	adminUserIDStr := c.Query("adminUserId")
	if adminUserIDStr == "" {
		occ.Error(c, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	adminUserID, err := strconv.ParseUint(adminUserIDStr, 10, 32)
	if err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "管理员用户ID格式错误")
		return
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())
	response, err := clientService.CreateClient(&req, uint(adminUserID))
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "客户端ID已存在") {
			errCode = constants.ErrCodeConflict
		} else if strings.Contains(errMsg, "请求参数错误") {
			errCode = constants.ErrCodeBadRequest
		}

		occ.Error(c, errCode, errMsg)
		return
	}

	occ.SuccessWithMessage(c, "OIDC客户端创建成功", response)
}

// GetClientList 获取OIDC客户端列表
// GET /api/admin/oidc/clients
func (occ OIDCClientController) GetClientList(c *gin.Context, helper interfaces.HelperInterface) {
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

	// 解析状态过滤参数
	var isActive *bool
	if statusStr := c.Query("isActive"); statusStr != "" {
		status, err := strconv.ParseBool(statusStr)
		if err == nil {
			isActive = &status
		}
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())
	clients, total, err := clientService.ListClients(offset, pageSize, isActive)
	if err != nil {
		occ.Error(c, constants.ErrCodeInternal, "获取OIDC客户端列表失败: "+err.Error())
		return
	}

	response := map[string]interface{}{
		"clients": clients,
		"pagination": map[string]interface{}{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	}

	occ.Success(c, response)
}

// GetClient 获取OIDC客户端详情
// GET /api/admin/oidc/clients/:id
func (occ OIDCClientController) GetClient(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "客户端ID格式错误")
		return
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())
	response, err := clientService.GetClient(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "客户端不存在") {
			errCode = constants.ErrCodeNotFound
		}
		occ.Error(c, errCode, err.Error())
		return
	}

	occ.Success(c, response)
}

// UpdateClient 更新OIDC客户端
// PUT /api/admin/oidc/clients/:id
func (occ OIDCClientController) UpdateClient(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "客户端ID格式错误")
		return
	}

	var req dto.UpdateOIDCClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())
	response, err := clientService.UpdateClient(uint(id), &req)
	if err != nil {
		errCode := constants.ErrCodeInternal
		errMsg := err.Error()

		if strings.Contains(errMsg, "客户端不存在") {
			errCode = constants.ErrCodeNotFound
		} else if strings.Contains(errMsg, "请求参数错误") {
			errCode = constants.ErrCodeBadRequest
		}

		occ.Error(c, errCode, errMsg)
		return
	}

	occ.SuccessWithMessage(c, "OIDC客户端更新成功", response)
}

// DeleteClient 删除OIDC客户端
// DELETE /api/admin/oidc/clients/:id
func (occ OIDCClientController) DeleteClient(c *gin.Context, helper interfaces.HelperInterface) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "客户端ID格式错误")
		return
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())
	err = clientService.DeleteClient(uint(id))
	if err != nil {
		errCode := constants.ErrCodeInternal
		if strings.Contains(err.Error(), "客户端不存在") {
			errCode = constants.ErrCodeNotFound
		}
		occ.Error(c, errCode, err.Error())
		return
	}

	occ.SuccessWithMessage(c, "OIDC客户端删除成功", nil)
}

// GetStats 获取OIDC统计信息
// GET /api/admin/oidc/stats
func (occ OIDCClientController) GetStats(c *gin.Context, helper interfaces.HelperInterface) {
	managementService := service.NewOIDCManagementService(helper.GetDatabase())
	stats, err := managementService.GetStats()
	if err != nil {
		occ.Error(c, constants.ErrCodeInternal, "获取统计信息失败: "+err.Error())
		return
	}

	occ.Success(c, stats)
}

// CleanupExpiredData 清理过期数据
// POST /api/admin/oidc/cleanup
func (occ OIDCClientController) CleanupExpiredData(c *gin.Context, helper interfaces.HelperInterface) {
	managementService := service.NewOIDCManagementService(helper.GetDatabase())
	err := managementService.CleanupExpiredData()
	if err != nil {
		occ.Error(c, constants.ErrCodeInternal, "清理过期数据失败: "+err.Error())
		return
	}

	occ.SuccessWithMessage(c, "过期数据清理成功", nil)
}

// BatchOperation 批量操作OIDC客户端
// POST /api/admin/oidc/clients/batch
func (occ OIDCClientController) BatchOperation(c *gin.Context, helper interfaces.HelperInterface) {
	var req struct {
		Action string `json:"action" binding:"required,oneof=enable disable delete"`
		IDs    []uint `json:"ids" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		occ.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	clientService := service.NewOIDCClientService(helper.GetDatabase())

	switch req.Action {
	case "enable":
		// 批量启用客户端
		for _, id := range req.IDs {
			updateReq := dto.UpdateOIDCClientRequest{
				IsActive: boolPtr(true),
			}
			_, err := clientService.UpdateClient(id, &updateReq)
			if err != nil {
				occ.Error(c, constants.ErrCodeInternal, "批量启用失败: "+err.Error())
				return
			}
		}
	case "disable":
		// 批量禁用客户端
		for _, id := range req.IDs {
			updateReq := dto.UpdateOIDCClientRequest{
				IsActive: boolPtr(false),
			}
			_, err := clientService.UpdateClient(id, &updateReq)
			if err != nil {
				occ.Error(c, constants.ErrCodeInternal, "批量禁用失败: "+err.Error())
				return
			}
		}
	case "delete":
		// 批量删除客户端
		for _, id := range req.IDs {
			err := clientService.DeleteClient(id)
			if err != nil {
				occ.Error(c, constants.ErrCodeInternal, "批量删除失败: "+err.Error())
				return
			}
		}
	}

	occ.SuccessWithMessage(c, "批量操作完成", nil)
}

// 辅助函数
func boolPtr(b bool) *bool {
	return &b
}
