package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username     string `json:"username" validate:"required,min=3,max=50" binding:"required"`
	Email        string `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Phone        string `json:"phone" validate:"max=20" binding:"omitempty"`
	Nickname     string `json:"nickname" validate:"max=50" binding:"omitempty"`
	RealName     string `json:"realName" validate:"max=50" binding:"omitempty"`
	Avatar       string `json:"avatar" validate:"max=255" binding:"omitempty"`
	Password     string `json:"password" validate:"required,min=6" binding:"required"`
	DepartmentID *uint  `json:"departmentId" binding:"omitempty"`
	Remark       string `json:"remark" binding:"omitempty"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email        string                 `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Phone        string                 `json:"phone" validate:"max=20" binding:"omitempty"`
	Nickname     string                 `json:"nickname" validate:"max=50" binding:"omitempty"`
	RealName     string                 `json:"realName" validate:"max=50" binding:"omitempty"`
	Avatar       string                 `json:"avatar" validate:"max=255" binding:"omitempty"`
	DepartmentID *uint                  `json:"departmentId" binding:"omitempty"`
	Remark       string                 `json:"remark" binding:"omitempty"`
	Attributes   map[string]interface{} `json:"attributes" binding:"omitempty"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required" binding:"required"`
	NewPassword string `json:"newPassword" validate:"required,min=6" binding:"required"`
}

// UserQueryRequest 用户查询请求
type UserQueryRequest struct {
	Page         int    `form:"page" validate:"min=1" binding:"omitempty,min=1"`
	PageSize     int    `form:"pageSize" validate:"min=1,max=100" binding:"omitempty,min=1,max=100"`
	Username     string `form:"username" binding:"omitempty"`
	Email        string `form:"email" binding:"omitempty"`
	Phone        string `form:"phone" binding:"omitempty"`
	RealName     string `form:"realName" binding:"omitempty"`
	Status       *int   `form:"status" binding:"omitempty"`
	DepartmentID *uint  `form:"departmentId" binding:"omitempty"`
	Keyword      string `form:"keyword" binding:"omitempty"` // 关键词搜索（用户名、邮箱、真实姓名）
}

// UserResponse 用户响应信息
type UserResponse struct {
	ID           uint                   `json:"id"`
	Username     string                 `json:"username"`
	Email        string                 `json:"email"`
	Phone        string                 `json:"phone"`
	Nickname     string                 `json:"nickname"`
	RealName     string                 `json:"realName"`
	Avatar       string                 `json:"avatar"`
	Status       model.UserStatus       `json:"status"`
	StatusText   string                 `json:"statusText"`
	IsActivated  bool                   `json:"isActivated"`
	IsVerified   bool                   `json:"isVerified"`
	LoginCount   int64                  `json:"loginCount"`
	FailureCount int                    `json:"failureCount"`
	LastLoginAt  *time.Time             `json:"lastLoginAt"`
	LastLoginIP  string                 `json:"lastLoginIP"`
	DepartmentID *uint                  `json:"departmentId"`
	Attributes   map[string]interface{} `json:"attributes,omitempty"`
	Remark       string                 `json:"remark"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
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
	return UserResponse{
		ID:           user.ID,
		Username:     user.Username,
		Email:        user.Email,
		Phone:        user.Phone,
		Nickname:     user.Nickname,
		RealName:     user.RealName,
		Avatar:       user.Avatar,
		Status:       user.Status,
		StatusText:   user.Status.String(),
		IsActivated:  user.IsActivated,
		IsVerified:   user.IsVerified,
		LoginCount:   user.LoginCount,
		FailureCount: user.FailureCount,
		LastLoginAt:  user.LastLoginAt,
		LastLoginIP:  user.LastLoginIP,
		DepartmentID: user.DepartmentID,
		Attributes:   user.Attributes,
		Remark:       user.Remark,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
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
