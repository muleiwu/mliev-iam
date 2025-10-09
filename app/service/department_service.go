package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// DepartmentService 部门业务逻辑层
type DepartmentService struct {
	departmentDao *dao.DepartmentDao
	database      *gorm.DB
}

// NewDepartmentService 创建部门服务实例
func NewDepartmentService(database *gorm.DB) *DepartmentService {
	return &DepartmentService{
		departmentDao: dao.NewDepartmentDao(database),
		database:      database,
	}
}

// CreateDepartment 创建部门
func (s *DepartmentService) CreateDepartment(req *dto.CreateDepartmentRequest) (*dto.DepartmentResponse, error) {
	// 验证部门编码是否已存在
	exists, err := s.departmentDao.ExistsCode(req.Code)
	if err != nil {
		return nil, fmt.Errorf("检查部门编码失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("部门编码已存在")
	}

	// 验证同级部门名称是否已存在
	exists, err = s.departmentDao.ExistsName(req.Name, req.ParentID)
	if err != nil {
		return nil, fmt.Errorf("检查部门名称失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("同级部门名称已存在")
	}

	// 如果有父部门，验证父部门是否存在并且启用
	if req.ParentID != nil {
		parent, err := s.departmentDao.GetByID(*req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("父部门不存在")
		}
		if !parent.IsEnabled() {
			return nil, fmt.Errorf("父部门未启用，无法创建子部门")
		}
	}

	// 创建部门对象
	department := &model.Department{
		Name:          req.Name,
		Code:          req.Code,
		ShortName:     req.ShortName,
		Description:   req.Description,
		Type:          req.Type,
		Status:        req.Status,
		ParentID:      req.ParentID,
		Sort:          req.Sort,
		ManagerID:     req.ManagerID,
		Phone:         req.Phone,
		Email:         req.Email,
		Address:       req.Address,
		PostalCode:    req.PostalCode,
		Fax:           req.Fax,
		Website:       req.Website,
		Location:      req.Location,
		CostCenter:    req.CostCenter,
		BudgetCode:    req.BudgetCode,
		TaxID:         req.TaxID,
		LegalEntity:   req.LegalEntity,
		BusinessType:  req.BusinessType,
		DataScope:     req.DataScope,
		DataScopeDesc: req.DataScopeDesc,
		EstablishedAt: req.EstablishedAt,
		Attributes:    req.Attributes,
		Remark:        req.Remark,
	}

	// 保存到数据库
	if err := s.departmentDao.Create(department); err != nil {
		return nil, err
	}

	// 重新查询以获取完整信息（包括关联数据）
	department, err = s.departmentDao.GetByID(department.ID)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	response := dto.ConvertToDepartmentResponse(department)
	return &response, nil
}

// GetDepartmentByID 根据ID获取部门
func (s *DepartmentService) GetDepartmentByID(id uint) (*dto.DepartmentResponse, error) {
	department, err := s.departmentDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	response := dto.ConvertToDepartmentResponse(department)
	return &response, nil
}

// UpdateDepartment 更新部门信息
func (s *DepartmentService) UpdateDepartment(id uint, req *dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error) {
	// 获取部门
	department, err := s.departmentDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 验证编码是否已存在（排除自己）
	if req.Code != "" && req.Code != department.Code {
		exists, err := s.departmentDao.ExistsCode(req.Code, id)
		if err != nil {
			return nil, fmt.Errorf("检查部门编码失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("部门编码已存在")
		}
		department.Code = req.Code
	}

	// 验证名称是否已存在（排除自己）
	if req.Name != "" && req.Name != department.Name {
		exists, err := s.departmentDao.ExistsName(req.Name, req.ParentID, id)
		if err != nil {
			return nil, fmt.Errorf("检查部门名称失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("同级部门名称已存在")
		}
		department.Name = req.Name
	}

	// 更新字段
	if req.ShortName != "" {
		department.ShortName = req.ShortName
	}
	if req.Description != "" {
		department.Description = req.Description
	}
	if req.Type != nil {
		department.Type = *req.Type
	}
	if req.Status != nil {
		department.Status = *req.Status
	}
	if req.Sort != nil {
		department.Sort = *req.Sort
	}
	if req.ManagerID != nil {
		department.ManagerID = req.ManagerID
	}
	if req.Phone != "" {
		department.Phone = req.Phone
	}
	if req.Email != "" {
		department.Email = req.Email
	}
	if req.Address != "" {
		department.Address = req.Address
	}
	if req.PostalCode != "" {
		department.PostalCode = req.PostalCode
	}
	if req.Fax != "" {
		department.Fax = req.Fax
	}
	if req.Website != "" {
		department.Website = req.Website
	}
	if req.Location != "" {
		department.Location = req.Location
	}
	if req.CostCenter != "" {
		department.CostCenter = req.CostCenter
	}
	if req.BudgetCode != "" {
		department.BudgetCode = req.BudgetCode
	}
	if req.TaxID != "" {
		department.TaxID = req.TaxID
	}
	if req.LegalEntity != "" {
		department.LegalEntity = req.LegalEntity
	}
	if req.BusinessType != "" {
		department.BusinessType = req.BusinessType
	}
	if req.DataScope != nil {
		department.DataScope = *req.DataScope
	}
	if req.DataScopeDesc != "" {
		department.DataScopeDesc = req.DataScopeDesc
	}
	if req.EstablishedAt != nil {
		department.EstablishedAt = req.EstablishedAt
	}
	if req.Attributes != nil {
		department.Attributes = req.Attributes
	}
	if req.Remark != "" {
		department.Remark = req.Remark
	}

	// 保存更新
	if err := s.departmentDao.Update(department); err != nil {
		return nil, err
	}

	// 重新查询以获取最新数据
	department, err = s.departmentDao.GetByID(department.ID)
	if err != nil {
		return nil, err
	}

	response := dto.ConvertToDepartmentResponse(department)
	return &response, nil
}

// DeleteDepartment 删除部门
func (s *DepartmentService) DeleteDepartment(id uint) error {
	// 检查部门是否存在
	_, err := s.departmentDao.GetByID(id)
	if err != nil {
		return err
	}

	return s.departmentDao.Delete(id)
}

// GetDepartmentList 获取部门列表
func (s *DepartmentService) GetDepartmentList(req *dto.DepartmentQueryRequest) (*dto.DepartmentListResponse, error) {
	departments, total, err := s.departmentDao.List(req)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	departmentResponses := dto.ConvertToDepartmentResponses(departments)

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	return &dto.DepartmentListResponse{
		Total: int(total),
		Page:  page,
		Size:  pageSize,
		Items: departmentResponses,
	}, nil
}

// GetDepartmentTree 获取部门树形结构
func (s *DepartmentService) GetDepartmentTree() (*dto.DepartmentTreeResponse, error) {
	// 获取所有启用的部门
	req := &dto.DepartmentQueryRequest{
		Status:      &[]model.DepartmentStatus{model.DepartmentStatusEnabled}[0],
		IncludeTree: true,
	}

	departments, _, err := s.departmentDao.List(req)
	if err != nil {
		return nil, err
	}

	// 构建树形结构
	tree := s.buildDepartmentTree(departments, nil)

	return &dto.DepartmentTreeResponse{
		Tree: tree,
	}, nil
}

// buildDepartmentTree 构建部门树形结构
func (s *DepartmentService) buildDepartmentTree(departments []model.Department, parentID *uint) []dto.DepartmentResponse {
	var tree []dto.DepartmentResponse

	for _, dept := range departments {
		// 检查是否为指定父部门的直接子部门
		if (parentID == nil && dept.ParentID == nil) || (parentID != nil && dept.ParentID != nil && *dept.ParentID == *parentID) {
			response := dto.ConvertToDepartmentResponse(&dept)

			// 递归构建子部门
			response.Children = s.buildDepartmentTree(departments, &dept.ID)

			tree = append(tree, response)
		}
	}

	return tree
}

// UpdateDepartmentStatus 更新部门状态
func (s *DepartmentService) UpdateDepartmentStatus(id uint, req *dto.DepartmentStatusUpdateRequest) error {
	// 检查部门是否存在
	_, err := s.departmentDao.GetByID(id)
	if err != nil {
		return err
	}

	// 更新状态
	if err := s.departmentDao.UpdateStatus(id, req.Status); err != nil {
		return err
	}

	// 如果有备注，更新备注
	if req.Remark != "" {
		updates := map[string]interface{}{
			"remark": req.Remark,
		}
		if err := s.departmentDao.UpdateById(id, updates); err != nil {
			return fmt.Errorf("更新备注失败: %w", err)
		}
	}

	return nil
}

// BatchOperation 批量操作部门
func (s *DepartmentService) BatchOperation(req *dto.BatchDepartmentRequest) error {
	if len(req.DepartmentIDs) == 0 {
		return fmt.Errorf("部门ID列表不能为空")
	}

	switch req.Action {
	case "enable":
		return s.departmentDao.BatchUpdateStatus(req.DepartmentIDs, model.DepartmentStatusEnabled)
	case "disable":
		return s.departmentDao.BatchUpdateStatus(req.DepartmentIDs, model.DepartmentStatusDisabled)
	case "freeze":
		return s.departmentDao.BatchUpdateStatus(req.DepartmentIDs, model.DepartmentStatusFrozen)
	case "delete":
		return s.departmentDao.BatchDelete(req.DepartmentIDs)
	default:
		return fmt.Errorf("不支持的操作类型: %s", req.Action)
	}
}

// MoveDepartment 移动部门
func (s *DepartmentService) MoveDepartment(id uint, req *dto.MoveDepartmentRequest) error {
	// 检查部门是否存在
	_, err := s.departmentDao.GetByID(id)
	if err != nil {
		return err
	}

	// 如果指定了目标父部门，检查父部门是否存在并且启用
	if req.TargetParentID != nil {
		parent, err := s.departmentDao.GetByID(*req.TargetParentID)
		if err != nil {
			return fmt.Errorf("目标父部门不存在")
		}
		if !parent.IsEnabled() {
			return fmt.Errorf("目标父部门未启用")
		}
	}

	// 调用DAO层移动部门
	if err := s.departmentDao.MoveDepartment(id, req.TargetParentID, req.Sort); err != nil {
		return err
	}

	// 如果有备注，更新备注
	if req.Remark != "" {
		updates := map[string]interface{}{
			"remark": req.Remark,
		}
		if err := s.departmentDao.UpdateById(id, updates); err != nil {
			return fmt.Errorf("更新移动备注失败: %w", err)
		}
	}

	return nil
}

// GetChildrenDepartments 获取子部门列表
func (s *DepartmentService) GetChildrenDepartments(parentID uint) ([]dto.DepartmentResponse, error) {
	// 检查父部门是否存在
	_, err := s.departmentDao.GetByID(parentID)
	if err != nil {
		return nil, err
	}

	departments, err := s.departmentDao.GetChildren(parentID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToDepartmentResponses(departments), nil
}

// GetDescendantDepartments 获取所有后代部门
func (s *DepartmentService) GetDescendantDepartments(parentID uint) ([]dto.DepartmentResponse, error) {
	// 检查父部门是否存在
	_, err := s.departmentDao.GetByID(parentID)
	if err != nil {
		return nil, err
	}

	departments, err := s.departmentDao.GetDescendants(parentID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToDepartmentResponses(departments), nil
}

// GetRootDepartments 获取根部门列表
func (s *DepartmentService) GetRootDepartments() ([]dto.DepartmentResponse, error) {
	departments, err := s.departmentDao.GetRootDepartments()
	if err != nil {
		return nil, err
	}

	return dto.ConvertToDepartmentResponses(departments), nil
}

// GetDepartmentsByManager 根据负责人获取部门列表
func (s *DepartmentService) GetDepartmentsByManager(managerID uint) ([]dto.DepartmentResponse, error) {
	departments, err := s.departmentDao.GetByManager(managerID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToDepartmentResponses(departments), nil
}

// GetActiveDepartmentsCount 获取启用部门数量
func (s *DepartmentService) GetActiveDepartmentsCount() (int64, error) {
	return s.departmentDao.GetActiveDepartmentsCount()
}

// ValidateDepartmentHierarchy 验证部门层级结构的完整性
func (s *DepartmentService) ValidateDepartmentHierarchy() error {
	// 获取所有部门
	req := &dto.DepartmentQueryRequest{
		IncludeTree: true,
	}
	departments, _, err := s.departmentDao.List(req)
	if err != nil {
		return fmt.Errorf("获取部门列表失败: %w", err)
	}

	// 检查每个部门的层级关系是否正确
	for _, dept := range departments {
		if dept.ParentID != nil {
			parent, err := s.departmentDao.GetByID(*dept.ParentID)
			if err != nil {
				return fmt.Errorf("部门 %s (ID: %d) 的父部门不存在", dept.Name, dept.ID)
			}

			// 检查层级是否正确
			if dept.Level != parent.Level+1 {
				return fmt.Errorf("部门 %s (ID: %d) 的层级不正确", dept.Name, dept.ID)
			}

			// 检查路径是否正确
			expectedPath := fmt.Sprintf("%s/%d", parent.Path, dept.ID)
			if dept.Path != expectedPath {
				return fmt.Errorf("部门 %s (ID: %d) 的路径不正确", dept.Name, dept.ID)
			}
		} else {
			// 根部门的层级应该为1，路径应该以/开头
			if dept.Level != 1 {
				return fmt.Errorf("根部门 %s (ID: %d) 的层级不正确", dept.Name, dept.ID)
			}
			expectedPath := fmt.Sprintf("/%d", dept.ID)
			if dept.Path != expectedPath {
				return fmt.Errorf("根部门 %s (ID: %d) 的路径不正确", dept.Name, dept.ID)
			}
		}
	}

	return nil
}

// RepairDepartmentHierarchy 修复部门层级结构
func (s *DepartmentService) RepairDepartmentHierarchy() error {
	return s.database.Transaction(func(tx *gorm.DB) error {
		// 获取所有部门，按层级排序
		var departments []model.Department
		if err := tx.Order("level ASC, id ASC").Find(&departments).Error; err != nil {
			return fmt.Errorf("获取部门列表失败: %w", err)
		}

		// 修复每个部门的路径和层级
		for _, dept := range departments {
			if dept.ParentID != nil {
				var parent model.Department
				if err := tx.First(&parent, *dept.ParentID).Error; err != nil {
					return fmt.Errorf("获取父部门失败: %w", err)
				}

				// 更新层级和路径
				dept.Level = parent.Level + 1
				dept.Path = fmt.Sprintf("%s/%d", parent.Path, dept.ID)
			} else {
				// 根部门
				dept.Level = 1
				dept.Path = fmt.Sprintf("/%d", dept.ID)
			}

			// 检查是否有子部门
			var childCount int64
			if err := tx.Model(&model.Department{}).Where("parent_id = ?", dept.ID).Count(&childCount).Error; err != nil {
				return fmt.Errorf("检查子部门数量失败: %w", err)
			}

			dept.HasChildren = childCount > 0
			dept.IsLeaf = childCount == 0

			// 保存更新
			if err := tx.Save(&dept).Error; err != nil {
				return fmt.Errorf("保存部门信息失败: %w", err)
			}
		}

		return nil
	})
}
