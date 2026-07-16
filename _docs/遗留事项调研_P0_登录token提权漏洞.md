# 遗留事项调研 · P0 — 登录 Token 空 role 回退提权漏洞

> 影响等级：**P0（存量提权漏洞，建议立即修复）**
> 调研日期：2026-07-15
> 关联改动：个人中心功能（feat/oidc-prompt-create）在修复 `AdminAuthMiddleware` 越权时暴露出此存量问题。

## 1. 漏洞核心

`internal/pkg/jwt/jwt.go:46-49` 在 `Role` 为空时静默回退为 `admin`：

```go
role := sub.Role
if role == "" {
    role = "admin"   // ← 空 role 静默提权为 admin
}
```

而 `app/service/jwt_auth_service.go:52-58` 的 `JWTAuthService.GenerateToken` **从不设置 Role**：

```go
func (s *JWTAuthService) GenerateToken(user *dto.UserResponse) (string, error) {
    return s.jwtService.GenerateToken(jwt.TokenSubject{
        UserID:   user.ID,
        Username: user.Username,
        // 无 Role 字段 → 底层回退 "admin"
    }, s.config.GetAccessTokenHours())
}
```

因此该方法签发的**每一个** token 的 claim 都是 `role: "admin"`。`app/middleware/auth_middleware.go` 的 `AdminAuthMiddleware` 仅凭 `role == "admin"` 放行，且 `extractTokenCandidates` 在存在 `Authorization: Bearer` 头时严格采信该头。

**攻击路径**：攻击者用任意注册账号走一条 `sessionId` 为空的社交/身份源/小程序登录，从 JSON 响应体取出 `token`，塞进 `Authorization: Bearer` 头，即可访问全部 `/api/admin/*` 与 `/api/system/*`。

> 注意：本次个人中心改动已修复 `AdminAuthMiddleware` 角色检查晚于 `c.Next()` 的越权问题，反而使这些 admin 角色的响应体 token 成为当前唯一的后台绕过通道，优先级因此更高。

## 2. 完整调用点清单（`jwtAuthService.GenerateToken(` 共 9 处）

| # | 文件:行号 | 所在函数 | token 去向 |
|---|-----------|----------|-----------|
| 1 | `app/controller/admin/auth_controller.go:81` | `AuthController.Login` | 管理员登录，写入 `AdminLoginResponse.AccessToken` + `admin_access_token` Cookie。**唯一合法应持 admin** |
| 2 | `app/service/social_auth_service.go:405` | `loginWithSocialAccount` | `SocialAuthCallbackResponse.Token` |
| 3 | `app/service/social_auth_service.go:524` | `registerWithSocialAccount` | 同上（新用户注册） |
| 4 | `app/service/social_auth_service.go:778` | `BindWithExistingUser` | 同上 |
| 5 | `app/service/social_auth_service.go:918` | `RegisterWithSocial` | 同上 |
| 6 | `app/service/identity_auth_service.go:271` | `BindWithExistingUser` | 同上（企业身份源） |
| 7 | `app/service/identity_auth_service.go:402` | `RegisterWithProvider` | 同上 |
| 8 | `app/service/identity_auth_service.go:473` | `loginWithIdentityAccount` | 同上 |
| 9 | `app/service/social_qrcode_service.go:174` | `SocialQRCodeService.GetStatus` | `MPQRCodeStatusResponse.Token`（扫码 confirmed 时） |

DTO 字段：`app/dto/social_login_dto.go:138`（`SocialAuthCallbackResponse.Token`）、`:203`（`MPQRCodeStatusResponse.Token`）。

对比：`GenerateUserToken`（显式 `Role: "user"`，`jwt_auth_service.go:62`）目前**只**被 `app/controller/user_session_cookie.go:25` 的 `issueUserSessionCookie` 使用（个人中心 Cookie）。上述 8 个业务登录点仍走旧的 `GenerateToken`。

### 响应体 token vs 会话 Cookie 是两套

登录成功路径同时做两件事：
- 调 `issueUserSessionCookie(...)` 下发 `user_access_token` Cookie —— 用 `GenerateUserToken`，**role=user，正确**。
- 把 service 返回的 `response.Token`（admin 角色）放进 JSON 响应体。

控制器对响应体 token 的处理（`social_auth_controller.go` / `identity_auth_controller.go`）：**OIDC 流程**（`sessionID != ""`）走 postMessage HTML 不返回 token，或标记成功后 `response.Token = ""`；但**非 OIDC 流程**（`sessionID == ""`）或会话标记失败时，admin token 原样进 JSON 响应体。

**结论**：任何 `sessionId` 为空的社交/身份/小程序登录都会把 role=admin 的 JWT 吐到 HTTP 响应体。

## 3. 前端消费分析（判定修复是否破坏功能）

### frontend-oidc
- 响应体 `token` 字段**未被当作 Bearer 使用**。唯一注入 `Authorization` 头的地方是 `request.ts:116-117`，取的是 `authStore.accessToken`；而它只由 `/api/oidc/login` 的 `LoginResponse.accessToken` 赋值——那是 **OIDC 应用签名 token**（`GenerateTokens`），与本漏洞的 `GenerateToken` 无关。
- 社交/身份登录在该前端全程是 OIDC 模式（`LoginForm.vue` 强制要求 `sessionId`），走 popup + postMessage，回调只取 `sessionId` 做路由跳转，**从不读取 token**。
- 小程序扫码：`MiniProgramQRLoginModal.vue` emit 的 `status.token` 被 `LoginForm.vue handleMPConfirmed` 完全忽略。
- 个人中心 API（`api/user.ts`）显式 `skipAuth`，靠 `user_access_token` Cookie 鉴权，刻意不带 Bearer。

