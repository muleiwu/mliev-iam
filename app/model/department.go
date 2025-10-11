package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DepartmentType 部门类型枚举
type DepartmentType int

const (
	DepartmentTypeCompany    DepartmentType = iota // 公司
	DepartmentTypeDivision                         // 事业部
	DepartmentTypeDepartment                       // 部门
	DepartmentTypeTeam                             // 团队/小组
)

// String returns the string representation of DepartmentType
func (dt DepartmentType) String() string {
	switch dt {
	case DepartmentTypeCompany:
		return "company"
	case DepartmentTypeDivision:
		return "division"
	case DepartmentTypeDepartment:
		return "department"
	case DepartmentTypeTeam:
		return "team"
	default:
		return "unknown"
	}
}

// DepartmentStatus 部门状态枚举
type DepartmentStatus int

const (
	DepartmentStatusDisabled DepartmentStatus = iota // 禁用
	DepartmentStatusEnabled                          // 启用
	DepartmentStatusFrozen                           // 冻结
)

// String returns the string representation of DepartmentStatus
func (ds DepartmentStatus) String() string {
	switch ds {
	case DepartmentStatusDisabled:
		return "disabled"
	case DepartmentStatusEnabled:
		return "enabled"
	case DepartmentStatusFrozen:
		return "frozen"
	default:
		return "unknown"
	}
}

