package service

import (
	"errors"
	"sort"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type oidcGrant struct {
	ValidUntil           *time.Time
	AccessTokenExpiresAt time.Time
	App                  *model.Application
	Scopes               []string
	Roles                *model.OIDCRoleSnapshot
	Departments          *model.OIDCDepartmentSnapshot
}

func (g *oidcGrant) attributes() map[string]interface{} {
	attrs := oidcRoleAttributes(g.Roles)
	if g.Departments != nil {
		attrs[model.OIDCDepartmentSnapshotAttribute] = g.Departments
	}
	return attrs
}

// CheckUserScopeAuthorization lets browser flows report policy denial before
// authenticating their OIDC transaction. Issuance still performs its own check.
func (s *OIDCAuthorizationService) CheckUserScopeAuthorization(app *model.Application, userID uint, rawScope string) error {
	config, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
	if err != nil {
		return constants.ErrServerError
	}
	scopes, err := model.ParseOIDCScopes(rawScope)
	if err != nil {
		return constants.ErrInvalidScope
	}
	// Preserve the standard-scope browser compatibility path. Unbound custom
	// strings are rejected even when the legacy policy switch is off.
	if !config.ScopeAuthorization.Enabled && len(config.CustomPermissionIDs) == 0 {
		for _, scope := range scopes {
			if !model.IsStandardOIDCScope(scope) {
				return constants.ErrInvalidScope
			}
		}
		return nil
	}
	_, err = s.evaluateOIDCGrant(app, userID, scopes, false)
	return err
}

// evaluateOIDCGrant is the last authorization boundary before signing. Reloading
// the application also prevents refresh/SSO callers from using stale policies.
func (s *OIDCAuthorizationService) evaluateOIDCGrant(app *model.Application, userID uint, requested []string, machine bool) (*oidcGrant, error) {
	var current model.Application
	if err := s.db.First(&current, app.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrAccessDenied
		}
		return nil, constants.ErrServerError
	}
	if !current.IsActive || current.ProtocolType != "oidc" {
		return nil, constants.ErrAccessDenied
	}
	c, err := model.DecodeOIDCAuthorizationConfig(current.ProtocolConfig)
	if err != nil {
		return nil, constants.ErrServerError
	}
	scopes, err := model.NormalizeOIDCScopes(requested)
	if err != nil {
		return nil, constants.ErrInvalidScope
	}
	effective, custom, err := dao.ResolveOIDCScopes(s.db, c)
	if err != nil {
		return nil, constants.ErrServerError
	}
	allowed := map[string]bool{}
	for _, scope := range effective {
		allowed[scope] = true
	}
	for _, scope := range scopes {
		if !allowed[scope] {
			return nil, constants.ErrInvalidScope
		}
	}
	if machine && current.ProtocolConfig["clientType"] != "confidential" {
		return nil, constants.ErrUnauthorizedClient
	}
	if machine && (c.ScopeAuthorization.Enabled || len(c.CustomPermissionIDs) > 0) && len(scopes) == 0 {
		return nil, constants.ErrInvalidScope
	}
	if !machine {
		user, err := s.userService.GetUserModelByID(userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, constants.ErrAccessDenied
			}
			return nil, constants.ErrServerError
		}
		if !user.CanLogin() || !s.appService.CheckUserAccess(current.ID, userID) {
			return nil, constants.ErrAccessDenied
		}
	}
	roles := []model.Role{}
	needsRoles := c.RoleClaims.Enabled
	for _, scope := range scopes {
		if rule, ok := c.ScopeAuthorization.Rules[scope]; model.IsStandardOIDCScope(scope) && c.ScopeAuthorization.Enabled && ok && rule.UserAccess != nil && len(rule.UserAccess.RoleIDs) > 0 {
			needsRoles = true
		}
	}
	if needsRoles {
		roles, err = s.oidcSubjectRoles(userID, c.ClientRoleIDs, machine)
		if err != nil {
			return nil, constants.ErrServerError
		}
	}
	if c.ScopeAuthorization.Enabled {
		for _, scope := range scopes {
			if !model.IsStandardOIDCScope(scope) {
				continue
			}
			rule, ok := c.ScopeAuthorization.Rules[scope]
			if !ok {
				return nil, constants.ErrInvalidScope
			}
			if machine {
				if !rule.AllowClientCredentials {
					return nil, constants.ErrInvalidScope
				}
				continue
			}
			matched, err := s.matchesOIDCScopeAccess(userID, rule.UserAccess, roles)
			if err != nil {
				return nil, constants.ErrServerError
			}
			if !matched {
				return nil, constants.ErrAccessDenied
			}
		}
	}
	until, err := s.evaluateCustomPermissions(&current, userID, scopes, custom, machine, time.Now())
	if err != nil {
		return nil, err
	}
	grant := &oidcGrant{App: &current, Scopes: scopes, ValidUntil: until}
	if c.RoleClaims.Enabled {
		whitelist := map[uint]bool{}
		for _, id := range c.RoleClaims.RoleIDs {
			whitelist[id] = true
		}
		codes := map[string]bool{}
		for _, role := range roles {
			if whitelist[role.ID] {
				codes[role.Code] = true
			}
		}
		values := []string{}
		for code := range codes {
			values = append(values, code)
		}
		sort.Strings(values)
		grant.Roles = &model.OIDCRoleSnapshot{ClaimName: c.RoleClaims.ClaimName, Values: values, Targets: c.RoleClaims.Targets}
	}
	if !machine && c.DepartmentClaims.Enabled {
		values, err := s.oidcUserDepartments(userID, c.DepartmentClaims.DepartmentIDs)
		if err != nil {
			return nil, constants.ErrServerError
		}
		grant.Departments = &model.OIDCDepartmentSnapshot{Values: values, Targets: c.DepartmentClaims.Targets}
	}
	return grant, nil
}

