package directory

import "context"

// Diagnostic separates connectivity, token and directory-scope checks so the
// administration UI can explain a partial permission setup precisely.
type Diagnostic struct {
	Healthy           bool     `json:"healthy"`
	TokenValid        bool     `json:"tokenValid"`
	DirectoryReadable bool     `json:"directoryReadable"`
	TenantID          string   `json:"tenantId,omitempty"`
	VisibleOrgUnits   int      `json:"visibleOrgUnits"`
	VisibleUsers      int      `json:"visibleUsers"`
	Warnings          []string `json:"warnings"`
}

type Identifier struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
	Primary   bool   `json:"primary"`
}

type OrgUnit struct {
	ExternalID       string         `json:"externalId"`
	ExternalParentID string         `json:"externalParentId"`
	Name             string         `json:"name"`
	SortOrder        int            `json:"sortOrder"`
	LeaderExternalID string         `json:"leaderExternalId"`
	Status           string         `json:"status"`
	Raw              map[string]any `json:"raw"`
}

type Membership struct {
	OrgUnitExternalID string         `json:"orgUnitExternalId"`
	Primary           bool           `json:"primary"`
	Leader            bool           `json:"leader"`
	Position          string         `json:"position"`
	SortOrder         int            `json:"sortOrder"`
	Raw               map[string]any `json:"raw"`
}

type User struct {
	StableID       string         `json:"stableId"`
	TenantID       string         `json:"tenantId"`
	ProviderUserID string         `json:"providerUserId"`
	Identifiers    []Identifier   `json:"identifiers"`
	DisplayName    string         `json:"displayName"`
	Email          string         `json:"email"`
	Phone          string         `json:"phone"`
	EmployeeNo     string         `json:"employeeNo"`
	Avatar         string         `json:"avatar"`
	Status         string         `json:"status"`
	Memberships    []Membership   `json:"memberships"`
	Raw            map[string]any `json:"raw"`
}

type OrgUnitPage struct {
	Items      []OrgUnit `json:"items"`
	NextCursor string    `json:"nextCursor"`
}

type UserPage struct {
	Items      []User `json:"items"`
	NextCursor string `json:"nextCursor"`
}

// Adapter deliberately keeps authentication, directory and event concerns
// separate even if a provider uses the same upstream application.
type Adapter interface {
	ValidateDirectoryConfig(ctx context.Context) (*Diagnostic, error)
	ListOrgUnits(ctx context.Context, cursor string) (*OrgUnitPage, error)
	ListUsers(ctx context.Context, cursor string) (*UserPage, error)
}
