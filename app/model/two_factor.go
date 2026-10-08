package model

import "time"

// TwoFactorMethod stores only confirmed factors. A changed contact never silently
// becomes an enrolled factor: Receiver must still match the verified user contact.
type TwoFactorMethod struct {
	UserID       uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Method       string    `gorm:"primaryKey;size:20" json:"method"`
	Receiver     string    `gorm:"size:100;not null;default:''" json:"-"`
	Ciphertext   string    `gorm:"type:text" json:"-"`
	Nonce        string    `gorm:"size:64" json:"-"`
	Algorithm    string    `gorm:"size:32" json:"-"`
	KeyVersion   int       `json:"-"`
	LastUsedStep int64     `gorm:"not null;default:0" json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

type TwoFactorRecoveryCode struct {
	ID       uint       `gorm:"primaryKey" json:"-"`
	UserID   uint       `gorm:"not null;index" json:"-"`
	CodeHash string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	UsedAt   *time.Time `json:"-"`
}

type TwoFactorEvent struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;index"`
	ActorID   uint   `gorm:"not null"`
	Action    string `gorm:"size:40;not null"`
	Method    string `gorm:"size:20"`
	Reason    string `gorm:"size:500"`
	IP        string `gorm:"size:64"`
	CreatedAt time.Time
}
