package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const OIDCRoleSnapshotAttribute = "oidcRoleClaims"
const OIDCDepartmentSnapshotAttribute = "oidcDepartmentClaims"

type OIDCDepartmentClaimsConfig struct {
	Enabled       bool     `json:"enabled"`
	DepartmentIDs []uint   `json:"departmentIds"`
	Targets       []string `json:"targets"`
}

type OIDCRoleClaimsConfig struct {
	Enabled   bool     `json:"enabled"`
	ClaimName string   `json:"claimName"`
	RoleIDs   []uint   `json:"roleIds"`
	Targets   []string `json:"targets"`
}

type OIDCScopeRule struct {
	Description            string                  `json:"description"`
	UserAccess             *ApplicationAccessScope `json:"userAccess"`
	AllowClientCredentials bool                    `json:"allowClientCredentials"`
}

type OIDCScopeAuthorization struct {
	Enabled bool                     `json:"enabled"`
	Rules   map[string]OIDCScopeRule `json:"rules"`
}

// OIDCAuthorizationConfig is the typed authorization subset of protocolConfig.
// Absence retains legacy authorization behavior.
type OIDCAuthorizationConfig struct {
	Scopes             []string                   `json:"scopes"`
	RoleClaims         OIDCRoleClaimsConfig       `json:"roleClaims"`
	DepartmentClaims   OIDCDepartmentClaimsConfig `json:"departmentClaims"`
	ScopeAuthorization OIDCScopeAuthorization     `json:"scopeAuthorization"`
	ClientRoleIDs      []uint                     `json:"clientRoleIds"`
}

func DecodeOIDCAuthorizationConfig(raw map[string]interface{}) (*OIDCAuthorizationConfig, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var config OIDCAuthorizationConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return nil, fmt.Errorf("OIDC 授权配置格式错误: %w", err)
	}
	if config.RoleClaims.ClaimName == "" {
		config.RoleClaims.ClaimName = "roles"
	}
	if config.RoleClaims.Targets == nil {
		config.RoleClaims.Targets = []string{"access_token"}
	}
	if config.DepartmentClaims.Targets == nil {
		config.DepartmentClaims.Targets = []string{"access_token"}
	}
	return &config, nil
}

// RFC 6749 §3.3: printable ASCII except double quote, backslash and space.
var oidcScopeName = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]+$`)

func ValidOIDCScope(scope string) bool { return oidcScopeName.MatchString(scope) }

func NormalizeOIDCScopes(scopes []string) ([]string, error) {
	result := make([]string, 0, len(scopes))
	seen := map[string]bool{}
	for _, scope := range scopes {
		if !ValidOIDCScope(scope) {
			return nil, fmt.Errorf("无效的 scope: %q", scope)
		}
		if !seen[scope] {
			result = append(result, scope)
			seen[scope] = true
		}
	}
	return result, nil
}

func ParseOIDCScopes(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	return NormalizeOIDCScopes(strings.Split(raw, " "))
}

// OIDCRoleSnapshot binds all claim destinations to the same issuance decision.
type OIDCRoleSnapshot struct {
	ClaimName string   `json:"claimName"`
	Values    []string `json:"values"`
	Targets   []string `json:"targets"`
}

func (s *OIDCRoleSnapshot) For(target string) (roles, groups *[]string) {
	if s == nil {
		return nil, nil
	}
	for _, t := range s.Targets {
		if t != target {
			continue
		}
		values := append([]string{}, s.Values...)
		switch s.ClaimName {
		case "roles":
			return &values, nil
		case "groups":
			return nil, &values
		}
	}
	return nil, nil
}

// OIDCDepartmentSnapshot is independent of role claims and never recalculated
// from live memberships when reading UserInfo for an existing access token.
type OIDCDepartmentSnapshot struct {
	Values  []string `json:"values"`
	Targets []string `json:"targets"`
}

func (s *OIDCDepartmentSnapshot) For(target string) *[]string {
	if s == nil {
		return nil
	}
	for _, destination := range s.Targets {
		if destination == target {
			values := append([]string{}, s.Values...)
			return &values
		}
	}
	return nil
}
