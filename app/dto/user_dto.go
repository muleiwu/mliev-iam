package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50" binding:"required"`
	Email    string `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Phone    string `json:"phone" validate:"max=20" binding:"omitempty"`
	Nickname string `json:"nickname" validate:"max=50" binding:"omitempty"`
	RealName string `json:"realName" validate:"max=50" binding:"omitempty"`
	Avatar   string `json:"avatar" validate:"max=255" binding:"omitempty"`
	Password string `json:"password" validate:"required,min=6" binding:"required"`
	Remark   string `json:"remark" binding:"omitempty"`
	// Attributes 扩展属性，写入用户 attributes（如旧系统身份映射信息）。
	// 保留键 email_verified / phone_number_verified 会在落库时被过滤。
	Attributes map[string]interface{} `json:"attributes" binding:"omitempty"`
}

// UpdateUserRequest 更新用户请求
// 指针字段 (*bool / *UserStatus) 用于区分"未提供"(nil) 与 "置为 false/零值"。
type UpdateUserRequest struct {
	Email         string                 `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Phone         string                 `json:"phone" validate:"max=20" binding:"omitempty"`
	Nickname      string                 `json:"nickname" validate:"max=50" binding:"omitempty"`
	RealName      string                 `json:"realName" validate:"max=50" binding:"omitempty"`
	Avatar        string                 `json:"avatar" validate:"max=255" binding:"omitempty"`
	Remark        string                 `json:"remark" binding:"omitempty"`
	Attributes    map[string]interface{} `json:"attributes" binding:"omitempty"`
	IsActivated   *bool                  `json:"isActivated" binding:"omitempty"`
	Status        *model.UserStatus      `json:"status" binding:"omitempty,oneof=1 2"`
	EmailVerified *bool                  `json:"emailVerified" binding:"omitempty"`
	PhoneVerified *bool                  `json:"phoneVerified" binding:"omitempty"`
}

// UpdateMyProfileRequest 个人中心自助更新请求（严格白名单）。
// 不含邮箱/手机号（需验证流程）、状态/验证位/备注（管理员字段），
// 防止自助接口被用于越权修改或清空敏感字段。
type UpdateMyProfileRequest struct {
	Nickname string `json:"nickname" binding:"omitempty,max=50"`
	RealName string `json:"realName" binding:"omitempty,max=50"`
	Avatar   string `json:"avatar" binding:"omitempty,max=255"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required" binding:"required"`
	NewPassword string `json:"newPassword" validate:"required,min=6" binding:"required"`
}

// SetInitialPasswordRequest 设置初始密码请求（仅限当前无密码的用户，如社交注册用户）
type SetInitialPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// AccountLoginRequest 个人中心独立登录请求（不依赖 OIDC session）
type AccountLoginRequest struct {
	Username      string         `json:"username" binding:"required"`
	Password      string         `json:"password" binding:"required"`
	CaptchaTicket *CaptchaTicket `json:"captchaTicket,omitempty"`
	RememberMe    bool           `json:"rememberMe"`
}

// SendContactVerificationCodeRequest 个人中心改绑邮箱/手机发送验证码请求
type SendContactVerificationCodeRequest struct {
	Type          string         `json:"type" binding:"required,oneof=email phone"`
	Receiver      string         `json:"receiver" binding:"required"`
	CaptchaTicket *CaptchaTicket `json:"captchaTicket,omitempty"`
}

// ChangeEmailRequest 改绑邮箱请求
type ChangeEmailRequest struct {
	Email string `json:"email" binding:"required,email,max=100"`
	Code  string `json:"code" binding:"required,len=6"`
}

// ChangePhoneRequest 改绑手机号请求
type ChangePhoneRequest struct {
	Phone string `json:"phone" binding:"required,max=20"`
	Code  string `json:"code" binding:"required,len=6"`
}

