package admin

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"cnb.cool/mliev/open/go-web/pkg/server/reload"
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
func (sc SystemController) ReloadConfig(c httpInterfaces.RouterContextInterface) {
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
func (sc SystemController) GetSystemInfo(c httpInterfaces.RouterContextInterface) {
	sc.Success(c, map[string]any{
		"version": "1.0.0",
		"status":  "running",
	})
}
