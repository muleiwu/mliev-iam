package dto

// ==================== OAuth认证流程相关 DTO ====================

// SocialAuthURLResponse OAuth授权URL响应
type SocialAuthURLResponse struct {
	AuthURL string `json:"authUrl"` // 授权URL
	State   string `json:"state"`   // State参数（用于防CSRF）
}

// SocialAuthCallbackRequest OAuth回调请求
type SocialAuthCallbackRequest struct {
	Code  string `form:"code" binding:"required"`
	State string `form:"state" binding:"required"`
}

// SocialAuthCallbackResponse OAuth回调响应（登录成功）
type SocialAuthCallbackResponse struct {
	Token            string        `json:"token"`     // JWT访问令牌
	User             *UserResponse `json:"user"`      // 用户信息
	IsNewUser        bool          `json:"isNewUser"` // 是否为新注册用户
	NeedsBind        bool          `json:"needsBind"` // 是否需要绑定已有账号
	ContinuationID   string        `json:"-"`         // 仅服务端用于设置 HttpOnly continuation Cookie
	ContinuationCSRF string        `json:"continuationCsrf,omitempty"`
	SessionID        string        `json:"sessionId,omitempty"` // OIDC会话ID（用于OIDC流程）
}

// SocialBindWithLoginRequest 用社交账号绑定已有账户
type SocialBindWithLoginRequest struct {
	ContinuationCSRF string         `json:"continuationCsrf" binding:"required"`
	Username         string         `json:"username" binding:"required"`
	Password         string         `json:"password" binding:"required"`
	SessionID        string         `json:"sessionId,omitempty"` // OIDC会话ID
	CaptchaTicket    *CaptchaTicket `json:"captchaTicket,omitempty"`
}

// SocialRegisterRequest 用社交账号注册新账户
type SocialRegisterRequest struct {
	ContinuationCSRF      string         `json:"continuationCsrf" binding:"required"`
	Username              string         `json:"username" binding:"required,min=3,max=20"`
	Email                 string         `json:"email" binding:"required,email"`
	Password              string         `json:"password" binding:"required,min=6"`
	RealName              string         `json:"realName" binding:"required,min=2,max=20"`
	AgreePolicy           bool           `json:"agreePolicy"`
	AcceptedAgreementKeys []string       `json:"acceptedAgreementKeys,omitempty"`
	SessionID             string         `json:"sessionId,omitempty"` // OIDC会话ID
	CaptchaTicket         *CaptchaTicket `json:"captchaTicket,omitempty"`
}

// SocialBindAccountRequest 绑定社交账号请求（已登录用户）
type SocialBindAccountRequest struct {
	ProviderIdentifier string `json:"providerIdentifier" binding:"required"`
}

// SocialUnbindAccountRequest 解绑社交账号请求
type SocialUnbindAccountRequest struct {
	AccountID uint `json:"accountId" binding:"required"`
}

// SocialMiniProgramLoginRequest 小程序登录请求
type SocialMiniProgramLoginRequest struct {
	JsCode    string `json:"jsCode" binding:"required"`
	SessionID string `json:"sessionId,omitempty"`
}

// ==================== 小程序扫码登录（PC 端展示小程序码） ====================

// MPQRCodeCreateRequest PC 创建扫码登录票据
type MPQRCodeCreateRequest struct {
	SessionID  string `json:"sessionId,omitempty"`  // 可选：OIDC 会话 ID
	Page       string `json:"page,omitempty"`       // 可选：小程序打开路径，默认由后端设置
	EnvVersion string `json:"envVersion,omitempty"` // 可选：小程序版本 release/trial/develop，默认 release
}

// MPQRCodeCreateResponse 创建扫码登录票据响应
type MPQRCodeCreateResponse struct {
	ScanHandle string `json:"scanHandle"`       // 只允许出现在二维码中的不透明句柄
	PollToken  string `json:"pollToken"`        // 只交给创建二维码的浏览器
	QRCode     string `json:"qrcode,omitempty"` // data:image/png;base64,... 形式的小程序码（可能为空）
	ExpiresIn  int    `json:"expiresIn"`        // 有效期（秒）
}

// MPQRCodeStatusResponse 扫码状态查询响应
// state 可取值：pending / scanned / confirmed / needs_bind / canceled / expired
type MPQRCodeStatusResponse struct {
	State            string         `json:"state"`
	User             *UserResponse  `json:"-"` // 仅供 controller 签发 HttpOnly Cookie
	ContinuationID   string         `json:"-"` // 仅供 controller 设置 continuation Cookie
	ContinuationCSRF string         `json:"continuationCsrf,omitempty"`
	SocialInfo       *MPSocialBrief `json:"socialInfo,omitempty"`
	SessionID        string         `json:"sessionId,omitempty"`
}

// MPSocialBrief 供 PC 在"注册/绑定"抉择页展示的最小社交资料
type MPSocialBrief struct {
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

// MPQRCodeScanRequest 小程序扫码后上报
type MPQRCodeScanRequest struct {
	ScanHandle string         `json:"scanHandle" binding:"required"`
	JsCode     string         `json:"jsCode" binding:"required"`
	UserInfo   *MPSocialBrief `json:"userInfo,omitempty"` // 小程序 wx.getUserProfile 获取到的昵称/头像
}

// MPQRCodeScanResponse 扫码上报响应（小程序端使用）
type MPQRCodeScanResponse struct {
	State         string `json:"state"`                   // scanned / already_bound / not_bound
	Nickname      string `json:"nickname,omitempty"`      // 后台根据 openid 找到的用户昵称（若已绑定）
	Avatar        string `json:"avatar,omitempty"`        // 同上
	PendingAction string `json:"pendingAction,omitempty"` // login / bind
	ConfirmToken  string `json:"confirmToken"`            // 仅返回给扫码设备
}

// MPQRCodeConfirmRequest 小程序端确认或取消
type MPQRCodeConfirmRequest struct {
	ScanHandle   string `json:"scanHandle" binding:"required"`
	ConfirmToken string `json:"confirmToken" binding:"required"`
	Action       string `json:"action" binding:"required,oneof=confirm cancel"`
}

// OIDCDiscoverRequest OIDC 发现请求
type OIDCDiscoverRequest struct {
	Issuer string `json:"issuer" binding:"required"`
}

// OIDCDiscoverResponse OIDC 发现响应
type OIDCDiscoverResponse struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	UserinfoEndpoint      string   `json:"userinfoEndpoint"`
	JwksURI               string   `json:"jwksUri"`
	ScopesSupported       []string `json:"scopesSupported,omitempty"`
}
