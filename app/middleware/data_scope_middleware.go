package middleware

import (
	"net/http"
	"strconv"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
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
func DataScopeMiddleware(helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 获取用户的数据权限范围
		dataScope := getMaxDataScope(userID, helper.GetDatabase())

		// 将数据权限范围存储到上下文中
		c.Set("data_scope", dataScope)
		c.Set("current_user_id", userID)

		c.Next()
	}
}

// ApplyDataScope 应用数据权限到查询中
func ApplyDataScope(db *gorm.DB, c *gin.Context, tableName string) *gorm.DB {
	// 获取数据权限范围
	dataScopeInterface, exists := c.Get("data_scope")
	if !exists {
		return db // 如果没有数据权限信息，直接返回原查询
	}

	dataScope, ok := dataScopeInterface.(int)
	if !ok {
		return db
	}

	// 获取当前用户ID
	userIDInterface, exists := c.Get("current_user_id")
	if !exists {
		return db
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		return db
	}

	switch dataScope {
	case DataScopeAll:
		// 全部数据，不添加任何条件
		return db

	case DataScopeDepartment:
		// 本部门数据
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			if tableName == "users" {
				// 通过 user_departments 表关联查询
				return db.Where("users.id IN (?)",
					db.Table("user_departments").
						Select("user_id").
						Where("department_id = ? AND is_active = ?", userDepartmentID, true))
			} else {
				return db.Where(tableName+".department_id = ?", userDepartmentID)
			}
		}
		return db.Where("1 = 0") // 如果用户没有部门，不显示任何数据

	case DataScopeDepartmentSub:
		// 本部门及下级数据
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			// 获取部门及其所有下级部门ID
			departmentIDs := getDepartmentAndSubIDs(userDepartmentID, db)
			if len(departmentIDs) > 0 {
				if tableName == "users" {
					// 通过 user_departments 表关联查询
					return db.Where("users.id IN (?)",
						db.Table("user_departments").
							Select("user_id").
							Where("department_id IN ? AND is_active = ?", departmentIDs, true))
				} else {
					return db.Where(tableName+".department_id IN ?", departmentIDs)
				}
			}
		}
		return db.Where("1 = 0") // 如果用户没有部门，不显示任何数据

	case DataScopeSelf:
		// 仅本人数据
		if tableName == "users" {
			return db.Where("id = ?", userID)
		} else {
			return db.Where(tableName+".user_id = ?", userID)
		}

	default:
		// 默认仅本人数据
		if tableName == "users" {
			return db.Where("id = ?", userID)
		} else {
			return db.Where(tableName+".user_id = ?", userID)
		}
	}
}

// getMaxDataScope 获取用户的最大数据权限范围
func getMaxDataScope(userID uint, db *gorm.DB) int {
	var maxDataScope int = DataScopeSelf // 默认仅本人数据

	// 查询用户的所有有效角色的数据权限范围
	var userRoles []model.UserRole
	if err := db.Where("user_id = ? AND is_active = ? AND deleted_at IS NULL", userID, true).
		Preload("Role").Find(&userRoles).Error; err != nil {
		return maxDataScope
	}

	// 找到最大的数据权限范围（数值越小，权限越大）
	for i := range userRoles {
		userRole := &userRoles[i]
		if userRole.Role.ID != 0 && userRole.Role.IsEnabled() {
			// 优先使用用户角色关联中的数据权限范围
			roleDataScope := userRole.DataScope
			if roleDataScope == 0 {
				// 如果用户角色关联中没有设置，使用角色的数据权限范围
				roleDataScope = userRole.Role.DataScope
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
	// 查询用户的主部门
	if err := db.Where("user_id = ? AND is_primary = ? AND is_active = ?", userID, true, true).
		First(&userDept).Error; err != nil {
		// 如果没有主部门，查询第一个活跃的部门
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
	var departmentIDs []uint
	departmentIDs = append(departmentIDs, departmentID)

	// 递归获取所有子部门ID
	var getSubDepartments func(uint)
	getSubDepartments = func(parentID uint) {
		var subDepartments []model.Department
		if err := db.Where("parent_id = ?", parentID).Find(&subDepartments).Error; err != nil {
			return
		}

		for _, dept := range subDepartments {
			departmentIDs = append(departmentIDs, dept.ID)
			getSubDepartments(dept.ID) // 递归获取子部门
		}
	}

	getSubDepartments(departmentID)
	return departmentIDs
}

// RequireDataScope 要求特定数据权限范围的中间件
func RequireDataScope(requiredScope int, helper interfaces.HelperInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取当前用户ID
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "用户未登录",
			})
			c.Abort()
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			// 尝试字符串转换
			if userIDStr, ok := userIDInterface.(string); ok {
				if id, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
					userID = uint(id)
				} else {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    401,
						"message": "用户ID格式错误",
					})
					c.Abort()
					return
				}
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "用户ID类型错误",
				})
				c.Abort()
				return
			}
		}

		// 获取用户的数据权限范围
		userDataScope := getMaxDataScope(userID, helper.GetDatabase())

		// 检查是否满足要求的数据权限范围（数值越小，权限越大）
		if userDataScope > requiredScope {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "数据权限范围不足",
			})
			c.Abort()
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
		// 全部数据，不添加任何条件
		return "", nil

	case DataScopeDepartment:
		// 本部门数据
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			if tableName == "users" {
				// 通过 user_departments 表关联查询
				return "users.id IN (SELECT user_id FROM user_departments WHERE department_id = ? AND is_active = ?)",
					[]interface{}{userDepartmentID, true}
			} else {
				return tableName + ".department_id = ?", []interface{}{userDepartmentID}
			}
		}
		return "1 = 0", nil // 如果用户没有部门，不显示任何数据

	case DataScopeDepartmentSub:
		// 本部门及下级数据
		userDepartmentID := getUserDepartmentID(userID, db)
		if userDepartmentID > 0 {
			departmentIDs := getDepartmentAndSubIDs(userDepartmentID, db)
			if len(departmentIDs) > 0 {
				if tableName == "users" {
					// 通过 user_departments 表关联查询
					return "users.id IN (SELECT user_id FROM user_departments WHERE department_id IN ? AND is_active = ?)",
						[]interface{}{departmentIDs, true}
				} else {
					return tableName + ".department_id IN ?", []interface{}{departmentIDs}
				}
			}
		}
		return "1 = 0", nil // 如果用户没有部门，不显示任何数据

	case DataScopeSelf:
		// 仅本人数据
		if tableName == "users" {
			return "id = ?", []interface{}{userID}
		} else {
			return tableName + ".user_id = ?", []interface{}{userID}
		}

	default:
		// 默认仅本人数据
		if tableName == "users" {
			return "id = ?", []interface{}{userID}
		} else {
			return tableName + ".user_id = ?", []interface{}{userID}
		}
	}
}
