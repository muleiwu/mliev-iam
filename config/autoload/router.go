package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	AdminController "cnb.cool/mliev/iam/mliev-iam/app/controller/admin"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	envInterface "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/http_server/impl"
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
					// 管理员认证路由
					adminAuth := admin.Group("/auth")
					{
						adminAuth.POST("/login", deps.WrapHandler(AdminController.AuthController{}.Login))                                                    // 管理员登录
						adminAuth.POST("/logout", middleware.AdminAuthMiddleware(helper), deps.WrapHandler(AdminController.AuthController{}.Logout))          // 管理员退出登录
						adminAuth.POST("/refresh", middleware.AdminAuthMiddleware(helper), deps.WrapHandler(AdminController.AuthController{}.RefreshToken))   // 刷新令牌
						adminAuth.GET("/profile", middleware.AdminAuthMiddleware(helper), deps.WrapHandler(AdminController.AuthController{}.GetAdminProfile)) // 获取管理员个人信息
					}

					// 管理员用户管理路由
					adminUsers := admin.Group("/users", middleware.AdminAuthMiddleware(helper))
					{
						// 用户基础CRUD操作（管理员）
						adminUsers.POST("", deps.WrapHandler(AdminController.UserController{}.CreateUser))       // 创建用户
						adminUsers.GET("", deps.WrapHandler(AdminController.UserController{}.GetUserList))       // 获取用户列表
						adminUsers.GET("/:id", deps.WrapHandler(AdminController.UserController{}.GetUser))       // 获取用户详情
						adminUsers.PUT("/:id", deps.WrapHandler(AdminController.UserController{}.UpdateUser))    // 更新用户信息
						adminUsers.DELETE("/:id", deps.WrapHandler(AdminController.UserController{}.DeleteUser)) // 删除用户

						// 用户状态管理（管理员）
						adminUsers.PUT("/:id/status", deps.WrapHandler(AdminController.UserController{}.UpdateUserStatus)) // 更新用户状态

						// 密码管理（管理员）
						adminUsers.PUT("/:id/reset-password", deps.WrapHandler(AdminController.UserController{}.ResetPassword)) // 重置密码

						// 批量操作（管理员）
						adminUsers.POST("/batch", deps.WrapHandler(AdminController.UserController{}.BatchOperation)) // 批量操作用户

						// 统计信息（管理员）
						adminUsers.GET("/stats/active-count", deps.WrapHandler(AdminController.UserController{}.GetActiveUsersCount)) // 活跃用户统计

						// 用户部门管理（管理员）
						adminUsers.GET("/:id/departments", deps.WrapHandler(AdminController.UserController{}.GetUserDepartments)) // 获取用户部门

						// 用户角色管理（管理员）
						adminUsers.POST("/:id/roles/:roleId", deps.WrapHandler(AdminController.UserController{}.AssignUserRole))            // 分配角色
						adminUsers.DELETE("/:id/roles/:roleId", deps.WrapHandler(AdminController.UserController{}.RemoveUserRole))          // 移除角色
						adminUsers.GET("/:id/roles", deps.WrapHandler(AdminController.UserController{}.GetUserRoles))                       // 获取用户角色
						adminUsers.GET("/:id/with-roles", deps.WrapHandler(AdminController.UserController{}.GetUserWithRoles))              // 获取用户及角色
						adminUsers.POST("/:id/roles/batch", deps.WrapHandler(AdminController.UserController{}.BatchAssignUserRoles))        // 批量分配角色
						adminUsers.DELETE("/:id/roles/batch", deps.WrapHandler(AdminController.UserController{}.BatchRemoveUserRoles))      // 批量移除角色
						adminUsers.PUT("/:id/roles/:roleId/primary", deps.WrapHandler(AdminController.UserController{}.SetUserPrimaryRole)) // 设置主要角色
						adminUsers.GET("/:id/permissions", deps.WrapHandler(AdminController.UserController{}.GetUserPermissions))           // 获取用户权限
						adminUsers.GET("/:id/menu-tree", deps.WrapHandler(AdminController.UserController{}.GetUserMenuTree))                // 获取用户菜单树
					}

					// 角色管理路由（管理员）
					adminRoles := admin.Group("/roles", middleware.AdminAuthMiddleware(helper))
					{
						// 角色基础CRUD操作（管理员）
						adminRoles.POST("", deps.WrapHandler(AdminController.RoleController{}.CreateRole))       // 创建角色
						adminRoles.GET("", deps.WrapHandler(AdminController.RoleController{}.GetRoleList))       // 获取角色列表
						adminRoles.GET("/tree", deps.WrapHandler(AdminController.RoleController{}.GetRoleTree))  // 获取角色树形结构
						adminRoles.GET("/:id", deps.WrapHandler(AdminController.RoleController{}.GetRole))       // 获取角色详情
						adminRoles.PUT("/:id", deps.WrapHandler(AdminController.RoleController{}.UpdateRole))    // 更新角色信息
						adminRoles.DELETE("/:id", deps.WrapHandler(AdminController.RoleController{}.DeleteRole)) // 删除角色

						// 角色状态管理（管理员）
						adminRoles.PUT("/:id/status", deps.WrapHandler(AdminController.RoleController{}.UpdateRoleStatus)) // 更新角色状态

						// 批量操作（管理员）
						adminRoles.POST("/batch", deps.WrapHandler(AdminController.RoleController{}.BatchOperation)) // 批量操作角色

						// 角色权限管理（管理员）
						adminRoles.POST("/:id/permissions", deps.WrapHandler(AdminController.RoleController{}.AssignPermissions)) // 分配权限
						adminRoles.GET("/:id/permissions", deps.WrapHandler(AdminController.RoleController{}.GetRolePermissions)) // 获取角色权限

						// 统计信息（管理员）
						adminRoles.GET("/stats", deps.WrapHandler(AdminController.RoleController{}.GetRoleStats)) // 获取角色统计信息
					}

					// 权限管理路由（管理员）
					adminPermissions := admin.Group("/permissions", middleware.AdminAuthMiddleware(helper))
					{
						// 权限基础CRUD操作（管理员）
						adminPermissions.POST("", deps.WrapHandler(AdminController.PermissionController{}.CreatePermission))       // 创建权限
						adminPermissions.GET("", deps.WrapHandler(AdminController.PermissionController{}.GetPermissionList))       // 获取权限列表
						adminPermissions.GET("/tree", deps.WrapHandler(AdminController.PermissionController{}.GetPermissionTree))  // 获取权限树形结构
						adminPermissions.GET("/menu-tree", deps.WrapHandler(AdminController.PermissionController{}.GetMenuTree))   // 获取菜单树形结构
						adminPermissions.GET("/:id", deps.WrapHandler(AdminController.PermissionController{}.GetPermission))       // 获取权限详情
						adminPermissions.PUT("/:id", deps.WrapHandler(AdminController.PermissionController{}.UpdatePermission))    // 更新权限信息
						adminPermissions.DELETE("/:id", deps.WrapHandler(AdminController.PermissionController{}.DeletePermission)) // 删除权限

						// 权限状态管理（管理员）
						adminPermissions.PUT("/:id/status", deps.WrapHandler(AdminController.PermissionController{}.UpdatePermissionStatus)) // 更新权限状态

						// 批量操作（管理员）
						adminPermissions.POST("/batch", deps.WrapHandler(AdminController.PermissionController{}.BatchOperation)) // 批量操作权限

						// 统计信息（管理员）
						adminPermissions.GET("/stats", deps.WrapHandler(AdminController.PermissionController{}.GetPermissionStats)) // 获取权限统计信息
					}

					// 部门管理路由（管理员）
					adminDepartments := admin.Group("/departments", middleware.AdminAuthMiddleware(helper))
					{
						// 部门基础CRUD操作（管理员）
						adminDepartments.POST("", deps.WrapHandler(AdminController.DepartmentController{}.CreateDepartment))        // 创建部门
						adminDepartments.GET("", deps.WrapHandler(AdminController.DepartmentController{}.GetDepartmentList))        // 获取部门列表
						adminDepartments.GET("/tree", deps.WrapHandler(AdminController.DepartmentController{}.GetDepartmentTree))   // 获取部门树形结构
						adminDepartments.GET("/roots", deps.WrapHandler(AdminController.DepartmentController{}.GetRootDepartments)) // 获取根部门列表
						adminDepartments.GET("/:id", deps.WrapHandler(AdminController.DepartmentController{}.GetDepartment))        // 获取部门详情
						adminDepartments.PUT("/:id", deps.WrapHandler(AdminController.DepartmentController{}.UpdateDepartment))     // 更新部门信息
						adminDepartments.DELETE("/:id", deps.WrapHandler(AdminController.DepartmentController{}.DeleteDepartment))  // 删除部门

						// 部门状态管理（管理员）
						adminDepartments.PUT("/:id/status", deps.WrapHandler(AdminController.DepartmentController{}.UpdateDepartmentStatus)) // 更新部门状态

						// 部门结构管理（管理员）
						adminDepartments.PUT("/:id/move", deps.WrapHandler(AdminController.DepartmentController{}.MoveDepartment))                  // 移动部门
						adminDepartments.GET("/:id/children", deps.WrapHandler(AdminController.DepartmentController{}.GetChildrenDepartments))      // 获取子部门列表
						adminDepartments.GET("/:id/descendants", deps.WrapHandler(AdminController.DepartmentController{}.GetDescendantDepartments)) // 获取所有后代部门

						// 批量操作（管理员）
						adminDepartments.POST("/batch", deps.WrapHandler(AdminController.DepartmentController{}.BatchOperation)) // 批量操作部门

						// 统计信息（管理员）
						adminDepartments.GET("/stats/active-count", deps.WrapHandler(AdminController.DepartmentController{}.GetActiveDepartmentsCount)) // 启用部门统计

						// 维护操作（管理员）
						adminDepartments.POST("/validate-hierarchy", deps.WrapHandler(AdminController.DepartmentController{}.ValidateHierarchy)) // 验证部门层级结构
						adminDepartments.POST("/repair-hierarchy", deps.WrapHandler(AdminController.DepartmentController{}.RepairHierarchy))     // 修复部门层级结构

						// 部门相关的用户路由（管理员）
						adminDepartments.GET("/:id/users", deps.WrapHandler(AdminController.UserController{}.GetUsersByDepartment)) // 根据部门获取用户
					}

					// 负责人相关路由（管理员）
					adminManagers := admin.Group("/managers", middleware.AdminAuthMiddleware(helper))
					{
						adminManagers.GET("/:managerId/departments", deps.WrapHandler(AdminController.DepartmentController{}.GetDepartmentsByManager)) // 根据负责人获取部门
					}

					// 应用管理路由（管理员）- 支持多协议
					adminApps := admin.Group("/applications", middleware.AdminAuthMiddleware(helper))
					{
						// 应用基础CRUD操作（管理员）
						adminApps.POST("", deps.WrapHandler(AdminController.ApplicationController{}.CreateApplication))       // 创建应用
						adminApps.GET("", deps.WrapHandler(AdminController.ApplicationController{}.GetApplicationList))       // 获取应用列表
						adminApps.GET("/:id", deps.WrapHandler(AdminController.ApplicationController{}.GetApplication))       // 获取应用详情
						adminApps.PUT("/:id", deps.WrapHandler(AdminController.ApplicationController{}.UpdateApplication))    // 更新应用
						adminApps.DELETE("/:id", deps.WrapHandler(AdminController.ApplicationController{}.DeleteApplication)) // 删除应用

						// 批量操作（管理员）
						adminApps.POST("/batch", deps.WrapHandler(AdminController.ApplicationController{}.BatchOperation)) // 批量操作应用

						// 统计信息（管理员）
						adminApps.GET("/stats", deps.WrapHandler(AdminController.ApplicationController{}.GetStats)) // 获取应用统计信息
					}

					// 社会化登录提供商管理路由（管理员）
					adminSocialProviders := admin.Group("/social-providers", middleware.AdminAuthMiddleware(helper))
					{
						// 提供商基础CRUD操作（管理员）
						adminSocialProviders.POST("", deps.WrapHandler(AdminController.SocialProviderController{}.CreateProvider))       // 创建提供商
						adminSocialProviders.GET("", deps.WrapHandler(AdminController.SocialProviderController{}.GetProviderList))       // 获取提供商列表
						adminSocialProviders.GET("/:id", deps.WrapHandler(AdminController.SocialProviderController{}.GetProvider))       // 获取提供商详情
						adminSocialProviders.PUT("/:id", deps.WrapHandler(AdminController.SocialProviderController{}.UpdateProvider))    // 更新提供商
						adminSocialProviders.DELETE("/:id", deps.WrapHandler(AdminController.SocialProviderController{}.DeleteProvider)) // 删除提供商

						// 批量操作（管理员）
						adminSocialProviders.POST("/batch", deps.WrapHandler(AdminController.SocialProviderController{}.BatchOperation)) // 批量操作提供商

						// 统计信息（管理员）
						adminSocialProviders.GET("/stats", deps.WrapHandler(AdminController.SocialProviderController{}.GetProviderStats)) // 获取提供商统计信息
					}
				}

				// 普通用户路由组
				user := api.Group("/user")
				{
					// 公开端点：用户认证相关
					user.POST("/login", deps.WrapHandler(controller.UserController{}.ValidateLogin)) // 用户登录验证

					// 需要用户认证的端点（使用JWT认证中间件）
					userAuth := user.Group("", middleware.AuthMiddleware(helper))
					{
						// 用户个人信息管理
						userAuth.GET("/profile", deps.WrapHandler(controller.UserController{}.GetUserProfile))    // 获取当前用户信息
						userAuth.PUT("/profile", deps.WrapHandler(controller.UserController{}.UpdateUserProfile)) // 更新当前用户信息

						// 用户密码管理
						userAuth.PUT("/password", deps.WrapHandler(controller.UserController{}.ChangePassword)) // 修改密码

						// 用户社交账号管理
						userAuth.GET("/social-accounts", deps.WrapHandler(controller.SocialAuthController{}.GetUserSocialAccounts)) // 获取已绑定社交账号
					}
				}

			}

			// 社会化登录路由组
			social := router.Group("/social")
			{
				// 公开端点
				social.GET("/providers", deps.WrapHandler(controller.SocialAuthController{}.GetEnabledProviders))     // 获取可用提供商列表
				social.GET("/auth/:provider", deps.WrapHandler(controller.SocialAuthController{}.InitiateAuth))       // 发起OAuth授权
				social.GET("/callback/:provider", deps.WrapHandler(controller.SocialAuthController{}.HandleCallback)) // OAuth回调处理

				// 绑定/注册端点（公开，用于OIDC流程）
				social.POST("/bind-with-login", deps.WrapHandler(controller.SocialAuthController{}.BindWithLogin))           // 用社交账号绑定已有账户
				social.POST("/register-with-social", deps.WrapHandler(controller.SocialAuthController{}.RegisterWithSocial)) // 用社交账号注册新账户

				// 需要用户认证的端点（使用JWT认证中间件）
				socialAuth := social.Group("", middleware.AuthMiddleware(helper))
				{
					socialAuth.POST("/bind/:provider", deps.WrapHandler(controller.SocialAuthController{}.BindAccount))        // 绑定社交账号（需认证）
					socialAuth.DELETE("/unbind/:accountId", deps.WrapHandler(controller.SocialAuthController{}.UnbindAccount)) // 解绑社交账号（需认证）
				}
			}

			// OIDC 路由组（公开端点）
			oidc := router.Group("/oidc")
			{
				// OIDC 核心端点
				oidc.GET("/authorize", deps.WrapHandler(controller.OIDCController{}.Authorize))    // 授权端点
				oidc.POST("/token", deps.WrapHandler(controller.OIDCController{}.Token))           // 令牌端点
				oidc.GET("/userinfo", deps.WrapHandler(controller.OIDCController{}.UserInfo))      // 用户信息端点
				oidc.POST("/introspect", deps.WrapHandler(controller.OIDCController{}.Introspect)) // 令牌内省端点
				oidc.POST("/revoke", deps.WrapHandler(controller.OIDCController{}.Revoke))         // 令牌撤销端点
				oidc.GET("/jwks", deps.WrapHandler(controller.OIDCController{}.JWKS))              // JSON Web Key Set端点

				// OIDC 授权流程端点
				oidc.POST("/login", deps.WrapHandler(controller.OIDCAuthFlowController{}.Login))          // 登录接口
				oidc.POST("/consent", deps.WrapHandler(controller.OIDCAuthFlowController{}.Consent))      // 授权确认接口
				oidc.GET("/auth-info", deps.WrapHandler(controller.OIDCAuthFlowController{}.GetAuthInfo)) // 获取授权信息接口
			}

			// OIDC 发现文档端点（标准位置）
			router.GET("/.well-known/openid-configuration", deps.WrapHandler(controller.OIDCController{}.Discovery)) // OIDC发现文档

		},
	}
}
