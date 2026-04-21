package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/open/go-web/pkg/server/reload"
	"github.com/gin-gonic/gin"
)

// SystemController 系统管理控制器
type SystemController struct {
	BaseController
}

// ReloadConfigResponse 重载配置响应
type ReloadConfigResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ReloadConfig 重新加载配置并重启服务
// POST /api/admin/system/reload
func (sc SystemController) ReloadConfig(c *gin.Context, helper interfaces.HelperInterface) {
	// 记录操作日志
	helper.GetLogger().Info("管理员触发配置重载")

	// 异步触发重启。go-web 的 TriggerReload 不返回状态；若通道已满会静默丢弃。
	go reload.TriggerReload()

	// 立即返回响应
	sc.Success(c, ReloadConfigResponse{
		Message: "配置重载请求已接受，服务将在短时间内重启",
		Status:  "accepted",
	})
}

// GetSystemInfo 获取系统信息
// GET /api/admin/system/info
func (sc SystemController) GetSystemInfo(c *gin.Context, helper interfaces.HelperInterface) {
	sc.Success(c, gin.H{
		"version": "1.0.0",
		"status":  "running",
	})
}
