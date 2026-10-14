package dao

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResolveOIDCScopes is shared by protocol validation, issuance and discovery.
// Unbound legacy strings never become permissions, even if their code exists.
func ResolveOIDCScopes(db *gorm.DB, c *model.OIDCAuthorizationConfig) ([]string, map[string]model.CustomPermission, error) {
	scopes := []string{}
	seen := map[string]bool{}
	for _, scope := range c.Scopes {
		if model.IsStandardOIDCScope(scope) && !seen[scope] {
			scopes = append(scopes, scope)
			seen[scope] = true
		}
	}
	custom := map[string]model.CustomPermission{}
	if len(c.CustomPermissionIDs) == 0 {
		return scopes, custom, nil
	}
	if db == nil {
		return nil, nil, fmt.Errorf("无法读取自定义权限")
	}
	var permissions []model.CustomPermission
	if err := db.Where("id IN ? AND enabled = ?", c.CustomPermissionIDs, true).Order("id").Find(&permissions).Error; err != nil {
		return nil, nil, err
	}
	for _, p := range permissions {
		scopes = append(scopes, p.Code)
		custom[p.Code] = p
	}
	return scopes, custom, nil
}

// LockCustomPermissionReferences serializes binding with permission deletion.
func LockCustomPermissionReferences(db *gorm.DB, raw map[string]interface{}) error {
	c, err := model.DecodeOIDCAuthorizationConfig(raw)
	if err != nil {
		return err
	}
	if len(c.CustomPermissionIDs) == 0 {
		return nil
	}
	var rows []model.CustomPermission
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", c.CustomPermissionIDs).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	known := map[uint]bool{}
	for _, row := range rows {
		known[row.ID] = true
	}
	for _, id := range c.CustomPermissionIDs {
		if !known[id] {
			return fmt.Errorf("请求参数错误: 自定义权限不存在或已删除: %d", id)
		}
	}
	return nil
}
