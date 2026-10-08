package protocol

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func (h *OIDCProtocolHandler) validateAuthorizationConfig(raw map[string]interface{}) error {
	c, err := model.DecodeOIDCAuthorizationConfig(raw)
	if err != nil {
		return err
	}
	if _, err := model.NormalizeOIDCScopes(c.Scopes); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, scope := range c.Scopes {
		allowed[scope] = true
	}
	if c.RoleClaims.ClaimName != "roles" && c.RoleClaims.ClaimName != "groups" {
		return fmt.Errorf("角色字段只能为 roles 或 groups")
	}
	if c.RoleClaims.Enabled && len(c.RoleClaims.Targets) == 0 {
		return fmt.Errorf("请选择角色输出位置")
	}
	for _, target := range c.RoleClaims.Targets {
		if target != "access_token" && target != "id_token" && target != "userinfo" {
			return fmt.Errorf("无效的角色输出位置: %s", target)
		}
	}
	roles := append(append([]uint{}, c.RoleClaims.RoleIDs...), c.ClientRoleIDs...)
	var users, departments []uint
	for scope, rule := range c.ScopeAuthorization.Rules {
		if !allowed[scope] {
			return fmt.Errorf("scope 规则 %s 不在应用允许列表中", scope)
		}
		if rule.UserAccess == nil || rule.UserAccess.IsPublic == nil {
			return fmt.Errorf("scope %s 必须明确配置用户授权范围", scope)
		}
		roles = append(roles, rule.UserAccess.RoleIDs...)
		users = append(users, rule.UserAccess.UserIDs...)
		departments = append(departments, rule.UserAccess.DepartmentIDs...)
		if rule.AllowClientCredentials && raw["clientType"] != "confidential" {
			return fmt.Errorf("机器授权仅限机密客户端")
		}
	}
	if c.ScopeAuthorization.Enabled {
		for _, scope := range c.Scopes {
			if _, ok := c.ScopeAuthorization.Rules[scope]; !ok {
				return fmt.Errorf("scope %s 缺少授权规则", scope)
			}
		}
	}
	if len(c.ClientRoleIDs) > 0 && raw["clientType"] != "confidential" {
		return fmt.Errorf("机器角色仅限机密客户端")
	}
	for _, ref := range []struct {
		table string
		ids   []uint
	}{{"roles", roles}, {"users", users}, {"departments", departments}} {
		ids := map[uint]bool{}
		for _, id := range ref.ids {
			if id == 0 {
				return fmt.Errorf("%s 引用 ID 必须大于零", ref.table)
			}
			ids[id] = true
		}
		if len(ids) == 0 {
			continue
		}
		if h.db == nil {
			return fmt.Errorf("无法验证 %s 引用", ref.table)
		}
		unique := make([]uint, 0, len(ids))
		for id := range ids {
			unique = append(unique, id)
		}
		var count int64
		if err := h.db.Table(ref.table).Where("id IN ? AND deleted_at IS NULL", unique).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(unique)) {
			return fmt.Errorf("%s 引用不存在或已删除", ref.table)
		}
	}
	return nil
}
