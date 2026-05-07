package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

// UserDepartmentCreateDto represents the data for creating a user department relationship
type UserDepartmentCreateDto struct {
	UserID       uint   `json:"userId" binding:"required"`
	DepartmentID uint   `json:"departmentId" binding:"required"`
	Position     string `json:"position" binding:"omitempty,max=100"`
	Title        string `json:"title" binding:"omitempty,max=100"`
	Type         int    `json:"type" binding:"omitempty,min=0,max=2"` // 0: member, 1: leader, 2: deputy
	IsPrimary    bool   `json:"isPrimary"`
	Sort         int    `json:"sort"`
	Remark       string `json:"remark" binding:"omitempty,max=500"`
}

// UserDepartmentUpdateDto represents the data for updating a user department relationship
type UserDepartmentUpdateDto struct {
	Position  *string `json:"position" binding:"omitempty,max=100"`
	Title     *string `json:"title" binding:"omitempty,max=100"`
	Type      *int    `json:"type" binding:"omitempty,min=0,max=2"`
	IsActive  *bool   `json:"isActive"`
	IsPrimary *bool   `json:"isPrimary"`
	Sort      *int    `json:"sort"`
	Remark    *string `json:"remark" binding:"omitempty,max=500"`
}

// UserDepartmentQueryDto represents the query parameters for listing user departments
type UserDepartmentQueryDto struct {
	UserID         *uint `form:"userId" json:"userId"`
	DepartmentID   *uint `form:"departmentId" json:"departmentId"`
	IsActive       *bool `form:"isActive" json:"isActive"`
	IsPrimary      *bool `form:"isPrimary" json:"isPrimary"`
	Type           *int  `form:"type" json:"type"`
	WithDepartment bool  `form:"withDepartment" json:"withDepartment"`
	WithUser       bool  `form:"withUser" json:"withUser"`
	Page           int   `form:"page" json:"page" binding:"required,min=1"`
	PageSize       int   `form:"pageSize" json:"pageSize" binding:"required,min=1,max=100"`
}

// UserDepartmentResponseDto represents the response data for a user department relationship
type UserDepartmentResponseDto struct {
	UserID       uint                `json:"userId"`
	DepartmentID uint                `json:"departmentId"`
	Position     string              `json:"position"`
	Title        string              `json:"title"`
	Type         int                 `json:"type"`
	TypeName     string              `json:"typeName"`
	IsActive     bool                `json:"isActive"`
	IsPrimary    bool                `json:"isPrimary"`
	Sort         int                 `json:"sort"`
	JoinedAt     *time.Time          `json:"joinedAt"`
	LeftAt       *time.Time          `json:"leftAt"`
	Remark       string              `json:"remark"`
	Department   *DepartmentResponse `json:"department,omitempty"`
	User         *UserResponse       `json:"user,omitempty"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
}

// BatchUserDepartmentDto represents the data for batch operations
type BatchUserDepartmentDto struct {
	UserID        uint   `json:"userId" binding:"required"`
	DepartmentIDs []uint `json:"departmentIds" binding:"required,min=1"`
}

// SetPrimaryDepartmentDto represents the data for setting primary department
type SetPrimaryDepartmentDto struct {
	UserID       uint `json:"userId" binding:"required"`
	DepartmentID uint `json:"departmentId" binding:"required"`
}

// ToUserDepartmentResponseDto converts model.UserDepartment to UserDepartmentResponseDto
func ToUserDepartmentResponseDto(ud *model.UserDepartment) *UserDepartmentResponseDto {
	resp := &UserDepartmentResponseDto{
		UserID:       ud.UserID,
		DepartmentID: ud.DepartmentID,
		Position:     ud.Position,
		Title:        ud.Title,
		Type:         int(ud.Type),
		TypeName:     ud.Type.String(),
		IsActive:     ud.IsActive,
		IsPrimary:    ud.IsPrimary,
		Sort:         ud.Sort,
		JoinedAt:     ud.JoinedAt,
		LeftAt:       ud.LeftAt,
		Remark:       ud.Remark,
		CreatedAt:    ud.CreatedAt,
		UpdatedAt:    ud.UpdatedAt,
	}

	// Include department if loaded
	if ud.Department.ID != 0 {
		resp.Department = &DepartmentResponse{
			ID:          ud.Department.ID,
			Name:        ud.Department.Name,
			Code:        ud.Department.Code,
			ShortName:   ud.Department.ShortName,
			Description: ud.Department.Description,
			Type:        ud.Department.Type,
			TypeText:    ud.Department.Type.String(),
			Status:      ud.Department.Status,
			StatusText:  ud.Department.Status.String(),
			ParentID:    ud.Department.ParentID,
			Level:       ud.Department.Level,
			Path:        ud.Department.Path,
			Sort:        ud.Department.Sort,
			CreatedAt:   ud.Department.CreatedAt,
			UpdatedAt:   ud.Department.UpdatedAt,
		}
	}

	// Include user if loaded
	if ud.User.ID != 0 {
		userResp := ConvertToUserResponse(&ud.User)
		resp.User = &userResp
	}

	return resp
}

// ToUserDepartmentResponseDtoList converts a slice of model.UserDepartment to a slice of UserDepartmentResponseDto
func ToUserDepartmentResponseDtoList(userDepts []model.UserDepartment) []*UserDepartmentResponseDto {
	result := make([]*UserDepartmentResponseDto, 0, len(userDepts))
	for i := range userDepts {
		result = append(result, ToUserDepartmentResponseDto(&userDepts[i]))
	}
	return result
}
