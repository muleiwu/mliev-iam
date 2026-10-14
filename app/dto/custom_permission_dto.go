package dto

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"time"
)

type CustomPermissionRequest struct {
	Code        string `json:"code" binding:"max=100"`
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description" binding:"max=2000"`
	Enabled     *bool  `json:"enabled"`
}

type CustomPermissionGrantRequest struct {
	PermissionID          uint       `json:"permissionId" binding:"required"`
	SubjectType           string     `json:"subjectType" binding:"required,oneof=user department role everyone client"`
	SubjectID             uint       `json:"subjectId"`
	IncludeSubDepartments bool       `json:"includeSubDepartments"`
	StartsAt              *time.Time `json:"startsAt"`
	EndsAt                *time.Time `json:"endsAt"`
	Remark                string     `json:"remark" binding:"max=2000"`
}

type CustomPermissionQuery struct {
	BaseQueryRequest
	Keyword      string `form:"keyword"`
	Enabled      *bool  `form:"enabled"`
	PermissionID uint   `form:"permissionId"`
	SubjectType  string `form:"subjectType" binding:"omitempty,oneof=user department role everyone client"`
	SubjectID    uint   `form:"subjectId"`
	State        string `form:"state" binding:"omitempty,oneof=pending active expired"`
}

type CustomPermissionApplication struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
type CustomPermissionResponse struct {
	model.CustomPermission
	Applications []CustomPermissionApplication `json:"applications"`
}
type CustomPermissionGrantResponse struct {
	model.CustomPermissionGrant
	PermissionEnabled bool   `json:"permissionEnabled"`
	PermissionName    string `json:"permissionName"`
	PermissionCode    string `json:"permissionCode"`
	SubjectName       string `json:"subjectName"`
	SubjectAvailable  bool   `json:"subjectAvailable"`
	State             string `json:"state"`
}
