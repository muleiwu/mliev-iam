package autoload

import (
	"cnb.cool/mliev/iam/mliev-iam/app/controller"
	AdminController "cnb.cool/mliev/iam/mliev-iam/app/controller/admin"
	"cnb.cool/mliev/iam/mliev-iam/app/middleware"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

// Router 注册所有 HTTP 路由。
//
// 约定：
//   - go-web 的 RouterInterface.Group(path) 只接收 path；中间件通过返回组的 Use(...) 注入。
//   - 公开与受保护路由共享前缀时，拆成两个同名 Group（不同实例）分别挂载 vs 不挂载中间件。
//   - 原 Gin 的 .Match(GET|POST, ...) 拆成 .GET 和 .POST 两次注册。
type Router struct {
}

func (receiver Router) InitConfig() map[string]any {
	return map[string]any{
		"http.router": func(router httpInterfaces.RouterInterface) {
			// 首页
			router.GET("/", controller.IndexController{}.GetIndex)

			// 健康检查
			health := router.Group("/health")
			health.GET("", controller.HealthController{}.GetHealth)
			health.GET("/simple", controller.HealthController{}.GetHealthSimple)

			// API 路由组
			api := router.Group("/api")

			// 系统安装（公开访问，中间件 InstallCheckMiddleware 会放行 /api/install/*）
			install := api.Group("/install")
			install.GET("/check", controller.InstallController{}.CheckInstall)
			install.POST("/test-connection", controller.InstallController{}.TestConnection)
			install.POST("/test-redis", controller.InstallController{}.TestRedisConnection)
			install.POST("/submit", controller.InstallController{}.SubmitInstall)

			// 系统级路由（非 /admin 命名空间，仅管理员可访问）
			adminSystem := api.Group("/system")
			adminSystem.Use(middleware.AdminAuthMiddleware())
			adminSystem.GET("/reload", AdminController.SystemController{}.ReloadConfig)
			adminSystem.GET("/info", AdminController.SystemController{}.GetSystemInfo)

			// === 管理员路由 ===
			admin := api.Group("/admin")

			// 管理员认证：login 公开，其余需要 AdminAuth
			adminAuthPublic := admin.Group("/auth")
			adminAuthPublic.POST("/login", AdminController.AuthController{}.Login)

			adminAuthProtected := admin.Group("/auth")
			adminAuthProtected.Use(middleware.AdminAuthMiddleware())
			adminAuthProtected.POST("/logout", AdminController.AuthController{}.Logout)
			adminAuthProtected.POST("/refresh", AdminController.AuthController{}.RefreshToken)
			adminAuthProtected.GET("/profile", AdminController.AuthController{}.GetAdminProfile)

			// 管理员用户管理
			adminUsers := admin.Group("/users")
			adminUsers.Use(middleware.AdminAuthMiddleware())
			adminUsers.POST("", AdminController.UserController{}.CreateUser)
			adminUsers.GET("", AdminController.UserController{}.GetUserList)
			adminUsers.GET("/:id", AdminController.UserController{}.GetUser)
			adminUsers.PUT("/:id", AdminController.UserController{}.UpdateUser)
			adminUsers.DELETE("/:id", AdminController.UserController{}.DeleteUser)
			adminUsers.PUT("/:id/status", AdminController.UserController{}.UpdateUserStatus)
			adminUsers.PUT("/:id/reset-password", AdminController.UserController{}.ResetPassword)
			adminUsers.POST("/batch", AdminController.UserController{}.BatchOperation)
			adminUsers.GET("/stats/active-count", AdminController.UserController{}.GetActiveUsersCount)
			adminUsers.GET("/:id/departments", AdminController.UserController{}.GetUserDepartments)
			adminUsers.POST("/:id/roles/:roleId", AdminController.UserController{}.AssignUserRole)
			adminUsers.DELETE("/:id/roles/:roleId", AdminController.UserController{}.RemoveUserRole)
			adminUsers.GET("/:id/roles", AdminController.UserController{}.GetUserRoles)
			adminUsers.GET("/:id/with-roles", AdminController.UserController{}.GetUserWithRoles)
			adminUsers.POST("/:id/roles/batch", AdminController.UserController{}.BatchAssignUserRoles)
			adminUsers.DELETE("/:id/roles/batch", AdminController.UserController{}.BatchRemoveUserRoles)
			adminUsers.PUT("/:id/roles/:roleId/primary", AdminController.UserController{}.SetUserPrimaryRole)
			adminUsers.GET("/:id/permissions", AdminController.UserController{}.GetUserPermissions)
			adminUsers.GET("/:id/menu-tree", AdminController.UserController{}.GetUserMenuTree)

			// 管理员角色管理
			adminRoles := admin.Group("/roles")
			adminRoles.Use(middleware.AdminAuthMiddleware())
			adminRoles.POST("", AdminController.RoleController{}.CreateRole)
			adminRoles.GET("", AdminController.RoleController{}.GetRoleList)
			adminRoles.GET("/tree", AdminController.RoleController{}.GetRoleTree)
			adminRoles.GET("/:id", AdminController.RoleController{}.GetRole)
			adminRoles.PUT("/:id", AdminController.RoleController{}.UpdateRole)
			adminRoles.DELETE("/:id", AdminController.RoleController{}.DeleteRole)
			adminRoles.PUT("/:id/status", AdminController.RoleController{}.UpdateRoleStatus)
			adminRoles.POST("/batch", AdminController.RoleController{}.BatchOperation)
			adminRoles.POST("/:id/permissions", AdminController.RoleController{}.AssignPermissions)
			adminRoles.GET("/:id/permissions", AdminController.RoleController{}.GetRolePermissions)
			adminRoles.GET("/stats", AdminController.RoleController{}.GetRoleStats)

			// 管理员权限管理
			adminPermissions := admin.Group("/permissions")
			adminPermissions.Use(middleware.AdminAuthMiddleware())
			adminPermissions.POST("", AdminController.PermissionController{}.CreatePermission)
			adminPermissions.GET("", AdminController.PermissionController{}.GetPermissionList)
			adminPermissions.GET("/tree", AdminController.PermissionController{}.GetPermissionTree)
			adminPermissions.GET("/menu-tree", AdminController.PermissionController{}.GetMenuTree)
			adminPermissions.GET("/:id", AdminController.PermissionController{}.GetPermission)
			adminPermissions.PUT("/:id", AdminController.PermissionController{}.UpdatePermission)
			adminPermissions.DELETE("/:id", AdminController.PermissionController{}.DeletePermission)
			adminPermissions.PUT("/:id/status", AdminController.PermissionController{}.UpdatePermissionStatus)
			adminPermissions.POST("/batch", AdminController.PermissionController{}.BatchOperation)
			adminPermissions.GET("/stats", AdminController.PermissionController{}.GetPermissionStats)

			// 管理员部门管理
			adminDepartments := admin.Group("/departments")
			adminDepartments.Use(middleware.AdminAuthMiddleware())
			adminDepartments.POST("", AdminController.DepartmentController{}.CreateDepartment)
			adminDepartments.GET("", AdminController.DepartmentController{}.GetDepartmentList)
			adminDepartments.GET("/tree", AdminController.DepartmentController{}.GetDepartmentTree)
			adminDepartments.GET("/roots", AdminController.DepartmentController{}.GetRootDepartments)
			adminDepartments.GET("/:id", AdminController.DepartmentController{}.GetDepartment)
			adminDepartments.PUT("/:id", AdminController.DepartmentController{}.UpdateDepartment)
			adminDepartments.DELETE("/:id", AdminController.DepartmentController{}.DeleteDepartment)
			adminDepartments.PUT("/:id/status", AdminController.DepartmentController{}.UpdateDepartmentStatus)
			adminDepartments.PUT("/:id/move", AdminController.DepartmentController{}.MoveDepartment)
			adminDepartments.GET("/:id/children", AdminController.DepartmentController{}.GetChildrenDepartments)
			adminDepartments.GET("/:id/descendants", AdminController.DepartmentController{}.GetDescendantDepartments)
			adminDepartments.POST("/batch", AdminController.DepartmentController{}.BatchOperation)
			adminDepartments.GET("/stats/active-count", AdminController.DepartmentController{}.GetActiveDepartmentsCount)
			adminDepartments.POST("/validate-hierarchy", AdminController.DepartmentController{}.ValidateHierarchy)
			adminDepartments.POST("/repair-hierarchy", AdminController.DepartmentController{}.RepairHierarchy)
			adminDepartments.GET("/:id/users", AdminController.UserController{}.GetUsersByDepartment)

			// 管理员 managers 子域
			adminManagers := admin.Group("/managers")
			adminManagers.Use(middleware.AdminAuthMiddleware())
			adminManagers.GET("/:managerId/departments", AdminController.DepartmentController{}.GetDepartmentsByManager)

			// 管理员应用管理
			adminApps := admin.Group("/applications")
			adminApps.Use(middleware.AdminAuthMiddleware())
			adminApps.POST("", AdminController.ApplicationController{}.CreateApplication)
			adminApps.GET("", AdminController.ApplicationController{}.GetApplicationList)
			adminApps.GET("/:id", AdminController.ApplicationController{}.GetApplication)
			adminApps.PUT("/:id", AdminController.ApplicationController{}.UpdateApplication)
			adminApps.DELETE("/:id", AdminController.ApplicationController{}.DeleteApplication)
			adminApps.GET("/:id/keys", AdminController.ApplicationController{}.GetApplicationKeys)
			adminApps.POST("/:id/keys/rotate", AdminController.ApplicationController{}.RotateApplicationKey)
			adminApps.POST("/:id/secret/reset", AdminController.ApplicationController{}.ResetClientSecret)
			adminApps.GET("/:id/access-scope", AdminController.ApplicationController{}.GetApplicationAccessScope)
			adminApps.PUT("/:id/access-scope", AdminController.ApplicationController{}.UpdateApplicationAccessScope)
			adminApps.POST("/batch", AdminController.ApplicationController{}.BatchOperation)
			adminApps.GET("/stats", AdminController.ApplicationController{}.GetStats)

			// 管理员社会化登录提供商
			adminSocialProviders := admin.Group("/social-providers")
			adminSocialProviders.Use(middleware.AdminAuthMiddleware())
			adminSocialProviders.POST("", AdminController.SocialProviderController{}.CreateProvider)
			adminSocialProviders.GET("", AdminController.SocialProviderController{}.GetProviderList)
			adminSocialProviders.GET("/:id", AdminController.SocialProviderController{}.GetProvider)
			adminSocialProviders.PUT("/:id", AdminController.SocialProviderController{}.UpdateProvider)
			adminSocialProviders.DELETE("/:id", AdminController.SocialProviderController{}.DeleteProvider)
			adminSocialProviders.POST("/batch", AdminController.SocialProviderController{}.BatchOperation)
			adminSocialProviders.GET("/stats", AdminController.SocialProviderController{}.GetProviderStats)

			// 管理员系统设置
			adminSettings := admin.Group("/settings")
			adminSettings.Use(middleware.AdminAuthMiddleware())
			adminSettings.GET("", AdminController.SettingController{}.GetAllSettings)
			adminSettings.GET("/:key", AdminController.SettingController{}.GetSettingByKey)
			adminSettings.POST("", AdminController.SettingController{}.CreateSetting)
			adminSettings.PUT("/:key", AdminController.SettingController{}.UpdateSetting)
			adminSettings.DELETE("/:key", AdminController.SettingController{}.DeleteSetting)
			adminSettings.GET("/register-fields/config", AdminController.SettingController{}.GetRegisterFieldsSettings)
			adminSettings.PUT("/register-fields/config", AdminController.SettingController{}.UpdateRegisterFieldsSettings)
			adminSettings.GET("/push/config", AdminController.SettingController{}.GetPushSettings)
			adminSettings.PUT("/push/config", AdminController.SettingController{}.UpdatePushSettings)
			adminSettings.GET("/captcha/bindings", AdminController.CaptchaConfigController{}.GetCaptchaSceneBindings)
			adminSettings.PUT("/captcha/bindings", AdminController.CaptchaConfigController{}.UpdateCaptchaSceneBindings)

			// 管理员验证码配置
			adminCaptchaConfigs := admin.Group("/captcha-configs")
			adminCaptchaConfigs.Use(middleware.AdminAuthMiddleware())
			adminCaptchaConfigs.GET("", AdminController.CaptchaConfigController{}.GetCaptchaConfigs)
			adminCaptchaConfigs.GET("/providers", AdminController.CaptchaConfigController{}.GetCaptchaProviders)
			adminCaptchaConfigs.GET("/:id", AdminController.CaptchaConfigController{}.GetCaptchaConfig)
			adminCaptchaConfigs.POST("", AdminController.CaptchaConfigController{}.CreateCaptchaConfig)
			adminCaptchaConfigs.PUT("/:id", AdminController.CaptchaConfigController{}.UpdateCaptchaConfig)
			adminCaptchaConfigs.DELETE("/:id", AdminController.CaptchaConfigController{}.DeleteCaptchaConfig)

			// === 普通用户路由 ===
			// 公开：/api/user/login
			userPublic := api.Group("/user")
			userPublic.POST("/login", controller.UserController{}.ValidateLogin)

			// 受保护：/api/user/*
			userAuth := api.Group("/user")
			userAuth.Use(middleware.AuthMiddleware())
			userAuth.GET("/profile", controller.UserController{}.GetUserProfile)
			userAuth.PUT("/profile", controller.UserController{}.UpdateUserProfile)
			userAuth.PUT("/password", controller.UserController{}.ChangePassword)
			userAuth.GET("/social-accounts", controller.SocialAuthController{}.GetUserSocialAccounts)

			// 用户 JWT 令牌管理
			auth := api.Group("/auth")
			auth.Use(middleware.AuthMiddleware())
			auth.POST("/logout", controller.UserController{}.Logout)
			auth.POST("/refresh", controller.UserController{}.RefreshToken)
			auth.GET("/codes", controller.UserController{}.GetAccessCodes)

			// 验证码公开接口
			captcha := api.Group("/captcha")
			captcha.POST("/params", controller.CaptchaController{}.GetCaptchaParams)

			// OIDC 授权流程 API（服务于 /auth 登录 UI）
			apiOidc := api.Group("/oidc")
			apiOidc.POST("/login", controller.OIDCAuthFlowController{}.Login)
			apiOidc.POST("/register", controller.OIDCAuthFlowController{}.Register)
			apiOidc.GET("/auth-info", controller.OIDCAuthFlowController{}.GetAuthInfo)
			apiOidc.GET("/flow-status", controller.OIDCAuthFlowController{}.GetFlowStatus)
			apiOidc.POST("/send-verification-code", controller.OIDCAuthFlowController{}.SendVerificationCode)
			apiOidc.POST("/send-register-verification-code", controller.OIDCAuthFlowController{}.SendRegisterVerificationCode)
			apiOidc.POST("/complete-profile", controller.OIDCAuthFlowController{}.CompleteProfile)
			apiOidc.GET("/register-settings", controller.OIDCAuthFlowController{}.GetRegisterSettings)
			apiOidc.POST("/init-session", controller.OIDCAuthFlowController{}.InitSession)

			// === 社会化登录 ===
			social := router.Group("/social")
			social.GET("/providers", controller.SocialAuthController{}.GetEnabledProviders)
			social.GET("/auth/:provider", controller.SocialAuthController{}.InitiateAuth)
			social.GET("/callback/:provider", controller.SocialAuthController{}.HandleCallback)
			social.POST("/bind-with-login", controller.SocialAuthController{}.BindWithLogin)
			social.POST("/register-with-social", controller.SocialAuthController{}.RegisterWithSocial)

			socialAuth := router.Group("/social")
			socialAuth.Use(middleware.AuthMiddleware())
			socialAuth.POST("/bind/:provider", controller.SocialAuthController{}.BindAccount)
			socialAuth.DELETE("/unbind/:accountId", controller.SocialAuthController{}.UnbindAccount)

			// === OIDC 协议端点（公开，按 RFC 规范） ===
			// 原 Gin .Match(GET|POST, ...) 在此拆成 .GET 和 .POST 双注册
			oidc := router.Group("/oidc")
			oidc.GET("/:client_id/authorize", controller.OIDCController{}.Authorize)
			oidc.POST("/:client_id/authorize", controller.OIDCController{}.Authorize)
			oidc.GET("/:client_id/token", controller.OIDCController{}.Token)
			oidc.POST("/:client_id/token", controller.OIDCController{}.Token)
			oidc.GET("/:client_id/userinfo", controller.OIDCController{}.UserInfo)
			oidc.POST("/:client_id/userinfo", controller.OIDCController{}.UserInfo)
			oidc.POST("/:client_id/introspect", controller.OIDCController{}.Introspect)
			oidc.POST("/:client_id/revoke", controller.OIDCController{}.Revoke)
			oidc.GET("/:client_id/jwks", controller.OIDCController{}.JWKS)
			oidc.GET("/:client_id/.well-known/openid-configuration", controller.OIDCController{}.Discovery)

			// === Webhook ===
			webhookOidc := router.Group("/webhook/oidc")
			webhookOidc.POST("/consent", controller.OIDCAuthFlowController{}.Consent)
		},
	}
}
