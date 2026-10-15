package model

import "time"

// Passkey holds public WebAuthn credential data; private keys never leave the authenticator.
type Passkey struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	UserID           uint       `gorm:"not null;index" json:"-"`
	RPID             string     `gorm:"size:253;not null" json:"-"`
	UserHandle       string     `gorm:"size:64;not null" json:"-"`
	CredentialIDHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	CredentialID     string     `gorm:"type:text;not null" json:"-"`
	CredentialData   string     `gorm:"type:text;not null" json:"-"`
	Name             string     `gorm:"size:100;not null" json:"name"`
	CreatedAt        time.Time  `json:"createdAt"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
}
