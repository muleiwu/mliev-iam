package constants

// 错误码定义
const (
	// 成功
	ErrCodeSuccess = 0

	// 客户端错误 4xx
	ErrCodeBadRequest   = 400 // 请求参数错误
	ErrCodeUnauthorized = 401 // 未授权
	ErrCodeForbidden    = 403 // 禁止访问
	ErrCodeNotFound     = 404 // 资源不存在
	ErrCodeConflict     = 409 // 资源冲突

	// 服务器错误 5xx
	ErrCodeInternal    = 500 // 内部服务器错误
	ErrCodeBadGateway  = 502 // 网关错误
	ErrCodeUnavailable = 503 // 服务不可用

	// 用户相关错误码 1000-1999
	ErrCodeUserNotFound         = 1001 // 用户不存在
	ErrCodeUserAlreadyExists    = 1002 // 用户已存在
	ErrCodeEmailAlreadyExists   = 1003 // 邮箱已存在
	ErrCodePhoneAlreadyExists   = 1004 // 手机号已存在
	ErrCodeInvalidPassword      = 1005 // 密码错误
	ErrCodeUserDisabled         = 1006 // 用户被禁用
	ErrCodeUserNotActivated     = 1008 // 用户未激活
	ErrCodePasswordExpired      = 1009 // 密码已过期
	ErrCodeInvalidUserStatus    = 1010 // 无效的用户状态
	ErrCodeUserValidationFailed = 1011 // 用户验证失败
	ErrCodeBatchOperationFailed = 1012 // 批量操作失败
)

// 错误信息映射
var ErrMessages = map[int]string{
	ErrCodeSuccess:      "操作成功",
	ErrCodeBadRequest:   "请求参数错误",
	ErrCodeUnauthorized: "未授权访问",
	ErrCodeForbidden:    "禁止访问",
	ErrCodeNotFound:     "资源不存在",
	ErrCodeConflict:     "资源冲突",
	ErrCodeInternal:     "内部服务器错误",
	ErrCodeBadGateway:   "网关错误",
	ErrCodeUnavailable:  "服务暂不可用",

	// 用户相关错误信息
	ErrCodeUserNotFound:         "用户不存在",
	ErrCodeUserAlreadyExists:    "用户名已存在",
	ErrCodeEmailAlreadyExists:   "邮箱已存在",
	ErrCodePhoneAlreadyExists:   "手机号已存在",
	ErrCodeInvalidPassword:      "密码错误",
	ErrCodeUserDisabled:         "用户账户已被禁用",
	ErrCodeUserNotActivated:     "用户账户未激活",
	ErrCodePasswordExpired:      "密码已过期",
	ErrCodeInvalidUserStatus:    "用户状态无效",
	ErrCodeUserValidationFailed: "用户信息验证失败",
	ErrCodeBatchOperationFailed: "批量操作失败",
}

// GetErrMessage 获取错误信息
func GetErrMessage(code int) string {
	if msg, exists := ErrMessages[code]; exists {
		return msg
	}
	return "未知错误"
}
