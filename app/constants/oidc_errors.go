package constants

import "errors"

// OIDC/OAuth2 标准错误
// 这些错误遵循 RFC 6749 (OAuth 2.0) 和 OpenID Connect Core 1.0 规范
var (
	// 请求格式错误或缺少必需参数
	ErrInvalidRequest = errors.New("invalid_request")

	// 客户端认证失败
	ErrInvalidClient = errors.New("invalid_client")

	// 授权码、刷新令牌或其他授权凭证无效、过期、已撤销或不匹配
	ErrInvalidGrant = errors.New("invalid_grant")

	// 客户端未被授权使用此授权类型
	ErrUnauthorizedClient = errors.New("unauthorized_client")

	// 不支持的授权类型
	ErrUnsupportedGrantType = errors.New("unsupported_grant_type")

	// 请求的作用域无效、未知或格式错误
	ErrInvalidScope = errors.New("invalid_scope")

	// 服务器内部错误
	ErrServerError = errors.New("server_error")

	// 不支持的响应类型
	ErrUnsupportedResponseType = errors.New("unsupported_response_type")

	// 资源所有者或授权服务器拒绝请求
	ErrAccessDenied = errors.New("access_denied")

	// 不支持的令牌类型
	ErrUnsupportedTokenType = errors.New("unsupported_token_type")

	// 无效的令牌
	ErrInvalidToken = errors.New("invalid_token")

	// 权限不足
	ErrInsufficientScope = errors.New("insufficient_scope")
)
