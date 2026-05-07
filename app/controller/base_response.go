package controller

import (
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type BaseResponse struct {
}

// Success 成功响应
func (receiver BaseResponse) Success(c httpInterfaces.RouterContextInterface, data any) {
	c.JSON(http.StatusOK, dto.Response{
		Code:    constants.ErrCodeSuccess,
		Message: constants.GetErrMessage(constants.ErrCodeSuccess),
		Data:    data,
	})
}

// SuccessWithMessage 带自定义消息的成功响应
func (receiver BaseResponse) SuccessWithMessage(c httpInterfaces.RouterContextInterface, message string, data any) {
	c.JSON(http.StatusOK, dto.Response{
		Code:    constants.ErrCodeSuccess,
		Message: message,
		Data:    data,
	})
}

// Error 错误响应
func (receiver BaseResponse) Error(c httpInterfaces.RouterContextInterface, code int, message string) {
	httpStatus := receiver.getHTTPStatus(code)
	if message == "" {
		message = constants.GetErrMessage(code)
	}

	c.JSON(httpStatus, dto.Response{
		Code:    code,
		Message: message,
	})
}

// ErrorWithData 带数据的错误响应
func (receiver BaseResponse) ErrorWithData(c httpInterfaces.RouterContextInterface, code int, message string, data any) {
	httpStatus := receiver.getHTTPStatus(code)
	if message == "" {
		message = constants.GetErrMessage(code)
	}

	c.JSON(httpStatus, dto.Response{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

// getHTTPStatus 根据业务错误码获取HTTP状态码
func (receiver BaseResponse) getHTTPStatus(code int) int {
	if code >= 400 && code < 600 {
		return code
	}
	return http.StatusOK
}