func (s *OIDCAuthorizationService) oidcSubjectRoles(userID uint, clientRoles []uint, machine bool) ([]model.Role, error) {
	roles := []model.Role{}
	if machine {
		if len(clientRoles) > 0 {
			if err := s.db.Where("id IN ? AND status = ?", clientRoles, model.RoleStatusEnabled).Find(&roles).Error; err != nil {
				return nil, err
			}
		}
		return roles, nil
	}
	assignments, err := dao.NewUserRoleDao(s.db).GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, assignment := range assignments {
		if !assignment.IsActive || !assignment.IsApproved() || (assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(now)) {
			continue
		}
		if assignment.ApplyScope != 1 || assignment.DepartmentID != nil || assignment.OrganizationID != nil {
			continue
		}
		if assignment.Role.ID != 0 && assignment.Role.IsEnabled() {
			roles = append(roles, assignment.Role)
		}
	}
	return roles, nil
}

func (s *OIDCAuthorizationService) matchesOIDCScopeAccess(userID uint, access *model.ApplicationAccessScope, roles []model.Role) (bool, error) {
	// Unlike legacy app access, an omitted/empty scope rule never means everyone.
	if access == nil || access.IsPublic == nil {
		return false, nil
	}
	if *access.IsPublic || access.HasUserID(userID) {
		return true, nil
	}
	for _, role := range roles {
		if access.HasRoleID(role.ID) {
			return true, nil
		}
	}
	if len(access.DepartmentIDs) == 0 {
		return false, nil
	}
	var allowed []model.Department
	if err := s.db.Where("id IN ? AND status = ?", access.DepartmentIDs, model.DepartmentStatusEnabled).Find(&allowed).Error; err != nil {
		return false, err
	}
	memberships, err := dao.NewUserDepartmentDao(s.db).FindActiveDepartmentsByUser(userID)
	if err != nil {
		return false, err
	}
	for _, member := range memberships {
		if member.Department.ID == 0 || member.Department.Status != model.DepartmentStatusEnabled {
			continue
		}
		for _, dept := range allowed {
			if member.DepartmentID == dept.ID {
				return true, nil
			}
			prefix := strings.TrimRight(dept.Path, "/") + "/"
			if access.IncludeSubDepartments && dept.Path != "" && strings.HasPrefix(member.Department.Path, prefix) {
				return true, nil
			}
		}
	}
	return false, nil
}

func oidcGrantError(err error) error {
	if errors.Is(err, constants.ErrAccessDenied) || errors.Is(err, constants.ErrInvalidScope) {
		return constants.ErrInvalidGrant
	}
	return err
}

func oidcRoleAttributes(snapshot *model.OIDCRoleSnapshot) map[string]interface{} {
	attrs := map[string]interface{}{}
	if snapshot != nil {
		attrs[model.OIDCRoleSnapshotAttribute] = snapshot
	}
	return attrs
}
