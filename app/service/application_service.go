package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Custom errors
var (
	ErrApplicationNotFound    = errors.New("应用不存在")
	ErrApplicationDisabled    = errors.New("应用已被禁用")
	ErrInvalidApplicationIcon = errors.New("应用图标无效")
)

// ApplicationService 应用服务
type ApplicationService struct {
	appDAO          *dao.ApplicationDAO
	attachmentDAO   *dao.AttachmentDao
	protocolFactory *protocol.ProtocolHandlerFactory
	db              *gorm.DB
}

// NewApplicationService 创建新的应用服务
func NewApplicationService(db *gorm.DB) *ApplicationService {
	return &ApplicationService{
		appDAO:          dao.NewApplicationDAO(db),
		attachmentDAO:   dao.NewAttachmentDao(db),
		protocolFactory: protocol.NewProtocolHandlerFactory(db),
		db:              db,
	}
}

// CreateApplication 创建应用
func (s *ApplicationService) CreateApplication(req *dto.CreateApplicationRequest, creatorID uint) (*dto.ApplicationResponse, error) {
	req.Icon = strings.TrimSpace(req.Icon)
	if err := s.validateApplicationIcon(req.Icon); err != nil {
		return nil, err
	}

	// 1. 验证协议类型
	if !s.protocolFactory.IsProtocolSupported(req.ProtocolType) {
		return nil, fmt.Errorf("不支持的协议类型: %s", req.ProtocolType)
	}

	// 2. 获取协议处理器
	handler, err := s.protocolFactory.GetHandler(req.ProtocolType)
	if err != nil {
		return nil, err
	}

	// 3. 验证协议配置
	if err := handler.ValidateConfig(req.ProtocolConfig); err != nil {
		return nil, err
	}

	// 4. 生成 UUID v7 作为应用的唯一标识符
	appUUID, err := uuid.NewV7()
	if err != nil {
		// 如果 UUID v7 生成失败，回退到 v4
		appUUID = uuid.New()
	}

	// 5. 准备协议配置（生成必要的凭据等）
	preparedConfig, err := handler.PrepareConfig(appUUID.String(), req.ProtocolConfig)
	if err != nil {
		return nil, err
	}

	// 6. 创建配置副本（用于返回明文敏感信息）
	plainConfig := make(map[string]interface{})
	for k, v := range preparedConfig {
		plainConfig[k] = v
	}

	// 7. 加密/哈希敏感字段（用于存储）
	if err := handler.EncryptSensitiveFields(preparedConfig); err != nil {
		return nil, fmt.Errorf("加密敏感字段失败: %w", err)
	}

	// 8. 创建应用模型（使用加密后的配置）
	app := &model.Application{
		UUID:            appUUID.String(),
		Name:            req.Name,
		Description:     req.Description,
		Icon:            req.Icon,
		ProtocolType:    req.ProtocolType,
		ProtocolConfig:  preparedConfig,
		CreatedByUserID: &creatorID,
		IsActive:        true,
		IsVerified:      false,
	}

	// 9. 使用事务确保应用和协议特定资源一起创建
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 9.1 先保存应用到数据库（获取ID）
		if err := tx.Create(app).Error; err != nil {
			return fmt.Errorf("保存应用失败: %w", err)
		}

		// 9.2 在事务上下文中创建协议处理器（使用 tx 而不是 db）
		txHandler, err := s.protocolFactory.GetHandlerWithDB(req.ProtocolType, tx)
		if err != nil {
			return err
		}

		// 9.3 执行协议特定的创建逻辑（此时app.ID已经有值）
		if err := txHandler.CreateApplication(app); err != nil {
			return fmt.Errorf("创建应用失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 11. 转换为响应（使用明文配置副本）
	return s.modelToResponse(app, plainConfig), nil
}

// GetApplication 获取应用
func (s *ApplicationService) GetApplication(id uint) (*dto.ApplicationResponse, error) {
	app, err := s.appDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("获取应用失败: %w", err)
	}

	return s.modelToResponse(app, nil), nil
}

// GetApplicationByClientID 根据ClientID获取应用（用于OIDC）
func (s *ApplicationService) GetApplicationByClientID(clientID string) (*model.Application, error) {
	app, err := s.appDAO.GetByClientID(clientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("获取应用失败: %w", err)
	}

	return app, nil
}

// UpdateApplication 更新应用
func (s *ApplicationService) UpdateApplication(id uint, req *dto.UpdateApplicationRequest) (*dto.ApplicationResponse, error) {
	app, err := s.appDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("获取应用失败: %w", err)
	}

	// 获取协议处理器
	handler, err := s.protocolFactory.GetHandler(app.ProtocolType)
	if err != nil {
		return nil, err
	}

	// 准备更新字段
	updates := make(map[string]interface{})

	// 更新基础字段
	if req.Name != nil {
		app.Name = *req.Name
	}
	if req.Description != nil {
		app.Description = *req.Description
	}
	if req.Icon != nil {
		icon := strings.TrimSpace(*req.Icon)
		if err := s.validateApplicationIcon(icon); err != nil {
			return nil, err
		}
		app.Icon = icon
	}
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}

	// 更新协议配置
	if req.ProtocolConfig != nil {
		updates["protocolConfig"] = *req.ProtocolConfig
		if err := handler.UpdateApplication(app, updates); err != nil {
			return nil, fmt.Errorf("更新应用失败: %w", err)
		}
	}

	// 保存到数据库
	if err := s.appDAO.Update(app); err != nil {
		return nil, fmt.Errorf("保存应用失败: %w", err)
	}

	return s.modelToResponse(app, nil), nil
}

