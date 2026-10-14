package model

import (
	"time"

	"gorm.io/gorm"
)

// CustomPermission is an external OIDC scope, independent of admin RBAC.
type CustomPermission struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"size:100;uniqueIndex;not null" json:"code"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Enabled     bool           `gorm:"not null" json:"enabled"`
	CreatedBy   uint           `json:"createdBy"`
	UpdatedBy   uint           `json:"updatedBy"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type CustomPermissionGrant struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	PermissionID          uint           `gorm:"index;not null" json:"permissionId"`
	SubjectType           string         `gorm:"size:20;index;not null" json:"subjectType"`
	SubjectID             uint           `gorm:"index;not null" json:"subjectId"`
	IncludeSubDepartments bool           `gorm:"not null" json:"includeSubDepartments"`
	StartsAt              *time.Time     `json:"startsAt"`
	EndsAt                *time.Time     `json:"endsAt"`
	Remark                string         `gorm:"type:text" json:"remark"`
	CreatedBy             uint           `json:"createdBy"`
	UpdatedBy             uint           `json:"updatedBy"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g CustomPermissionGrant) State(now time.Time) string {
	if g.StartsAt != nil && now.Before(*g.StartsAt) {
		return "pending"
	}
	if g.EndsAt != nil && !now.Before(*g.EndsAt) {
		return "expired"
	}
	return "active"
}

func IsStandardOIDCScope(code string) bool {
	switch code {
	case "openid", "profile", "email", "phone", "address", "offline_access":
		return true
	}
	return false
}

// CleanLegacyCustomScopes removes only obsolete app-local custom definitions.
// IDs are the sole authority for new custom scope bindings.
func CleanLegacyCustomScopes(raw map[string]interface{}) error {
	c, err := DecodeOIDCAuthorizationConfig(raw)
	if err != nil {
		return err
	}
	if _, err := NormalizeOIDCScopes(c.Scopes); err != nil {
		return err
	}
	scopes := make([]interface{}, 0, len(c.Scopes))
	for _, scope := range c.Scopes {
		if IsStandardOIDCScope(scope) {
			scopes = append(scopes, scope)
		}
	}
	raw["scopes"] = scopes
	if _, exists := raw["scopeAuthorization"]; exists {
		for scope := range c.ScopeAuthorization.Rules {
			if !IsStandardOIDCScope(scope) {
				delete(c.ScopeAuthorization.Rules, scope)
			}
		}
		raw["scopeAuthorization"] = c.ScopeAuthorization
	}
	return nil
}

// Custom grant timestamps are stored as UTC, including fixtures and imports.
func (g *CustomPermissionGrant) BeforeSave(_ *gorm.DB) error {
	if g.StartsAt != nil {
		v := g.StartsAt.UTC()
		g.StartsAt = &v
	}
	if g.EndsAt != nil {
		v := g.EndsAt.UTC()
		g.EndsAt = &v
	}
	return nil
}
