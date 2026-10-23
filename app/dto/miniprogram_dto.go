package dto

// A transactionToken authenticates only the native continuation APIs. It is
// never accepted as an IAM session token or an application access token.
type MiniProgramLoginRequest struct {
	JsCode string `json:"jsCode" binding:"required,max=512"`
	Scope  string `json:"scope" binding:"max=2048"`
	Nonce  string `json:"nonce" binding:"max=255"`
}
type MiniProgramPhoneRequest struct {
	PhoneCode string `json:"phoneCode" binding:"required,max=512"`
}
type MiniProgramBindRequest struct {
	Username      string         `json:"username" binding:"required,max=255"`
	Password      string         `json:"password" binding:"required,max=1024"`
	CaptchaTicket *CaptchaTicket `json:"captchaTicket,omitempty"`
}
type MiniProgramProfile struct {
	Email    string `json:"email" binding:"omitempty,email,max=100"`
	RealName string `json:"realName" binding:"max=50"`
	Nickname string `json:"nickname" binding:"max=50"`
}
type MiniProgramCompleteRequest struct {
	Profile               MiniProgramProfile `json:"profile"`
	AcceptedAgreementKeys []string           `json:"acceptedAgreementKeys" binding:"max=100"`
	ApproveScopes         bool               `json:"approveScopes"`
}
type MiniProgramFactorRequest struct {
	Method string `json:"method" binding:"required,oneof=password totp phone email recovery"`
	Code   string `json:"code" binding:"max=1024"`
}
type MiniProgramResponse struct {
	State              string              `json:"state"`
	TransactionToken   string              `json:"transactionToken,omitempty"`
	ExpiresIn          int64               `json:"expiresIn,omitempty"`
	Scope              string              `json:"scope,omitempty"`
	TwoFactorChallenge *TwoFactorChallenge `json:"twoFactorChallenge,omitempty"`
	Tokens             *TokenResponse      `json:"tokens,omitempty"`
	IsNewUser          bool                `json:"isNewUser,omitempty"`
}
type MiniProgramConfigResponse struct {
	Name           string                  `json:"name"`
	Icon           string                  `json:"icon"`
	AccountPolicy  string                  `json:"accountPolicy"`
	RegisterFields *RegisterFieldsSettings `json:"registerFields"`
	Agreements     []ClientAgreementLink   `json:"agreements"`
}

type MiniProgramCaptchaParamsRequest struct {
	Fingerprint string `json:"fingerprint" binding:"required,max=256"`
}
