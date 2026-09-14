package model

import "time"

// LoginSession is a browser login, shared by its IAM credentials and OIDC grants.
// It contains no credentials. Missing or ended records can never authenticate.
type LoginSession struct {
	ID             string     `gorm:"primaryKey;size:36" json:"sessionId"`
	UserID         uint       `gorm:"not null;index" json:"userId"`
	Device         string     `gorm:"size:32" json:"device"`
	Browser        string     `gorm:"size:64" json:"browser"`
	OS             string     `gorm:"size:64" json:"os"`
	LoginMethod    string     `gorm:"size:64" json:"loginMethod"`
	LoginIP        string     `gorm:"size:64" json:"loginIp"`
	LastIP         string     `gorm:"size:64" json:"lastIp"`
	CreatedAt      time.Time  `json:"loginAt"`
	LastActiveAt   time.Time  `json:"lastActiveAt"`
	ExpiresAt      time.Time  `gorm:"index" json:"expiresAt"`
	HasAdminAccess bool       `json:"hasAdminAccess"`
	EndedAt        *time.Time `gorm:"index" json:"endedAt,omitempty"`
	EndReason      string     `gorm:"size:64" json:"endReason,omitempty"`
	EndedBy        *uint      `json:"endedBy,omitempty"`
}

func (LoginSession) TableName() string { return "login_sessions" }

func (s LoginSession) Status(now time.Time) string {
	if s.EndedAt != nil {
		if s.EndReason == "logout" {
			return "logged_out"
		}
		return "revoked"
	}
	if !s.ExpiresAt.After(now) {
		return "expired"
	}
	return "active"
}