// DeleteApplication 删除应用
func (s *ApplicationService) DeleteApplication(id uint) error {
	// 检查应用是否存在
	app, err := s.appDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return fmt.Errorf("获取应用失败: %w", err)
	}

	// 获取协议处理器
	handler, err := s.protocolFactory.GetHandler(app.ProtocolType)
	if err != nil {
		return err
	}

	// 执行协议特定的删除逻辑
	if err := handler.DeleteApplication(id); err != nil {
		return fmt.Errorf("删除应用失败: %w", err)
	}

	// 从数据库删除
	if err := s.appDAO.Delete(id); err != nil {
		return fmt.Errorf("删除应用失败: %w", err)
	}

	return nil
}

// ListApplications 获取应用列表
func (s *ApplicationService) ListApplications(offset, limit int, protocolType *string, isActive *bool) ([]*dto.ApplicationResponse, int64, error) {
	apps, total, err := s.appDAO.List(offset, limit, protocolType, isActive)
	if err != nil {
		return nil, 0, fmt.Errorf("获取应用列表失败: %w", err)
	}

	responses := make([]*dto.ApplicationResponse, len(apps))
	for i, app := range apps {
		responses[i] = s.modelToResponse(app, nil)
	}

	return responses, total, nil
}

// GetStats 获取应用统计信息
func (s *ApplicationService) GetStats() (*dto.ApplicationStats, error) {
	activeCount, err := s.appDAO.CountActive()
	if err != nil {
		return nil, err
	}

	inactiveCount, err := s.appDAO.CountInactive()
	if err != nil {
		return nil, err
	}

	// TODO: 统计活跃令牌数量（需要访问令牌DAO）

	return &dto.ApplicationStats{
		ActiveApplications:   activeCount,
		InactiveApplications: inactiveCount,
		ActiveTokens:         0, // TODO: 实现令牌统计
	}, nil
}

// ValidateApplication 验证应用（用于OIDC客户端验证）
func (s *ApplicationService) ValidateApplication(clientID, clientSecret string) (*model.Application, error) {
	app, err := s.GetApplicationByClientID(clientID)
	if err != nil {
		return nil, err
	}

	if !app.IsActive {
		return nil, ErrApplicationDisabled
	}

	// 只有OIDC协议需要验证客户端密钥
	if app.ProtocolType == string(model.ProtocolTypeOIDC) {
		handler, err := s.protocolFactory.GetHandler(app.ProtocolType)
		if err != nil {
			return nil, err
		}

		oidcHandler, ok := handler.(*protocol.OIDCProtocolHandler)
		if !ok {
			return nil, errors.New("协议处理器类型错误")
		}

		if err := oidcHandler.ValidateClient(app, clientSecret); err != nil {
			return nil, err
		}
	}

	return app, nil
}

// 私有方法

