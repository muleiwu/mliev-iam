package service

import (
	"fmt"
	"sort"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// oidcUserDepartments expands active memberships via ParentID, not the denormalized
// Path. Every node is fetched at most once per decision, including shared parents.
func (s *OIDCAuthorizationService) oidcUserDepartments(userID uint, whitelist []uint) ([]string, error) {
	values := []string{}
	if len(whitelist) == 0 {
		return values, nil
	}
	allowed := make(map[uint]bool, len(whitelist))
	for _, id := range whitelist {
		allowed[id] = true
	}
	var memberships []model.UserDepartment
	if err := s.db.Select("department_id").Where("user_id = ? AND is_active = ? AND left_at IS NULL", userID, true).Find(&memberships).Error; err != nil {
		return nil, err
	}
	// A nil entry memoizes a deleted/missing node and terminates that path.
	nodes := map[uint]*model.Department{}
	frontier := map[uint]bool{}
	for _, membership := range memberships {
		frontier[membership.DepartmentID] = true
	}
	for len(frontier) > 0 {
		ids := make([]uint, 0, len(frontier))
		for id := range frontier {
			ids = append(ids, id)
			nodes[id] = nil
		}
		var departments []model.Department
		if err := s.db.Select("id", "code", "parent_id", "status").Where("id IN ?", ids).Find(&departments).Error; err != nil {
			return nil, err
		}
		frontier = map[uint]bool{}
		for i := range departments {
			department := &departments[i]
			nodes[department.ID] = department
			if department.IsEnabled() && department.ParentID != nil {
				if _, loaded := nodes[*department.ParentID]; !loaded {
					frontier[*department.ParentID] = true
				}
			}
		}
	}
	codes := map[string]bool{}
	for _, membership := range memberships {
		visited := map[uint]bool{}
		for department := nodes[membership.DepartmentID]; department != nil && department.IsEnabled(); {
			if visited[department.ID] {
				return nil, fmt.Errorf("OIDC department hierarchy contains a cycle at department %d", department.ID)
			}
			visited[department.ID] = true
			if allowed[department.ID] {
				codes[department.Code] = true
			}
			if department.ParentID == nil {
				break
			}
			department = nodes[*department.ParentID]
		}
	}
	for code := range codes {
		values = append(values, code)
	}
	sort.Strings(values)
	return values, nil
}
