package model

import "time"

// AppearanceTheme is an immutable imported snapshot, independent of live settings.
type AppearanceTheme struct {
	ID             uint      `gorm:"primaryKey" json:"-"`
	UUID           string    `gorm:"size:36;not null;uniqueIndex:uk_appearance_themes_uuid" json:"id"`
	IdentityKey    string    `gorm:"size:64;not null;uniqueIndex:uk_appearance_themes_identity" json:"-"`
	Name           string    `gorm:"size:100;not null" json:"name"`
	Version        string    `gorm:"size:32;not null" json:"version"`
	Author         string    `gorm:"size:100;not null;default:''" json:"author"`
	Description    string    `gorm:"size:1000;not null;default:''" json:"description"`
	FormatVersion  int       `gorm:"not null" json:"formatVersion"`
	AppearanceJSON string    `gorm:"type:text;not null" json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (*AppearanceTheme) TableName() string { return "appearance_themes" }