### admin-web
- 全量搜索无任何 `Bearer` / `Authorization` / `accessToken` 使用，完全依赖 `admin_access_token` HttpOnly Cookie。

**影响判定**：把 8 个调用点改用 role=user 的 token **不破坏任何现有前端功能**——这些响应体 token 在两个前端都未被消费为凭据。真正驱动会话的是各自的 HttpOnly Cookie（user / admin / refresh_token）。响应体 token 至多是历史遗留字段。

## 4. admin 登录路径（唯一合法 admin）

`app/controller/admin/auth_controller.go:39-108 Login`：
1. 密码校验 `ValidateUserForLogin`。
2. **权限门槛** `isAdminUser`（`:200-230`）：查询用户是否关联有效的 `super_admin` 系统角色，非管理员在此 403。
3. 通过后 `GenerateToken` 签发 → `admin_access_token` Cookie + 响应体 `accessToken`。

**关键**：admin 登录本身也走 `GenerateToken` 且**不传 role**，即它**依赖空 role 回退为 admin** 才拿到管理员身份。因此任何"去掉/改变 admin 回退"的修复，**必须同步给 admin 登录一条显式获取 admin 角色的通路**，否则管理员无法登录后台。

## 5. jwt 包定位

- `internal/pkg/jwt/doc.go` 明确该包设计为**可抽取的独立 SDK**（依赖边界禁止引用 `app/*`）。这正是"默认 admin"回退极其危险的原因——SDK 化后任何外部调用方漏填 role 都会静默拿到 admin。
- 底层 `jwt.JWTService.GenerateToken` 的唯一调用者是 `jwt_auth_service.go` 的两个包装方法，全仓无其他直接调用。
- `JWTAuthService.GetJWTService()` 导出了底层实例（当前无调用方），属潜在的空 role 直调入口。

## 6. 修复方案评估

### 方案 a：非 admin 登录点改用 `GenerateUserToken`
- 改动：调用点 2-9（8 处）`GenerateToken` → `GenerateUserToken`，admin 登录不变。
- 风险：**低**，前端无感。但**治标不治本**——`jwt.go` 的 admin 回退仍在，未来新调用点漏设 role 会再次踩坑。
- 定位：最小热修。

### 方案 b：去掉 admin 回退 + admin 登录显式传 admin（推荐）
- 改动：
  1. `JWTAuthService` 新增 `GenerateAdminToken`（显式 `Role: "admin"`），`AuthController.Login` 切过去；
  2. 8 个业务点切 `GenerateUserToken`；
  3. 删除 `jwt.go:46-49` 的 admin 回退（空 role 视为非法或保持空）。
- 风险：**中**（面大可控）。**必须先确认 admin 登录已改为显式 admin**，否则后台登录失效。
- 收益最大：默认不再是 admin，根除同类回归。

### 方案 c：空 role 回退改为 `user`
- 改动一行（`jwt.go:48` `admin` → `user`）。
- 风险：**高**——admin 登录也依赖空 role 回退，改后管理员 token 变 role=user，直接击穿后台登录（除非同步改 admin 登录）。语义上"隐式默认"仍在，不如方案 b 干净。
- 定位：不单独采用。

**综合推荐**：方案 a（立即消除提权）+ 方案 b（根因治理）。落地顺序：
1. 先给 `JWTAuthService` 加 `GenerateAdminToken` 并让 admin 登录切过去；
2. 把 8 个业务点切到 `GenerateUserToken`；
3. 最后删除 `jwt.go` 的 admin 回退。

## 7. 验证方法

**单元/集成层**
- 对 7 个 service 登录方法断言返回 `response.Token` 经 `ValidateToken` 解析后 `claims.Role == "user"`。
- 对 admin `AuthController.Login` 返回的 `accessToken` 断言 `claims.Role == "admin"`（防方案 b/c 误伤）。

**端到端黑盒（提权验证）**
1. 走一条 `sessionId` 为空的社交/身份/小程序登录，从 JSON 响应体取 `token`。
2. `curl -H "Authorization: Bearer <token>" /api/admin/users`：修复前 `200`（漏洞存在），修复后 `403`。
3. 反向确认：同一 user token 访问普通用户接口应 `200`；admin 用 `admin_access_token` 访问 `/api/admin/*` 应 `200`。

## 8. 影响文件速查

- `internal/pkg/jwt/jwt.go`（46-49 回退）、`internal/pkg/jwt/doc.go`（SDK 定位）
- `app/service/jwt_auth_service.go`（GenerateToken / GenerateUserToken）
- `app/middleware/auth_middleware.go`（extractTokenCandidates / AdminAuthMiddleware）
- `app/controller/admin/auth_controller.go`（Login / isAdminUser）
- `app/service/social_auth_service.go`（405/524/778/918）
- `app/service/identity_auth_service.go`（271/402/473）
- `app/service/social_qrcode_service.go`（174）
- `app/controller/social_auth_controller.go` / `identity_auth_controller.go`（响应体 token 处理分支）
- `app/dto/social_login_dto.go`（138/203）
