package admin

import (
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// MigrationController 数据迁移模块管理端控制器。
type MigrationController struct {
	BaseController
}

// ListSources GET /api/admin/migration/sources
func (mc MigrationController) ListSources(c httpInterfaces.RouterContextInterface) {
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	sources, err := svc.List()
	if err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.Success(c, sources)
}

// CreateSource POST /api/admin/migration/sources
func (mc MigrationController) CreateSource(c httpInterfaces.RouterContextInterface) {
	var req dto.CreateMigrationSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	source, err := svc.Create(&req)
	if err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.SuccessWithMessage(c, "迁移源创建成功", source)
}

// GetSource GET /api/admin/migration/sources/:id
func (mc MigrationController) GetSource(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	source, err := svc.Get(id)
	if err != nil {
		mc.Error(c, constants.ErrCodeNotFound, "迁移源不存在")
		return
	}
	mc.Success(c, source)
}

// UpdateSource PUT /api/admin/migration/sources/:id
func (mc MigrationController) UpdateSource(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateMigrationSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	source, err := svc.Update(id, &req)
	if err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.SuccessWithMessage(c, "迁移源更新成功", source)
}

// DeleteSource DELETE /api/admin/migration/sources/:id
func (mc MigrationController) DeleteSource(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	if err := svc.Delete(id); err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.SuccessWithMessage(c, "迁移源删除成功", nil)
}

// TestScript POST /api/admin/migration/test-script
func (mc MigrationController) TestScript(c httpInterfaces.RouterContextInterface) {
	var req dto.TestScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	out, err := svc.TestScript(&req)
	if err != nil {
		mc.ErrorWithData(c, constants.ErrCodeBadRequest, "脚本执行失败: "+err.Error(), nil)
		return
	}
	mc.Success(c, map[string]any{"output": out})
}

// TestConnection POST /api/admin/migration/test-connection
func (mc MigrationController) TestConnection(c httpInterfaces.RouterContextInterface) {
	var req dto.TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	svc := service.NewMigrationSourceService(helper.GetDatabase())
	result, err := svc.TestConnection(req.ConnectionConfig)
	if err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	mc.Success(c, result)
}

// StartImport POST /api/admin/migration/sources/:id/import
func (mc MigrationController) StartImport(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	svc := service.NewMigrationImportService(helper.GetDatabase())
	task, err := svc.StartImport(id)
	if err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	mc.SuccessWithMessage(c, "导入任务已启动", task)
}

// ListTasks GET /api/admin/migration/tasks
func (mc MigrationController) ListTasks(c httpInterfaces.RouterContextInterface) {
	sourceID := parseUintQuery(c.Query("sourceId"))
	page := parseIntQueryDefault(c.Query("page"), 1)
	pageSize := parseIntQueryDefault(c.Query("pageSize"), 20)

	taskDao := dao.NewMigrationTaskDao(helper.GetDatabase())
	tasks, total, err := taskDao.List(sourceID, page, pageSize)
	if err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.Success(c, map[string]any{"items": tasks, "total": total, "page": page, "pageSize": pageSize})
}

// GetTask GET /api/admin/migration/tasks/:id
func (mc MigrationController) GetTask(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	taskDao := dao.NewMigrationTaskDao(helper.GetDatabase())
	task, err := taskDao.GetByID(id)
	if err != nil {
		mc.Error(c, constants.ErrCodeNotFound, "任务不存在")
		return
	}
	mc.Success(c, task)
}

// CancelTask POST /api/admin/migration/tasks/:id/cancel
func (mc MigrationController) CancelTask(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	svc := service.NewMigrationImportService(helper.GetDatabase())
	if err := svc.CancelImport(id); err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	mc.SuccessWithMessage(c, "已请求取消任务", nil)
}

// ListLegacyUsers GET /api/admin/migration/legacy-users
func (mc MigrationController) ListLegacyUsers(c httpInterfaces.RouterContextInterface) {
	q := &dao.LegacyUserQuery{
		SourceID: parseUintQuery(c.Query("sourceId")),
		Username: c.Query("username"),
		Status:   c.Query("status"),
		Page:     parseIntQueryDefault(c.Query("page"), 1),
		PageSize: parseIntQueryDefault(c.Query("pageSize"), 20),
	}
	legacyDao := dao.NewLegacyUserDao(helper.GetDatabase())
	users, total, err := legacyDao.List(q)
	if err != nil {
		mc.Error(c, constants.ErrCodeInternal, err.Error())
		return
	}
	mc.Success(c, map[string]any{"items": users, "total": total, "page": q.Page, "pageSize": q.PageSize})
}

// GetLegacyUser GET /api/admin/migration/legacy-users/:id
func (mc MigrationController) GetLegacyUser(c httpInterfaces.RouterContextInterface) {
	id, ok := mc.parseID(c)
	if !ok {
		return
	}
	legacyDao := dao.NewLegacyUserDao(helper.GetDatabase())
	u, err := legacyDao.GetByID(id)
	if err != nil {
		mc.Error(c, constants.ErrCodeNotFound, "旧用户不存在")
		return
	}
	mc.Success(c, u)
}

func (mc MigrationController) parseID(c httpInterfaces.RouterContextInterface) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		mc.Error(c, constants.ErrCodeBadRequest, "ID 格式错误")
		return 0, false
	}
	return uint(id), true
}

func parseUintQuery(s string) uint {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint(v)
}

func parseIntQueryDefault(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
