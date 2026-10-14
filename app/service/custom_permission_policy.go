package service

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// nil denotes infinity. AND combines deadlines using min; OR uses max.
func earlierDeadline(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || a.Before(*b) {
		return a
	}
	return b
}
func laterDeadline(a, b *time.Time) *time.Time {
	if a == nil || b == nil {
		return nil
	}
	if a.After(*b) {
		return a
	}
	return b
}

func (s *OIDCAuthorizationService) evaluateCustomPermissions(app *model.Application, userID uint, requested []string, custom map[string]model.CustomPermission, machine bool, now time.Time) (*time.Time, error) {
	ids := []uint{}
	for _, code := range requested {
		if p, ok := custom[code]; ok {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var grants []model.CustomPermissionGrant
	if err := s.db.Where("permission_id IN ? AND (starts_at IS NULL OR starts_at <= ?) AND (ends_at IS NULL OR ends_at > ?)", ids, now.UTC(), now.UTC()).Find(&grants).Error; err != nil {
		return nil, constants.ErrServerError
	}
	roles := map[uint]*time.Time{}
	needsRoles := false
	for _, grant := range grants {
		if grant.SubjectType == "role" {
			needsRoles = true
		}
	}
	if !machine && needsRoles {
		assignments, err := dao.NewUserRoleDao(s.db).GetByUserID(userID)
		if err != nil {
			return nil, constants.ErrServerError
		}
		for _, a := range assignments {
			if a.IsActive && a.IsApproved() && (a.ExpiresAt == nil || a.ExpiresAt.After(now)) && a.ApplyScope == 1 && a.DepartmentID == nil && a.OrganizationID == nil && a.Role.ID != 0 && a.Role.IsEnabled() {
				roles[a.RoleID] = a.ExpiresAt
			}
		}
	}
	var until *time.Time
	for _, id := range ids {
		matched := false
		var permissionUntil *time.Time
		for _, grant := range grants {
			if grant.PermissionID != id {
				continue
			}
			// Malformed partial intervals must never act as permanent grants.
			if (grant.StartsAt == nil) != (grant.EndsAt == nil) || (grant.StartsAt != nil && !grant.StartsAt.Before(*grant.EndsAt)) {
				continue
			}
			ok := false
			end := grant.EndsAt
			if machine {
				ok = grant.SubjectType == "client" && grant.SubjectID == app.ID && machinePermissionClient(app)
			} else {
				switch grant.SubjectType {
				case "everyone":
					ok = grant.SubjectID == 0
				case "user":
					ok = grant.SubjectID == userID
				case "role":
					roleEnd, has := roles[grant.SubjectID]
					ok = has
					end = earlierDeadline(end, roleEnd)
				case "department":
					public := false
					access := &model.ApplicationAccessScope{IsPublic: &public, DepartmentIDs: []uint{grant.SubjectID}, IncludeSubDepartments: grant.IncludeSubDepartments}
					var err error
					ok, err = s.matchesOIDCScopeAccess(userID, access, nil)
					if err != nil {
						return nil, constants.ErrServerError
					}
				}
			}
			if ok {
				if !matched {
					permissionUntil = end
				} else {
					permissionUntil = laterDeadline(permissionUntil, end)
				}
				matched = true
			}
		}
		if !matched {
			if machine {
				return nil, constants.ErrInvalidScope
			}
			return nil, constants.ErrAccessDenied
		}
		until = earlierDeadline(until, permissionUntil)
	}
	return until, nil
}

func (s *OIDCAuthorizationService) grantAccessTokenExpiry(grant *oidcGrant) error {
	now := time.Now()
	lifetime := s.getTokenLifetimeFromApp(grant.App)
	if lifetime <= 0 {
		lifetime = 3600
	}
	expires := now.Add(time.Duration(lifetime) * time.Second)
	if grant.ValidUntil != nil && grant.ValidUntil.Before(expires) {
		expires = *grant.ValidUntil
	}
	// JWT NumericDate and database share the same whole-second boundary.
	grant.AccessTokenExpiresAt = time.Unix(expires.Unix(), 0).In(now.Location())
	if grant.AccessTokenExpiresAt.Unix() <= now.Unix() {
		return constants.ErrAccessDenied
	}
	return nil
}
