package dto

import (
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/datatypes"
)

// CreateDepartmentRequest 创建部门请求
type CreateDepartmentRequest struct {
	Name          string                 `json:"name" validate:"required,min=2,max=100" binding:"required"`
	Code          string                 `json:"code" validate:"required,min=2,max=50" binding:"required"`
	ShortName     string                 `json:"shortName" validate:"max=50" binding:"omitempty"`
	Description   string                 `json:"description" validate:"max=255" binding:"omitempty"`
	Type          model.DepartmentType   `json:"type" binding:"omitempty"`
	Status        model.DepartmentStatus `json:"status" binding:"omitempty"`
	ParentID      *uint                  `json:"parentId" binding:"omitempty"`
	Sort          int                    `json:"sort" binding:"omitempty"`
	ManagerID     *uint                  `json:"managerId" binding:"omitempty"`
	Phone         string                 `json:"phone" validate:"max=20" binding:"omitempty"`
	Email         string                 `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Address       string                 `json:"address" validate:"max=255" binding:"omitempty"`
	PostalCode    string                 `json:"postalCode" validate:"max=20" binding:"omitempty"`
	Fax           string                 `json:"fax" validate:"max=20" binding:"omitempty"`
	Website       string                 `json:"website" validate:"max=255" binding:"omitempty"`
	Location      string                 `json:"location" validate:"max=255" binding:"omitempty"`
	CostCenter    string                 `json:"costCenter" validate:"max=50" binding:"omitempty"`
	BudgetCode    string                 `json:"budgetCode" validate:"max=50" binding:"omitempty"`
	TaxID         string                 `json:"taxId" validate:"max=50" binding:"omitempty"`
	LegalEntity   string                 `json:"legalEntity" validate:"max=100" binding:"omitempty"`
	BusinessType  string                 `json:"businessType" validate:"max=100" binding:"omitempty"`
	DataScope     int                    `json:"dataScope" binding:"omitempty"`
	DataScopeDesc string                 `json:"dataScopeDesc" validate:"max=255" binding:"omitempty"`
	EstablishedAt *time.Time             `json:"establishedAt" binding:"omitempty"`
	Attributes    datatypes.JSON         `json:"attributes" binding:"omitempty"`
	Remark        string                 `json:"remark" binding:"omitempty"`
}

// UpdateDepartmentRequest 更新部门请求
type UpdateDepartmentRequest struct {
	Name          string                  `json:"name" validate:"min=2,max=100" binding:"omitempty"`
	Code          string                  `json:"code" validate:"min=2,max=50" binding:"omitempty"`
	ShortName     string                  `json:"shortName" validate:"max=50" binding:"omitempty"`
	Description   string                  `json:"description" validate:"max=255" binding:"omitempty"`
	Type          *model.DepartmentType   `json:"type" binding:"omitempty"`
	Status        *model.DepartmentStatus `json:"status" binding:"omitempty"`
	ParentID      *uint                   `json:"parentId" binding:"omitempty"`
	Sort          *int                    `json:"sort" binding:"omitempty"`
	ManagerID     *uint                   `json:"managerId" binding:"omitempty"`
	Phone         string                  `json:"phone" validate:"max=20" binding:"omitempty"`
	Email         string                  `json:"email" validate:"email,max=100" binding:"omitempty,email"`
	Address       string                  `json:"address" validate:"max=255" binding:"omitempty"`
	PostalCode    string                  `json:"postalCode" validate:"max=20" binding:"omitempty"`
	Fax           string                  `json:"fax" validate:"max=20" binding:"omitempty"`
	Website       string                  `json:"website" validate:"max=255" binding:"omitempty"`
	Location      string                  `json:"location" validate:"max=255" binding:"omitempty"`
	CostCenter    string                  `json:"costCenter" validate:"max=50" binding:"omitempty"`
	BudgetCode    string                  `json:"budgetCode" validate:"max=50" binding:"omitempty"`
	TaxID         string                  `json:"taxId" validate:"max=50" binding:"omitempty"`
	LegalEntity   string                  `json:"legalEntity" validate:"max=100" binding:"omitempty"`
	BusinessType  string                  `json:"businessType" validate:"max=100" binding:"omitempty"`
	DataScope     *int                    `json:"dataScope" binding:"omitempty"`
	DataScopeDesc string                  `json:"dataScopeDesc" validate:"max=255" binding:"omitempty"`
	EstablishedAt *time.Time              `json:"establishedAt" binding:"omitempty"`
	Attributes    datatypes.JSON          `json:"attributes" binding:"omitempty"`
	Remark        string                  `json:"remark" binding:"omitempty"`
}

// DepartmentQueryRequest 部门查询请求
type DepartmentQueryRequest struct {
	Page        int                     `form:"page" validate:"min=1" binding:"omitempty,min=1"`
	PageSize    int                     `form:"pageSize" validate:"min=1,max=100" binding:"omitempty,min=1,max=100"`
	Name        string                  `form:"name" binding:"omitempty"`
	Code        string                  `form:"code" binding:"omitempty"`
	Type        *model.DepartmentType   `form:"type" binding:"omitempty"`
	Status      *model.DepartmentStatus `form:"status" binding:"omitempty"`
	ParentID    *uint                   `form:"parentId" binding:"omitempty"`
	ManagerID   *uint                   `form:"managerId" binding:"omitempty"`
	Keyword     string                  `form:"keyword" binding:"omitempty"`     // 关键词搜索（部门名称、编码、简称）
	IncludeTree bool                    `form:"includeTree" binding:"omitempty"` // 是否包含树形结构
}

// DepartmentResponse 部门响应信息
type DepartmentResponse struct {
	ID            uint                   `json:"id"`
	Name          string                 `json:"name"`
	Code          string                 `json:"code"`
	ShortName     string                 `json:"shortName"`
	Description   string                 `json:"description"`
	Type          model.DepartmentType   `json:"type"`
	TypeText      string                 `json:"typeText"`
	Status        model.DepartmentStatus `json:"status"`
	StatusText    string                 `json:"statusText"`
	ParentID      *uint                  `json:"parentId"`
	Level         int                    `json:"level"`
	Path          string                 `json:"path"`
	Sort          int                    `json:"sort"`
	ManagerID     *uint                  `json:"managerId"`
	Phone         string                 `json:"phone"`
	Email         string                 `json:"email"`
	Address       string                 `json:"address"`
	PostalCode    string                 `json:"postalCode"`
	Fax           string                 `json:"fax"`
	Website       string                 `json:"website"`
	Location      string                 `json:"location"`
	CostCenter    string                 `json:"costCenter"`
	BudgetCode    string                 `json:"budgetCode"`
	TaxID         string                 `json:"taxId"`
	LegalEntity   string                 `json:"legalEntity"`
	BusinessType  string                 `json:"businessType"`
	DataScope     int                    `json:"dataScope"`
	DataScopeDesc string                 `json:"dataScopeDesc"`
	IsLeaf        bool                   `json:"isLeaf"`
	HasChildren   bool                   `json:"hasChildren"`
	EstablishedAt *time.Time             `json:"establishedAt"`
	Attributes    datatypes.JSON         `json:"attributes,omitempty"`
	Remark        string                 `json:"remark"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Children      []DepartmentResponse   `json:"children,omitempty"`
	Parent        *DepartmentResponse    `json:"parent,omitempty"`
	Manager       *UserResponse          `json:"manager,omitempty"`
}

