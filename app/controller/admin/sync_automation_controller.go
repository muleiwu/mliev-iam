package admin

import (
	"strconv"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type SyncAutomationController struct{ controller.BaseResponse }

func (sc SyncAutomationController) ListJobs(c httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	connectionID, _ := strconv.ParseUint(c.Query("connectionId"), 10, 64)
	items, total, err := service.NewSyncJobService(helper.GetDatabase()).List(service.SyncJobListFilter{
		ConnectionID: uint(connectionID), JobType: model.SyncJobType(c.Query("jobType")),
		Status: model.SyncJobStatus(c.Query("status")), Page: page, PageSize: pageSize,
	})
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (sc SyncAutomationController) CreateJob(c httpInterfaces.RouterContextInterface) {
	var req struct {
		ConnectionID uint              `json:"connectionId" binding:"required"`
		JobType      model.SyncJobType `json:"jobType" binding:"required"`
		Payload      map[string]any    `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	job, err := service.NewSyncJobService(helper.GetDatabase()).Enqueue(req.ConnectionID, req.JobType, req.Payload, time.Now(), nil)
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	sc.SuccessWithMessage(c, "同步任务已入队", job)
}

func (sc SyncAutomationController) RetryJob(c httpInterfaces.RouterContextInterface) {
	sc.jobAction(c, "retry")
}

func (sc SyncAutomationController) CancelJob(c httpInterfaces.RouterContextInterface) {
	sc.jobAction(c, "cancel")
}

func (sc SyncAutomationController) jobAction(c httpInterfaces.RouterContextInterface, action string) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "无效的任务 ID")
		return
	}
	jobService := service.NewSyncJobService(helper.GetDatabase())
	if action == "retry" {
		err = jobService.Retry(id)
	} else {
		err = jobService.Cancel(id)
	}
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	sc.SuccessWithMessage(c, "任务状态已更新", nil)
}

func (sc SyncAutomationController) ListSchedules(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	items, err := service.NewSyncScheduleService(helper.GetDatabase()).List(connectionID)
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, items)
}

func (sc SyncAutomationController) SaveSchedules(c httpInterfaces.RouterContextInterface) {
	connectionID, err := parseUintParam(c, "id")
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "无效的连接 ID")
		return
	}
	var req struct {
		Schedules []service.SyncScheduleInput `json:"schedules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	items, err := service.NewSyncScheduleService(helper.GetDatabase()).Replace(connectionID, req.Schedules)
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	sc.SuccessWithMessage(c, "同步计划已保存", items)
}

func (sc SyncAutomationController) ListEvents(c httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	connectionID, _ := strconv.ParseUint(c.Query("connectionId"), 10, 64)
	items, total, err := service.NewExternalEventService(helper.GetDatabase()).List(service.ExternalEventListFilter{
		ConnectionID: uint(connectionID), Status: model.ExternalEventStatus(c.Query("status")),
		EventType: c.Query("eventType"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (sc SyncAutomationController) ReplayEvent(c httpInterfaces.RouterContextInterface) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, "无效的事件 ID")
		return
	}
	if err := service.NewExternalEventService(helper.GetDatabase()).Replay(id); err != nil {
		sc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	sc.SuccessWithMessage(c, "事件已重新入队", nil)
}

func (sc SyncAutomationController) Summary(c httpInterfaces.RouterContextInterface) {
	response, err := service.NewSyncCenterService(helper.GetDatabase()).Summary()
	if err != nil {
		sc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	sc.Success(c, response)
}
