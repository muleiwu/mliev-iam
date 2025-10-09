package service

import (
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"gorm.io/gorm"
)

// Custom errors
var (
	ErrApplicationNotFound = errors.New("应用不存在")
	ErrApplicationDisabled = errors.New("应用已被禁用")
)

// ApplicationService 应用服务
type ApplicationService struct {
	appDAO          *dao.ApplicationDAO
	protocolFactory *protocol.ProtocolHandlerFactory
	db              *gorm.DB
}

// NewApplicationService 创建新的应用服务
func NewApplicationService(db *gorm.DB) *ApplicationService {
	return &ApplicationService{
		appDAO:          dao.NewApplicationDAO(db),
		protocolFactory: protocol.NewProtocolHandlerFactory(db),
		db:              db,
	}
}

// CreateApplication 创建应用
func (s *ApplicationService) CreateApplication(req *dto.CreateApplicationRequest, creatorID uint) (*dto.ApplicationResponse, error) {
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

	// 4. 准备协议配置（生成必要的凭据等）
	preparedConfig, err := handler.PrepareConfig(req.ProtocolConfig)
	if err != nil {
		return nil, err
	}

	// 5. 创建应用模型
	app := &model.Application{
		Name:            req.Name,
		Description:     req.Description,
		ProtocolType:    req.ProtocolType,
		ProtocolConfig:  preparedConfig,
		CreatedByUserID: &creatorID,
		IsActive:        true,
		IsVerified:      false,
	}

	// 6. 执行协议特定的创建逻辑
	if err := handler.CreateApplication(app); err != nil {
		return nil, fmt.Errorf("创建应用失败: %w", err)
	}

	// 7. 保存到数据库
	if err := s.appDAO.Create(app); err != nil {
		return nil, fmt.Errorf("保存应用失败: %w", err)
	}

	// 8. 转换为响应
	return s.modelToResponse(app, preparedConfig), nil
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
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}

	// 更新协议配置
	if req.ProtocolConfig != nil {
		updates["protocolConfig"] = req.ProtocolConfig
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
func (s *ApplicationService) modelToResponse(app *model.Application, preparedConfig map[string]interface{}) *dto.ApplicationResponse {
	response := &dto.ApplicationResponse{
		ID:             app.ID,
		Name:           app.Name,
		Description:    app.Description,
		ProtocolType:   app.ProtocolType,
		ProtocolConfig: app.ProtocolConfig,
		IsActive:       app.IsActive,
		IsVerified:     app.IsVerified,
		CreatedAt:      app.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      app.UpdatedAt.Format(time.RFC3339),
	}

	// 如果是创建操作，需要返回明文的ClientSecret（仅OIDC）
	if preparedConfig != nil && app.ProtocolType == string(model.ProtocolTypeOIDC) {
		if clientSecretPlain, ok := preparedConfig["clientSecretPlain"].(string); ok {
			// 创建一个新的配置副本，包含明文密钥
			configCopy := make(map[string]interface{})
			for k, v := range app.ProtocolConfig {
				configCopy[k] = v
			}
			// 替换哈希后的密钥为明文密钥（仅用于响应）
			configCopy["clientSecret"] = clientSecretPlain
			response.ProtocolConfig = configCopy
		}
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
