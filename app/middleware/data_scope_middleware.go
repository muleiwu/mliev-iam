package middleware

import (
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"gorm.io/gorm"
)

// DataScope 数据权限范围常量
const (
	DataScopeAll           = 1 // 全部数据
	DataScopeDepartment    = 2 // 本部门数据
	DataScopeDepartmentSub = 3 // 本部门及下级数据
	DataScopeSelf          = 4 // 仅本人数据
)

// DataScopeMiddleware 数据权限中间件
func DataScopeMiddleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		dataScope := getMaxDataScope(userID, helper.GetDatabase())

		c.Set("data_scope", dataScope)
		c.Set("current_user_id", userID)

		c.Next()
	}
}

// ApplyDataScope 应用数据权限到查询中
func ApplyDataScope(db *gorm.DB, c httpInterfaces.RouterContextInterface, tableName string) *gorm.DB {
	dataScopeAny := c.Get("data_scope")
	if dataScopeAny == nil {
		return db
	}
	dataScope, ok := dataScopeAny.(int)
	if !ok {
		return db
	}

	userIDAny := c.Get("current_user_id")
	if userIDAny == nil {
		return db
	}
	userID, ok := userIDAny.(uint)
	if !ok {
		return db
	}

	switch dataScope {
	case DataScopeAll:
		return db

	case DataScopeDepartment:
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			if tableName == "users" {
				return db.Where("users.id IN (?)",
					db.Table("user_departments").
						Select("user_id").
						Where("department_id = ? AND is_active = ?", userDepartmentID, true))
			}
			return db.Where(tableName+".department_id = ?", userDepartmentID)
		}
		return db.Where("1 = 0")

	case DataScopeDepartmentSub:
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			departmentIDs := getDepartmentAndSubIDs(userDepartmentID, db)
			if len(departmentIDs) > 0 {
				if tableName == "users" {
					return db.Where("users.id IN (?)",
						db.Table("user_departments").
							Select("user_id").
							Where("department_id IN ? AND is_active = ?", departmentIDs, true))
				}
				return db.Where(tableName+".department_id IN ?", departmentIDs)
			}
		}
		return db.Where("1 = 0")

	case DataScopeSelf:
		if tableName == "users" {
			return db.Where("id = ?", userID)
		}
		return db.Where(tableName+".user_id = ?", userID)

	default:
		if tableName == "users" {
			return db.Where("id = ?", userID)
		}
		return db.Where(tableName+".user_id = ?", userID)
	}
}

// getMaxDataScope 获取用户的最大数据权限范围
func getMaxDataScope(userID uint, db *gorm.DB) int {
	maxDataScope := DataScopeSelf

	var userRoles []model.UserRole
	if err := db.Where("user_id = ? AND is_active = ? AND deleted_at IS NULL", userID, true).
		Preload("Role").Find(&userRoles).Error; err != nil {
		return maxDataScope
	}

	for i := range userRoles {
		userRole := &userRoles[i]
		if userRole.Role.ID != 0 && userRole.Role.IsEnabled() {
			roleDataScope := userRole.DataScope
			if roleDataScope == 0 {
				if userRole.Role.DataScope != nil {
					roleDataScope = *userRole.Role.DataScope
				}
			}

			if roleDataScope < maxDataScope {
				maxDataScope = roleDataScope
			}
		}
	}

	return maxDataScope
}

// getUserDepartmentID 获取用户的主部门ID（通过 user_departments 关联表）
func getUserDepartmentID(userID uint, db *gorm.DB) uint {
	var userDept model.UserDepartment
	if err := db.Where("user_id = ? AND is_primary = ? AND is_active = ?", userID, true, true).
		First(&userDept).Error; err != nil {
		if err := db.Where("user_id = ? AND is_active = ?", userID, true).
			Order("sort ASC").
			First(&userDept).Error; err != nil {
			return 0
		}
	}

	return userDept.DepartmentID
}

// getDepartmentAndSubIDs 获取部门及其所有下级部门ID
func getDepartmentAndSubIDs(departmentID uint, db *gorm.DB) []uint {
	departmentIDs := []uint{departmentID}

	var getSubDepartments func(uint)
	getSubDepartments = func(parentID uint) {
		var subDepartments []model.Department
		if err := db.Where("parent_id = ?", parentID).Find(&subDepartments).Error; err != nil {
			return
		}

		for _, dept := range subDepartments {
			departmentIDs = append(departmentIDs, dept.ID)
			getSubDepartments(dept.ID)
		}
	}

	getSubDepartments(departmentID)
	return departmentIDs
}

// RequireDataScope 要求特定数据权限范围的中间件
func RequireDataScope(requiredScope int) httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		userID, ok := resolveUserID(c)
		if !ok {
			return
		}

		userDataScope := getMaxDataScope(userID, helper.GetDatabase())

		if userDataScope > requiredScope {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"code":    403,
				"message": "数据权限范围不足",
			})
			return
		}

		c.Next()
	}
}

// GetDataScopeCondition 获取数据权限查询条件（用于Service层）
func GetDataScopeCondition(userID uint, db *gorm.DB, tableName string) (string, []interface{}) {
	dataScope := getMaxDataScope(userID, db)

	switch dataScope {
	case DataScopeAll:
		return "", nil

	case DataScopeDepartment:
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			if tableName == "users" {
				return "users.id IN (SELECT user_id FROM user_departments WHERE department_id = ? AND is_active = ?)",
					[]interface{}{userDepartmentID, true}
			}
			return tableName + ".department_id = ?", []interface{}{userDepartmentID}
		}
		return "1 = 0", nil

	case DataScopeDepartmentSub:
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			departmentIDs := getDepartmentAndSubIDs(userDepartmentID, db)
			if len(departmentIDs) > 0 {
				if tableName == "users" {
					return "users.id IN (SELECT user_id FROM user_departments WHERE department_id IN ? AND is_active = ?)",
						[]interface{}{departmentIDs, true}
				}
				return tableName + ".department_id IN ?", []interface{}{departmentIDs}
			}
		}
		return "1 = 0", nil

	case DataScopeSelf:
		if tableName == "users" {
			return "id = ?", []interface{}{userID}
		}
		return tableName + ".user_id = ?", []interface{}{userID}

	default:
		if tableName == "users" {
			return "id = ?", []interface{}{userID}
		}
		return tableName + ".user_id = ?", []interface{}{userID}
	}
}
