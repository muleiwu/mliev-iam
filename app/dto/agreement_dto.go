package dto

// AgreementUpsertRequest 新增或更新协议请求。
type AgreementUpsertRequest struct {
	Code      string `json:"code,omitempty" binding:"omitempty,max=100"`
	Title     string `json:"title" binding:"required,max=200"`
	ContentMD string `json:"contentMd"`
	Enabled   bool   `json:"enabled"`
	Sort      int    `json:"sort"`
}

// AgreementResponse 管理端协议响应。
type AgreementResponse struct {
	ID            uint   `json:"id"`
	Scope         string `json:"scope"`
	ApplicationID uint   `json:"applicationId"`
	Code          string `json:"code"`
	Title         string `json:"title"`
	ContentMD     string `json:"contentMd"`
	Enabled       bool   `json:"enabled"`
	Sort          int    `json:"sort"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// AgreementPublicResponse 公开协议页面响应。
type AgreementPublicResponse struct {
	Scope     string `json:"scope"`
	Code      string `json:"code"`
	Title     string `json:"title"`
	ContentMD string `json:"contentMd"`
	UpdatedAt string `json:"updatedAt"`
	AppName   string `json:"appName,omitempty"`
}

// ClientAgreementLink OIDC 前端注册页协议链接。
type ClientAgreementLink struct {
	Key   string `json:"key"`
	Scope string `json:"scope"`
	Code  string `json:"code"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
