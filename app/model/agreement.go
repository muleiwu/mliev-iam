package model

import (
	"time"

	"gorm.io/gorm"
)

// AgreementScope 协议归属范围。
type AgreementScope string

const (
	AgreementScopeGlobal      AgreementScope = "global"
	AgreementScopeApplication AgreementScope = "application"
)

const (
	AgreementCodeTerms   = "terms"
	AgreementCodePrivacy = "privacy"

	AgreementContentTypeMarkdown = "markdown"
	AgreementContentTypeURL      = "url"
)

// Agreement 服务协议、隐私协议和应用自定义协议。
type Agreement struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Scope         string         `gorm:"size:20;not null;uniqueIndex:uk_agreements_scope_app_code,priority:1;index" json:"scope"`
	ApplicationID uint           `gorm:"not null;default:0;uniqueIndex:uk_agreements_scope_app_code,priority:2;index" json:"applicationId"`
	Code          string         `gorm:"size:100;not null;uniqueIndex:uk_agreements_scope_app_code,priority:3" json:"code"`
	Title         string         `gorm:"size:200;not null" json:"title"`
	ContentMD     string         `gorm:"column:content_md;type:longtext" json:"contentMd"`
	ContentType   string         `gorm:"size:20;not null;default:markdown" json:"contentType"`
	ContentURL    string         `gorm:"column:content_url;size:2048;not null;default:''" json:"contentUrl"`
	Enabled       bool           `gorm:"not null;default:false;index" json:"enabled"`
	Sort          int            `gorm:"not null;default:0;index" json:"sort"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"uniqueIndex:uk_agreements_scope_app_code,priority:4;index" json:"-"`
}

// TableName returns the table name for Agreement.
func (*Agreement) TableName() string {
	return "agreements"
}

// UsesURL reports whether the active application agreement is hosted externally.
func (a *Agreement) UsesURL() bool {
	return a.Scope == string(AgreementScopeApplication) && a.ContentType == AgreementContentTypeURL
}

// AgreementAcceptance 用户注册时的协议同意记录。
// ContentSHA256 hashes Markdown, or "url\n" plus the active URL (not the remote page).
type AgreementAcceptance struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"not null;index" json:"userId"`
	ApplicationID uint      `gorm:"not null;default:0;index" json:"applicationId"`
	AgreementID   uint      `gorm:"not null;index" json:"agreementId"`
	Scope         string    `gorm:"size:20;not null;index" json:"scope"`
	Code          string    `gorm:"size:100;not null;index" json:"code"`
	Title         string    `gorm:"size:200;not null" json:"title"`
	ContentSHA256 string    `gorm:"column:content_sha256;size:64;not null" json:"contentSha256"`
	AcceptedAt    time.Time `gorm:"not null;index" json:"acceptedAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

// TableName returns the table name for AgreementAcceptance.
func (*AgreementAcceptance) TableName() string {
	return "agreement_acceptances"
}
