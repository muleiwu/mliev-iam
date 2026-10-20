package admin

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type AppearanceThemeController struct{ controller.BaseResponse }

func themeKitFS() fs.FS {
	config := helper.GetConfig()
	if config.GetString("http.static_mode", "embed") == "disk" {
		return os.DirFS("static/auth/theme-kit")
	}
	files, ok := config.Get("static.fs", nil).(map[string]embed.FS)
	if !ok {
		return nil
	}
	web, ok := files["web.static"]
	if !ok {
		return nil
	}
	sub, err := fs.Sub(web, "static/auth/theme-kit")
	if err != nil {
		return nil
	}
	return sub
}
func (c AppearanceThemeController) respondError(ctx httpInterfaces.RouterContextInterface, err error) {
	code := 500
	switch {
	case errors.Is(err, service.ErrThemePackage):
		code = 400
	case errors.Is(err, service.ErrThemeConflict), errors.Is(err, service.ErrThemeInUse):
		code = 409
	case errors.Is(err, service.ErrThemeNotFound):
		code = 404
	}
	c.Error(ctx, code, err.Error())
}
func (c AppearanceThemeController) Export(ctx httpInterfaces.RouterContextInterface) {
	ctx.Request().Body = http.MaxBytesReader(ctx.ResponseWriter(), ctx.Request().Body, 256<<10)
	var request dto.ThemeExportRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		c.Error(ctx, 400, "导出配置格式错误或超过限制")
		return
	}
	// Used only to resolve references, never as a remote fetch destination.
	scheme := "http"
	if ctx.Request().TLS != nil {
		scheme = "https"
	}
	if forwarded := ctx.GetHeader("X-Forwarded-Proto"); forwarded == "https" {
		scheme = "https"
	}
	data, err := service.NewAppearanceThemeService(helper.GetDatabase(), themeKitFS()).Export(ctx.Request().Context(), request, scheme+"://"+ctx.Request().Host)
	if err != nil {
		c.respondError(ctx, err)
		return
	}
	ctx.SetHeader("Content-Type", "application/zip")
	ctx.SetHeader("Content-Disposition", `attachment; filename="iam-theme.zip"`)
	ctx.SetHeader("Cache-Control", "no-store")
	ctx.ResponseWriter().WriteHeader(http.StatusOK)
	_, _ = ctx.ResponseWriter().Write(data)
}
func (c AppearanceThemeController) Import(ctx httpInterfaces.RouterContextInterface) {
	ctx.Request().Body = http.MaxBytesReader(ctx.ResponseWriter(), ctx.Request().Body, service.ThemePackageMaxZip+(1<<20))
	file, err := ctx.FormFile("file")
	if ctx.Request().MultipartForm != nil {
		defer ctx.Request().MultipartForm.RemoveAll()
	}
	if err != nil || file.Size > service.ThemePackageMaxZip {
		c.Error(ctx, 400, "请选择不超过 32 MiB 的主题 ZIP")
		return
	}
	stream, err := file.Open()
	if err != nil {
		c.Error(ctx, 400, "无法读取主题包")
		return
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, service.ThemePackageMaxZip+1))
	if err != nil {
		c.Error(ctx, 400, "读取主题包失败")
		return
	}
	userID, _ := middleware.GetCurrentUserID(ctx)
	theme, err := service.NewAppearanceThemeService(helper.GetDatabase(), nil).Import(ctx.Request().Context(), data, userID)
	if err != nil {
		c.respondError(ctx, err)
		return
	}
	c.Success(ctx, theme)
}
func (c AppearanceThemeController) List(ctx httpInterfaces.RouterContextInterface) {
	page, _ := strconv.Atoi(ctx.Query("page"))
	size, _ := strconv.Atoi(ctx.Query("pageSize"))
	if page > 1_000_000 {
		c.Error(ctx, 400, "页码过大")
		return
	}
	result, err := service.NewAppearanceThemeService(helper.GetDatabase(), nil).List(page, size)
	if err != nil {
		c.respondError(ctx, err)
		return
	}
	ctx.SetHeader("Cache-Control", "no-store")
	c.Success(ctx, result)
}
func (c AppearanceThemeController) Get(ctx httpInterfaces.RouterContextInterface) {
	result, err := service.NewAppearanceThemeService(helper.GetDatabase(), nil).Get(ctx.Param("id"))
	if err != nil {
		c.respondError(ctx, err)
		return
	}
	ctx.SetHeader("Cache-Control", "no-store")
	c.Success(ctx, result)
}
func (c AppearanceThemeController) Delete(ctx httpInterfaces.RouterContextInterface) {
	if err := service.NewAppearanceThemeService(helper.GetDatabase(), nil).Delete(ctx.Param("id")); err != nil {
		c.respondError(ctx, err)
		return
	}
	c.Success(ctx, map[string]bool{"deleted": true})
}