// modelToResponse 将模型转换为响应
func (s *ApplicationService) modelToResponse(app *model.Application, plainConfig map[string]interface{}) *dto.ApplicationResponse {
	response := &dto.ApplicationResponse{
		ID:           app.ID,
		UUID:         app.UUID,
		Name:         app.Name,
		Description:  app.Description,
		Icon:         app.Icon,
		ProtocolType: app.ProtocolType,
		IsActive:     app.IsActive,
		IsVerified:   app.IsVerified,
		CreatedAt:    app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    app.UpdatedAt.Format(time.RFC3339),
	}

	// 如果提供了明文配置（创建操作），使用明文配置
	if plainConfig != nil {
		response.ProtocolConfig = plainConfig
	} else {
		// 对于非创建操作，移除敏感信息
		configCopy := make(map[string]interface{})
		for k, v := range app.ProtocolConfig {
			if k != "clientSecret" {
				configCopy[k] = v
			}
		}
		response.ProtocolConfig = configCopy
	}

	return response
}

// validateApplicationIcon 仅允许引用由应用图标上传接口创建的内部附件。
func (s *ApplicationService) validateApplicationIcon(icon string) error {
	if icon == "" {
		return nil
	}

	const prefix = "/attachments/"
	attachmentUUID, ok := strings.CutPrefix(icon, prefix)
	if !ok || attachmentUUID == "" || strings.Contains(attachmentUUID, "/") {
		return fmt.Errorf("%w：请先通过应用图标上传接口上传图片", ErrInvalidApplicationIcon)
	}
	if _, err := uuid.Parse(attachmentUUID); err != nil {
		return fmt.Errorf("%w：附件地址格式错误", ErrInvalidApplicationIcon)
	}

	attachment, err := s.attachmentDAO.GetByUUID(attachmentUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w：附件不存在", ErrInvalidApplicationIcon)
		}
		return fmt.Errorf("校验应用图标失败: %w", err)
	}
	if attachment.BizType != model.AttachmentBizTypeApplicationIcon {
		return fmt.Errorf("%w：附件类型不匹配", ErrInvalidApplicationIcon)
	}

	return nil
}

// ResetClientSecret 重置应用的客户端密钥（仅 OIDC）
func (s *ApplicationService) ResetClientSecret(appID uint) (string, error) {
	// 检查应用是否存在
	app, err := s.GetApplication(appID)
	if err != nil {
		return "", err
	}

	// 只有 OIDC 协议支持
	if app.ProtocolType != "oidc" {
		return "", errors.New("只有 OIDC 协议应用支持重置客户端密钥")
	}

	// 获取 OIDC 处理器
	handler, err := s.protocolFactory.GetHandler("oidc")
	if err != nil {
		return "", err
	}

	oidcHandler, ok := handler.(*protocol.OIDCProtocolHandler)
	if !ok {
		return "", errors.New("OIDC 处理器类型错误")
	}

	// 重置密钥
	return oidcHandler.ResetClientSecret(appID)
}

// GetAccessScope 获取应用的授权范围
func (s *ApplicationService) GetAccessScope(appID uint) (*dto.ApplicationAccessScopeResponse, error) {
	// 1. 获取应用
	app, err := s.appDAO.GetByID(appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("获取应用失败: %w", err)
	}

	// 2. 准备响应
	response := &dto.ApplicationAccessScopeResponse{
		UserIDs:               []uint{},
		DepartmentIDs:         []uint{},
		RoleIDs:               []uint{},
		IncludeSubDepartments: false,
		Users:                 []dto.AccessScopeUserInfo{},
		Departments:           []dto.AccessScopeDeptInfo{},
		Roles:                 []dto.AccessScopeRoleInfo{},
	}

	// 3. 如果未设置授权范围，返回空配置
	if app.AccessScope == nil || app.AccessScope.IsEmpty() {
		return response, nil
	}

	// 4. 填充基础数据
	response.UserIDs = app.AccessScope.UserIDs
	response.DepartmentIDs = app.AccessScope.DepartmentIDs
	response.RoleIDs = app.AccessScope.RoleIDs
	response.IncludeSubDepartments = app.AccessScope.IncludeSubDepartments

	// 5. 获取用户详细信息
	if len(app.AccessScope.UserIDs) > 0 {
		userDAO := dao.NewUserDao(s.db)
		for _, userID := range app.AccessScope.UserIDs {
			user, err := userDAO.GetByID(userID)
			if err != nil {
				// 如果用户不存在，跳过
				continue
			}
			email := ""
			if user.Email != nil {
				email = *user.Email
			}
			response.Users = append(response.Users, dto.AccessScopeUserInfo{
				ID:       user.ID,
				Username: user.Username,
				RealName: user.RealName,
				Email:    email,
			})
		}
	}

	// 6. 获取部门详细信息
	if len(app.AccessScope.DepartmentIDs) > 0 {
		deptDAO := dao.NewDepartmentDao(s.db)
		for _, deptID := range app.AccessScope.DepartmentIDs {
			dept, err := deptDAO.GetByID(deptID)
			if err != nil {
				// 如果部门不存在，跳过
				continue
			}
			response.Departments = append(response.Departments, dto.AccessScopeDeptInfo{
				ID:   dept.ID,
				Name: dept.Name,
				Code: dept.Code,
				Path: dept.Path,
			})
		}
	}

	// 7. 获取角色详细信息
	if len(app.AccessScope.RoleIDs) > 0 {
		roleDAO := dao.NewRoleDao(s.db)
		for _, roleID := range app.AccessScope.RoleIDs {
			role, err := roleDAO.GetByID(roleID)
			if err != nil {
				// 如果角色不存在，跳过
				continue
			}
			response.Roles = append(response.Roles, dto.AccessScopeRoleInfo{
				ID:   role.ID,
				Name: role.Name,
				Code: role.Code,
			})
		}
	}

	return response, nil
}

