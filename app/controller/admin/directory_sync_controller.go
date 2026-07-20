package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type DirectorySyncController struct{ controller.BaseResponse }

func (dc DirectorySyncController) GetPolicy(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).GetPolicy(connectionID)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.Success(c, response)
}

func (dc DirectorySyncController) SavePolicy(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	var req dto.DirectoryPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).SavePolicy(connectionID, &req)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.SuccessWithMessage(c, "目录同步策略已保存", response)
}

func (dc DirectorySyncController) Diagnostic(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).Diagnostic(c.Request().Context(), connectionID)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.Success(c, response)
}

func (dc DirectorySyncController) CreateRun(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	var actor *uint
	if ok {
		actor = &actorID
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).CreateManualRun(c.Request().Context(), connectionID, actor)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.SuccessWithMessage(c, "同步预览已生成", response)
}

func (dc DirectorySyncController) ListRuns(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	items, total, err := service.NewDirectorySyncService(helper.GetDatabase()).ListRuns(connectionID, page, pageSize)
	if err != nil {
		dc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	dc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (dc DirectorySyncController) GetRun(c httpInterfaces.RouterContextInterface) {
	runID, err := parseUintParam(c, "runId")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的同步 ID")
		return
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).GetRun(runID)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.Success(c, response)
}

func (dc DirectorySyncController) ListChanges(c httpInterfaces.RouterContextInterface) {
	runID, err := parseUintParam(c, "runId")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的同步 ID")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	items, total, err := service.NewDirectorySyncService(helper.GetDatabase()).ListChanges(runID, c.Query("status"), c.Query("changeType"), c.Query("risk"), page, pageSize)
	if err != nil {
		dc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	dc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (dc DirectorySyncController) ApproveRun(c httpInterfaces.RouterContextInterface) {
	runID, err := parseUintParam(c, "runId")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的同步 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		dc.Error(c, constants.ErrCodeUnauthorized, "未获取到管理员身份")
		return
	}
	if err := service.NewDirectorySyncService(helper.GetDatabase()).ApproveRun(runID, actorID); err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.SuccessWithMessage(c, "安全变更已批量批准，冲突项仍需逐项处理", nil)
}

func (dc DirectorySyncController) ApplyRun(c httpInterfaces.RouterContextInterface) {
	runID, err := parseUintParam(c, "runId")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的同步 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		dc.Error(c, constants.ErrCodeUnauthorized, "未获取到管理员身份")
		return
	}
	response, err := service.NewDirectorySyncService(helper.GetDatabase()).ApplyRun(runID, actorID)
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	dc.SuccessWithMessage(c, "已审批变更应用完成", response)
}

func (dc DirectorySyncController) ApproveChange(c httpInterfaces.RouterContextInterface) {
	dc.decideChange(c, true)
}

func (dc DirectorySyncController) RejectChange(c httpInterfaces.RouterContextInterface) {
	dc.decideChange(c, false)
}

func (dc DirectorySyncController) decideChange(c httpInterfaces.RouterContextInterface, approve bool) {
	changeID, err := parseUintParam(c, "changeId")
	if err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, "无效的变更 ID")
		return
	}
	actorID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		dc.Error(c, constants.ErrCodeUnauthorized, "未获取到管理员身份")
		return
	}
	if err := service.NewDirectorySyncService(helper.GetDatabase()).DecideChange(changeID, approve, actorID); err != nil {
		dc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	message := "变更已拒绝"
	if approve {
		message = "变更已批准"
	}
	dc.SuccessWithMessage(c, message, nil)
}
