package controller

import (
	"errors"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AttachmentController 附件控制器：公开 302 访问 + 用户自助头像上传。
type AttachmentController struct {
	BaseResponse
}

// RedirectAttachment 302 跳转到附件真实存储地址
// GET /attachments/:uuid
func (c AttachmentController) RedirectAttachment(ctx httpInterfaces.RouterContextInterface) {
	attachmentUUID := ctx.Param("uuid")
	if attachmentUUID == "" {
		c.Error(ctx, constants.ErrCodeBadRequest, "缺少附件标识")
		return
	}

	attachmentService := service.NewAttachmentService(helper.GetDatabase())
	url, err := attachmentService.ResolveRedirectURL(ctx.Request().Context(), attachmentUUID)
	if err != nil {
		if errors.Is(err, service.ErrAttachmentNotFound) {
			ctx.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	// 预签名地址 15 分钟过期，限制浏览器缓存时间避免缓存失效链接
	ctx.SetHeader("Cache-Control", "private, max-age=300")
	ctx.Redirect(http.StatusFound, url)
}

// UploadMyAvatar 当前登录用户上传头像（两步式：上传拿到地址后由 PUT /api/user/profile 持久化）
// POST /api/user/avatar
func (c AttachmentController) UploadMyAvatar(ctx httpInterfaces.RouterContextInterface) {
	userID, ok := middleware.GetCurrentUserID(ctx)
	if !ok {
		c.Error(ctx, constants.ErrCodeUnauthorized, "未登录或登录已过期")
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请选择要上传的图片")
		return
	}

	attachmentService := service.NewAttachmentService(helper.GetDatabase())
	response, err := attachmentService.UploadAvatar(ctx.Request().Context(), file, userID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	c.Success(ctx, response)
}