// Department represents a department/organization entity in the database.
// 部门模型用于组织架构管理，支持层级结构和数据权限隔离
type Department struct {
	// 基础信息
	ID          uint             `gorm:"primaryKey" json:"id"`
	Name        string           `gorm:"size:100;not null" json:"name" validate:"required,min=2,max=100"`
	Code        string           `gorm:"uniqueIndex;size:50;not null" json:"code" validate:"required,min=2,max=50"`
	ShortName   string           `gorm:"size:50" json:"shortName" validate:"max=50"`
	Description string           `gorm:"size:255" json:"description" validate:"max=255"`
	Type        DepartmentType   `gorm:"type:tinyint;default:2;not null" json:"type"`
	Status      DepartmentStatus `gorm:"type:tinyint;default:1;not null" json:"status"`

	// 层级关系
	ParentID *uint  `gorm:"index" json:"parentId"`
	Level    int    `gorm:"default:1;not null" json:"level"`
	Path     string `gorm:"size:500" json:"path"` // 层级路径，如: /1/2/3/
	Sort     int    `gorm:"default:0" json:"sort"`

	// 组织信息
	ManagerID    *uint  `gorm:"index" json:"managerId"`       // 部门负责人ID
	Phone        string `gorm:"size:20" json:"phone"`         // 部门电话
	Email        string `gorm:"size:100" json:"email"`        // 部门邮箱
	Address      string `gorm:"size:255" json:"address"`      // 部门地址
	PostalCode   string `gorm:"size:20" json:"postalCode"`    // 邮政编码
	Fax          string `gorm:"size:20" json:"fax"`           // 传真
	Website      string `gorm:"size:255" json:"website"`      // 部门网站
	Location     string `gorm:"size:255" json:"location"`     // 地理位置
	CostCenter   string `gorm:"size:50" json:"costCenter"`    // 成本中心
	BudgetCode   string `gorm:"size:50" json:"budgetCode"`    // 预算代码
	TaxID        string `gorm:"size:50" json:"taxId"`         // 税务识别号
	LegalEntity  string `gorm:"size:100" json:"legalEntity"`  // 法律实体
	BusinessType string `gorm:"size:100" json:"businessType"` // 业务类型

	// 数据权限
	DataScope     int    `gorm:"default:1;not null" json:"dataScope"` // 数据权限范围：1全部 2本部门 3本部门及下级 4仅本人
	DataScopeDesc string `gorm:"size:255" json:"dataScopeDesc"`       // 数据权限描述
	IsLeaf        bool   `gorm:"default:true" json:"isLeaf"`          // 是否叶子节点
	HasChildren   bool   `gorm:"default:false" json:"hasChildren"`    // 是否有子部门

	// 时间信息
	EstablishedAt *time.Time     `json:"establishedAt"` // 成立时间
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// 扩展属性
	Attributes datatypes.JSON `gorm:"type:json" json:"attributes,omitempty"`
	Remark     string         `gorm:"type:text" json:"remark"`

	// 关联关系
	Children []Department `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Parent   *Department  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Manager  *User        `gorm:"foreignKey:ManagerID" json:"manager,omitempty"`
	Users    []User       `gorm:"foreignKey:DepartmentID" json:"users,omitempty"`
}

// TableName returns the table name for Department model.
func (*Department) TableName() string {
	return "departments"
}

// BeforeCreate hook to set default values before creating
func (d *Department) BeforeCreate(tx *gorm.DB) error {
	if d.Status == 0 {
		d.Status = DepartmentStatusEnabled
	}
	if d.Type == 0 {
		d.Type = DepartmentTypeDepartment
	}
	if d.Level == 0 {
		d.Level = 1
	}
	if d.DataScope == 0 {
		d.DataScope = 2 // 默认本部门权限
	}
	return nil
}

// AfterCreate hook to update path and parent's hasChildren flag
func (d *Department) AfterCreate(tx *gorm.DB) error {
	if d.ParentID != nil {
		// 查询父部门路径
		var parent Department
		if err := tx.First(&parent, *d.ParentID).Error; err != nil {
			return err
		}
		d.Path = parent.Path + "/" + string(rune(d.ID))
		d.Level = parent.Level + 1

		// 更新父部门的hasChildren标志
		if err := tx.Model(&parent).Updates(map[string]interface{}{
			"has_children": true,
			"is_leaf":      false,
		}).Error; err != nil {
			return err
		}
	} else {
		d.Path = "/" + string(rune(d.ID))
		d.Level = 1
	}

	return tx.Model(d).Updates(map[string]interface{}{
		"path":  d.Path,
		"level": d.Level,
	}).Error
}

// AfterDelete hook to update parent's hasChildren flag if needed
func (d *Department) AfterDelete(tx *gorm.DB) error {
	if d.ParentID != nil {
		// 检查父部门是否还有其他子部门
		var count int64
		if err := tx.Model(&Department{}).Where("parent_id = ?", *d.ParentID).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			// 更新父部门的hasChildren标志
			if err := tx.Model(&Department{}).Where("id = ?", *d.ParentID).Updates(map[string]interface{}{
				"has_children": false,
				"is_leaf":      true,
			}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// IsEnabled checks if department is enabled
func (d *Department) IsEnabled() bool {
	return d.Status == DepartmentStatusEnabled
}

// IsFrozen checks if department is frozen
func (d *Department) IsFrozen() bool {
	return d.Status == DepartmentStatusFrozen
}

// IsCompany checks if department is company type
func (d *Department) IsCompany() bool {
	return d.Type == DepartmentTypeCompany
}

// IsDivision checks if department is division type
func (d *Department) IsDivision() bool {
	return d.Type == DepartmentTypeDivision
}

// IsTeam checks if department is team type
func (d *Department) IsTeam() bool {
	return d.Type == DepartmentTypeTeam
}

// Enable enables the department
func (d *Department) Enable() {
	d.Status = DepartmentStatusEnabled
}

// Disable disables the department
func (d *Department) Disable() {
	d.Status = DepartmentStatusDisabled
}

// Freeze freezes the department
func (d *Department) Freeze() {
	d.Status = DepartmentStatusFrozen
}

// IsRoot checks if department is root level
func (d *Department) IsRoot() bool {
	return d.ParentID == nil
}

// GetAncestors returns all ancestor departments from path
func (d *Department) GetAncestors() []uint {
	// 解析路径获取祖先ID列表
	// 实现略，需要解析Path字段
	return []uint{}
}

// CanManageDataScope checks if department can manage data in specified scope
func (d *Department) CanManageDataScope(targetScope int) bool {
	return d.DataScope <= targetScope
}

// SetManager sets the department manager
func (d *Department) SetManager(managerID uint) {
	d.ManagerID = &managerID
}

// RemoveManager removes the department manager
func (d *Department) RemoveManager() {
	d.ManagerID = nil
}

// GetFullName returns the full department name including parent names
func (d *Department) GetFullName() string {
	// 实现略，需要根据层级路径构建完整名称
	return d.Name
}