// UserQueryRequest 用户查询请求
type UserQueryRequest struct {
	Page      int    `form:"page" validate:"min=1" binding:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" validate:"min=1,max=100" binding:"omitempty,min=1,max=100"`
	UUID      string `form:"uuid" binding:"omitempty"`
	Username  string `form:"username" binding:"omitempty"`
	Email     string `form:"email" binding:"omitempty"`
	Phone     string `form:"phone" binding:"omitempty"`
	RealName  string `form:"realName" binding:"omitempty"`
	Status    *int   `form:"status" binding:"omitempty"`
	Keyword   string `form:"keyword" binding:"omitempty"`   // 关键词搜索（用户名、邮箱、真实姓名）
	SortBy    string `form:"sortBy" binding:"omitempty"`    // 排序字段：loginCount, lastLoginAt, createdAt
	SortOrder string `form:"sortOrder" binding:"omitempty"` // 排序方向：asc, desc
}

// UserResponse 用户响应信息
type UserResponse struct {
	ID              uint                   `json:"id"`
	UUID            string                 `json:"uuid"`
	Username        string                 `json:"username"`
	Email           string                 `json:"email"`
	Phone           string                 `json:"phone"`
	Nickname        string                 `json:"nickname"`
	RealName        string                 `json:"realName"`
	Avatar          string                 `json:"avatar"`
	Status          model.UserStatus       `json:"status"`
	StatusText      string                 `json:"statusText"`
	IsActivated     bool                   `json:"isActivated"`
	IsVerified      bool                   `json:"isVerified"` // 派生：EmailVerifiedAt 或 PhoneVerifiedAt 任一非空
	EmailVerified   bool                   `json:"emailVerified"`
	PhoneVerified   bool                   `json:"phoneVerified"`
	EmailVerifiedAt *time.Time             `json:"emailVerifiedAt"`
	PhoneVerifiedAt *time.Time             `json:"phoneVerifiedAt"`
	HasPassword     bool                   `json:"hasPassword"` // 是否已设置密码（社交注册用户可能没有）
	LoginCount      int64                  `json:"loginCount"`
	LastLoginAt     *time.Time             `json:"lastLoginAt"`
	LastLoginIP     string                 `json:"lastLoginIP"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Remark          string                 `json:"remark"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

// UserListResponse 用户列表响应
type UserListResponse struct {
	Total int            `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Items []UserResponse `json:"items"`
}

// UserStatusUpdateRequest 用户状态更新请求
type UserStatusUpdateRequest struct {
	Status model.UserStatus `json:"status" validate:"required" binding:"required"`
	Remark string           `json:"remark" binding:"omitempty"`
}

// BatchUserRequest 批量用户操作请求
type BatchUserRequest struct {
	UserIDs []uint `json:"userIds" validate:"required,min=1" binding:"required"`
	Action  string `json:"action" validate:"required,oneof=activate deactivate lock unlock delete" binding:"required"`
	Remark  string `json:"remark" binding:"omitempty"`
}

// ConvertToUserResponse 将用户模型转换为响应DTO
func ConvertToUserResponse(user *model.User) UserResponse {
	// 处理指针类型字段
	email := ""
	if user.Email != nil {
		email = *user.Email
	}
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}

	emailVerified := user.EmailVerified()
	phoneVerified := user.PhoneVerified()

	return UserResponse{
		ID:              user.ID,
		UUID:            user.UUID,
		Username:        user.Username,
		Email:           email,
		Phone:           phone,
		Nickname:        user.Nickname,
		RealName:        user.RealName,
		Avatar:          user.Avatar,
		Status:          user.Status,
		StatusText:      user.Status.String(),
		IsActivated:     user.IsActivated,
		IsVerified:      emailVerified || phoneVerified,
		EmailVerified:   emailVerified,
		PhoneVerified:   phoneVerified,
		EmailVerifiedAt: user.EmailVerifiedAt,
		PhoneVerifiedAt: user.PhoneVerifiedAt,
		HasPassword:     user.PasswordHash != "",
		LoginCount:      user.LoginCount,
		LastLoginAt:     user.LastLoginAt,
		LastLoginIP:     user.LastLoginIP,
		Attributes:      user.Attributes,
		Remark:          user.Remark,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}
}

// ConvertToUserResponses 批量转换用户模型为响应DTO
func ConvertToUserResponses(users []model.User) []UserResponse {
	responses := make([]UserResponse, len(users))
	for i := range users {
		responses[i] = ConvertToUserResponse(&users[i])
	}
	return responses
}