// UpdateAccessScope 更新应用的授权范围
func (s *ApplicationService) UpdateAccessScope(appID uint, req *dto.ApplicationAccessScopeRequest) error {
	// 1. 获取应用
	app, err := s.appDAO.GetByID(appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return fmt.Errorf("获取应用失败: %w", err)
	}

	// 2. 更新授权范围
	if req.UserIDs == nil {
		req.UserIDs = []uint{}
	}
	if req.DepartmentIDs == nil {
		req.DepartmentIDs = []uint{}
	}
	if req.RoleIDs == nil {
		req.RoleIDs = []uint{}
	}

	app.AccessScope = &model.ApplicationAccessScope{
		UserIDs:               req.UserIDs,
		DepartmentIDs:         req.DepartmentIDs,
		RoleIDs:               req.RoleIDs,
		IncludeSubDepartments: req.IncludeSubDepartments,
	}

	// 3. 保存到数据库
	if err := s.appDAO.Update(app); err != nil {
		return fmt.Errorf("更新授权范围失败: %w", err)
	}

	return nil
}

// CheckUserAccess 检查用户是否有访问应用的权限
func (s *ApplicationService) CheckUserAccess(appID uint, userID uint) bool {
	// 1. 获取应用
	app, err := s.appDAO.GetByID(appID)
	if err != nil {
		return false
	}

	// 2. 如果未设置授权范围，允许所有用户访问
	if app.AccessScope == nil || app.AccessScope.IsEmpty() {
		return true
	}

	// 3. 检查用户是否在授权列表中
	if app.AccessScope.HasUserID(userID) {
		return true
	}

	// 4. 检查用户的部门是否在授权列表中
	if len(app.AccessScope.DepartmentIDs) > 0 {
		userDeptDAO := dao.NewUserDepartmentDao(s.db)
		userDepts, err := userDeptDAO.FindActiveDepartmentsByUser(userID)
		if err == nil && len(userDepts) > 0 {
			for _, userDept := range userDepts {
				// 检查用户的直接部门
				if app.AccessScope.HasDepartmentID(userDept.DepartmentID) {
					return true
				}

				// 如果设置了包含子部门，检查父级部门
				if app.AccessScope.IncludeSubDepartments {
					// 获取部门详情以检查路径
					deptDAO := dao.NewDepartmentDao(s.db)
					dept, err := deptDAO.GetByID(userDept.DepartmentID)
					if err == nil {
						// 检查用户部门的所有祖先部门
						for _, authDeptID := range app.AccessScope.DepartmentIDs {
							authDept, err := deptDAO.GetByID(authDeptID)
							if err == nil {
								// 如果用户部门路径包含授权部门路径，则用户属于该部门的子部门
								if len(dept.Path) >= len(authDept.Path) &&
									dept.Path[:len(authDept.Path)] == authDept.Path {
									return true
								}
							}
						}
					}
				}
			}
		}
	}

	// 5. 检查用户的角色是否在授权列表中
	if len(app.AccessScope.RoleIDs) > 0 {
		userRoleDAO := dao.NewUserRoleDao(s.db)
		userRoles, err := userRoleDAO.GetByUserID(userID)
		if err == nil && len(userRoles) > 0 {
			for _, userRole := range userRoles {
				if app.AccessScope.HasRoleID(userRole.RoleID) {
					return true
				}
			}
		}
	}

	// 6. 如果所有检查都不通过，拒绝访问
	return false
}
