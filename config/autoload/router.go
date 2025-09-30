package autoload

import (
	"cnb.cool/mliev/examples/go-web/app/controller"
	AdminDepartmentController "cnb.cool/mliev/examples/go-web/app/controller/admin"
	AdminUserController "cnb.cool/mliev/examples/go-web/app/controller/admin"
	envInterface "cnb.cool/mliev/examples/go-web/internal/interfaces"
	"cnb.cool/mliev/examples/go-web/internal/pkg/http_server/impl"
	"github.com/gin-gonic/gin"
)

type Router struct {
}

func (receiver Router) InitConfig(helper envInterface.HelperInterface) map[string]any {
	return map[string]any{
		"http.router": func(router *gin.Engine, deps *impl.HttpDeps) {

			// 首页
			router.GET("/", deps.WrapHandler(controller.IndexController{}.GetIndex))

			health := router.Group("/health")
			{
				// 健康检查接口
				health.GET("", deps.WrapHandler(controller.HealthController{}.GetHealth))
				health.GET("/simple", deps.WrapHandler(controller.HealthController{}.GetHealthSimple))
			}

			// API 路由组
			api := router.Group("/api")
			{
				// 管理员路由组
				admin := api.Group("/admin")
				{
					// 管理员用户管理路由
					adminUsers := admin.Group("/users")
					{
						// 用户基础CRUD操作（管理员）
						adminUsers.POST("", deps.WrapHandler(AdminUserController.AdminUserController{}.CreateUser))       // 创建用户
						adminUsers.GET("", deps.WrapHandler(AdminUserController.AdminUserController{}.GetUserList))       // 获取用户列表
						adminUsers.GET("/:id", deps.WrapHandler(AdminUserController.AdminUserController{}.GetUser))       // 获取用户详情
						adminUsers.PUT("/:id", deps.WrapHandler(AdminUserController.AdminUserController{}.UpdateUser))    // 更新用户信息
						adminUsers.DELETE("/:id", deps.WrapHandler(AdminUserController.AdminUserController{}.DeleteUser)) // 删除用户

						// 用户状态管理（管理员）
						adminUsers.PUT("/:id/status", deps.WrapHandler(AdminUserController.AdminUserController{}.UpdateUserStatus)) // 更新用户状态

						// 密码管理（管理员）
						adminUsers.PUT("/:id/reset-password", deps.WrapHandler(AdminUserController.AdminUserController{}.ResetPassword)) // 重置密码

						// 批量操作（管理员）
						adminUsers.POST("/batch", deps.WrapHandler(AdminUserController.AdminUserController{}.BatchOperation)) // 批量操作用户

						// 统计信息（管理员）
						adminUsers.GET("/stats/active-count", deps.WrapHandler(AdminUserController.AdminUserController{}.GetActiveUsersCount)) // 活跃用户统计
					}

					// 部门管理路由（管理员）
					adminDepartments := admin.Group("/departments")
					{
						// 部门基础CRUD操作（管理员）
						adminDepartments.POST("", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.CreateDepartment))        // 创建部门
						adminDepartments.GET("", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetDepartmentList))        // 获取部门列表
						adminDepartments.GET("/tree", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetDepartmentTree))   // 获取部门树形结构
						adminDepartments.GET("/roots", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetRootDepartments)) // 获取根部门列表
						adminDepartments.GET("/:id", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetDepartment))        // 获取部门详情
						adminDepartments.PUT("/:id", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.UpdateDepartment))     // 更新部门信息
						adminDepartments.DELETE("/:id", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.DeleteDepartment))  // 删除部门

						// 部门状态管理（管理员）
						adminDepartments.PUT("/:id/status", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.UpdateDepartmentStatus)) // 更新部门状态

						// 部门结构管理（管理员）
						adminDepartments.PUT("/:id/move", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.MoveDepartment))                  // 移动部门
						adminDepartments.GET("/:id/children", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetChildrenDepartments))      // 获取子部门列表
						adminDepartments.GET("/:id/descendants", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetDescendantDepartments)) // 获取所有后代部门

						// 批量操作（管理员）
						adminDepartments.POST("/batch", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.BatchOperation)) // 批量操作部门

						// 统计信息（管理员）
						adminDepartments.GET("/stats/active-count", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetActiveDepartmentsCount)) // 启用部门统计

						// 维护操作（管理员）
						adminDepartments.POST("/validate-hierarchy", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.ValidateHierarchy)) // 验证部门层级结构
						adminDepartments.POST("/repair-hierarchy", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.RepairHierarchy))     // 修复部门层级结构

						// 部门相关的用户路由（管理员）
						adminDepartments.GET("/:departmentId/users", deps.WrapHandler(AdminUserController.AdminUserController{}.GetUsersByDepartment)) // 根据部门获取用户
					}

					// 负责人相关路由（管理员）
					adminManagers := admin.Group("/managers")
					{
						adminManagers.GET("/:managerId/departments", deps.WrapHandler(AdminDepartmentController.AdminDepartmentController{}.GetDepartmentsByManager)) // 根据负责人获取部门
					}
				}

				// 普通用户路由组
				user := api.Group("/user")
				{
					// 用户认证相关
					user.POST("/login", deps.WrapHandler(controller.UserController{}.ValidateLogin)) // 用户登录验证

					// 用户个人信息管理
					user.GET("/profile", deps.WrapHandler(controller.UserController{}.GetUserProfile))    // 获取当前用户信息
					user.PUT("/profile", deps.WrapHandler(controller.UserController{}.UpdateUserProfile)) // 更新当前用户信息

					// 用户密码管理
					user.PUT("/password", deps.WrapHandler(controller.UserController{}.ChangePassword)) // 修改密码
				}
			}

		},
	}
}
