package admin

import (
	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// AttachmentController 管理端附件控制器
type AttachmentController struct {
	controller.BaseResponse
}

// UploadAvatar 管理端上传用户头像（返回 /attachments/{uuid}，由用户创建/更新接口持久化）
// POST /api/admin/attachments/avatar
func (c AttachmentController) UploadAvatar(ctx httpInterfaces.RouterContextInterface) {
	uploaderID, _ := middleware.GetCurrentUserID(ctx)

	file, err := ctx.FormFile("file")
	if err != nil {
		c.Error(ctx, constants.ErrCodeBadRequest, "请选择要上传的图片")
		return
	}

	attachmentService := service.NewAttachmentService(helper.GetDatabase())
	response, err := attachmentService.UploadAvatar(ctx.Request().Context(), file, uploaderID)
	if err != nil {
		c.Error(ctx, constants.ErrCodeInternal, err.Error())
		return
	}

	c.Success(ctx, response)
}