// DepartmentListResponse 部门列表响应
type DepartmentListResponse struct {
	Total int                  `json:"total"`
	Page  int                  `json:"page"`
	Size  int                  `json:"size"`
	Items []DepartmentResponse `json:"items"`
}

// DepartmentTreeResponse 部门树形结构响应
type DepartmentTreeResponse struct {
	Tree []DepartmentResponse `json:"tree"`
}

// DepartmentStatusUpdateRequest 部门状态更新请求
type DepartmentStatusUpdateRequest struct {
	Status model.DepartmentStatus `json:"status" validate:"required" binding:"required"`
	Remark string                 `json:"remark" binding:"omitempty"`
}

// BatchDepartmentRequest 批量部门操作请求
type BatchDepartmentRequest struct {
	DepartmentIDs []uint `json:"departmentIds" validate:"required,min=1" binding:"required"`
	Action        string `json:"action" validate:"required,oneof=enable disable freeze delete" binding:"required"`
	Remark        string `json:"remark" binding:"omitempty"`
}

// MoveDepartmentRequest 移动部门请求
type MoveDepartmentRequest struct {
	TargetParentID *uint  `json:"targetParentId" binding:"omitempty"`
	Sort           int    `json:"sort" binding:"omitempty"`
	Remark         string `json:"remark" binding:"omitempty"`
}

// ConvertToDepartmentResponse 将部门模型转换为响应DTO
func ConvertToDepartmentResponse(dept *model.Department) DepartmentResponse {
	response := DepartmentResponse{
		ID:            dept.ID,
		Name:          dept.Name,
		Code:          dept.Code,
		ShortName:     dept.ShortName,
		Description:   dept.Description,
		Type:          dept.Type,
		TypeText:      dept.Type.String(),
		Status:        dept.Status,
		StatusText:    dept.Status.String(),
		ParentID:      dept.ParentID,
		Level:         dept.Level,
		Path:          dept.Path,
		Sort:          dept.Sort,
		ManagerID:     dept.ManagerID,
		Phone:         dept.Phone,
		Email:         dept.Email,
		Address:       dept.Address,
		PostalCode:    dept.PostalCode,
		Fax:           dept.Fax,
		Website:       dept.Website,
		Location:      dept.Location,
		CostCenter:    dept.CostCenter,
		BudgetCode:    dept.BudgetCode,
		TaxID:         dept.TaxID,
		LegalEntity:   dept.LegalEntity,
		BusinessType:  dept.BusinessType,
		DataScope:     dept.DataScope,
		DataScopeDesc: dept.DataScopeDesc,
		IsLeaf:        dept.IsLeaf,
		HasChildren:   dept.HasChildren,
		EstablishedAt: dept.EstablishedAt,
		Attributes:    dept.Attributes,
		Remark:        dept.Remark,
		CreatedAt:     dept.CreatedAt,
		UpdatedAt:     dept.UpdatedAt,
	}

	// 如果有管理员信息，转换管理员信息
	if dept.Manager != nil {
		manager := ConvertToUserResponse(dept.Manager)
		response.Manager = &manager
	}

	// 如果有父部门信息，转换父部门信息
	if dept.Parent != nil {
		parent := ConvertToDepartmentResponse(dept.Parent)
		response.Parent = &parent
	}

	// 如果有子部门信息，转换子部门信息
	if len(dept.Children) > 0 {
		response.Children = ConvertToDepartmentResponses(dept.Children)
	}

	return response
}

// ConvertToDepartmentResponses 批量转换部门模型为响应DTO
func ConvertToDepartmentResponses(depts []model.Department) []DepartmentResponse {
	responses := make([]DepartmentResponse, len(depts))
	for i := range depts {
		responses[i] = ConvertToDepartmentResponse(&depts[i])
	}
	return responses
}
