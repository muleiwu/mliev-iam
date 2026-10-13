package dto

type TwoFactorMethodInfo struct {
	Method         string `json:"method"`
	Enabled        bool   `json:"enabled"`
	Available      bool   `json:"available"`
	MaskedReceiver string `json:"maskedReceiver,omitempty"`
}
type TwoFactorChallenge struct {
	ID         string                `json:"id"`
	Purpose    string                `json:"purpose"`
	Methods    []TwoFactorMethodInfo `json:"methods"`
	ExpiresAt  int64                 `json:"expiresAt"`
	RetryAfter int64                 `json:"retryAfter,omitempty"`
}
type TwoFactorRequired struct {
	RequiresTwoFactor  bool                `json:"requiresTwoFactor"`
	TwoFactorChallenge *TwoFactorChallenge `json:"twoFactorChallenge"`
}
type TwoFactorStatus struct {
	Enabled                bool                  `json:"enabled"`
	Methods                []TwoFactorMethodInfo `json:"methods"`
	RecoveryCodesRemaining int64                 `json:"recoveryCodesRemaining"`
	HasPassword            bool                  `json:"hasPassword"`
}
