package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SocialProviderService 社会化登录提供商服务
type SocialProviderService struct {
	db          *gorm.DB
	redis       *redis.Client
	providerDAO *dao.SocialProviderDAO
	accountDAO  *dao.UserSocialAccountDAO
}

// NewSocialProviderService 创建服务实例。
// redisClient 可为 nil；OIDC discovery 缓存会降级为每次重新抓取。
func NewSocialProviderService(db *gorm.DB, redisClient *redis.Client) *SocialProviderService {
	return &SocialProviderService{
		db:          db,
		redis:       redisClient,
		providerDAO: dao.NewSocialProviderDAO(db),
		accountDAO:  dao.NewUserSocialAccountDAO(db),
	}
}

// CreateProvider 创建提供商
func (s *SocialProviderService) CreateProvider(req *dto.CreateSocialProviderRequest) (*dto.SocialProviderResponse, error) {
	// 检查标识符是否已存在
	exists, err := s.providerDAO.ExistsByIdentifier(req.Identifier)
	if err != nil {
		return nil, fmt.Errorf("检查标识符失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("标识符 %s 已存在", req.Identifier)
	}

	// 创建提供商模型
	provider := &model.SocialProvider{
		Name:         req.Name,
		Identifier:   req.Identifier,
		Type:         req.Type,
		Description:  req.Description,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		AuthURL:      req.AuthURL,
		TokenURL:     req.TokenURL,
		UserInfoURL:  req.UserInfoURL,
		RedirectURL:  req.RedirectURL,
		Scopes:       req.Scopes,
		Icon:         req.Icon,
		SortOrder:    req.SortOrder,
		Status:       req.Status,
		IsDefault:    req.IsDefault,
		Attributes:   model.JSONObject(req.Attributes),
		Remark:       req.Remark,
	}

	// 验证配置
	if err := s.validateProviderConfig(provider); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 保存到数据库
	if err := s.providerDAO.Create(provider); err != nil {
		return nil, fmt.Errorf("创建提供商失败: %w", err)
	}

	return s.toProviderResponse(provider), nil
}

// GetProvider 获取提供商详情
func (s *SocialProviderService) GetProvider(id uint) (*dto.SocialProviderResponse, error) {
	provider, err := s.providerDAO.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("提供商不存在")
		}
		return nil, fmt.Errorf("获取提供商失败: %w", err)
	}

	return s.toProviderResponse(provider), nil
}

// GetProviderByIdentifier 根据标识符获取提供商
func (s *SocialProviderService) GetProviderByIdentifier(identifier string) (*model.SocialProvider, error) {
	provider, err := s.providerDAO.GetByIdentifier(identifier)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("提供商不存在")
		}
		return nil, fmt.Errorf("获取提供商失败: %w", err)
	}

	return provider, nil
}

// UpdateProvider 更新提供商
func (s *SocialProviderService) UpdateProvider(id uint, req *dto.UpdateSocialProviderRequest) (*dto.SocialProviderResponse, error) {
	// 获取现有提供商
	provider, err := s.providerDAO.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("提供商不存在")
		}
		return nil, fmt.Errorf("获取提供商失败: %w", err)
	}

	// 更新字段
	if req.Name != nil {
		provider.Name = *req.Name
	}
	if req.Identifier != nil {
		// 检查新标识符是否已存在
		exists, err := s.providerDAO.ExistsByIdentifier(*req.Identifier, id)
		if err != nil {
			return nil, fmt.Errorf("检查标识符失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("标识符 %s 已存在", *req.Identifier)
		}
		provider.Identifier = *req.Identifier
	}
	if req.Type != nil {
		provider.Type = *req.Type
	}
	if req.Description != nil {
		provider.Description = *req.Description
	}
	if req.ClientID != nil {
		provider.ClientID = *req.ClientID
	}
	if req.ClientSecret != nil {
		provider.ClientSecret = *req.ClientSecret
	}
	if req.AuthURL != nil {
		provider.AuthURL = *req.AuthURL
	}
	if req.TokenURL != nil {
		provider.TokenURL = *req.TokenURL
	}
	if req.UserInfoURL != nil {
		provider.UserInfoURL = *req.UserInfoURL
	}
	if req.RedirectURL != nil {
		provider.RedirectURL = *req.RedirectURL
	}
	if req.Scopes != nil {
		provider.Scopes = *req.Scopes
	}
	if req.Icon != nil {
		provider.Icon = *req.Icon
	}
	if req.SortOrder != nil {
		provider.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		provider.Status = *req.Status
	}
	if req.IsDefault != nil {
		provider.IsDefault = *req.IsDefault
	}
	if req.Attributes != nil {
		provider.Attributes = model.JSONObject(req.Attributes)
	}
	if req.Remark != nil {
		provider.Remark = *req.Remark
	}

	// 验证配置
	if err := s.validateProviderConfig(provider); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 保存更新
	if err := s.providerDAO.Update(provider); err != nil {
		return nil, fmt.Errorf("更新提供商失败: %w", err)
	}

	return s.toProviderResponse(provider), nil
}

// DeleteProvider 删除提供商
func (s *SocialProviderService) DeleteProvider(id uint) error {
	// 检查是否有绑定的账号
	count, err := s.accountDAO.CountByProvider(id)
	if err != nil {
		return fmt.Errorf("检查绑定账号失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("该提供商有 %d 个绑定账号，无法删除", count)
	}

	// 删除提供商
	if err := s.providerDAO.Delete(id); err != nil {
		return fmt.Errorf("删除提供商失败: %w", err)
	}

	return nil
}

// GetProviderList 获取提供商列表
func (s *SocialProviderService) GetProviderList(page, pageSize int, status *model.ProviderStatus) ([]*dto.SocialProviderListItem, int64, error) {
	providers, total, err := s.providerDAO.List(page, pageSize, status)
	if err != nil {
		return nil, 0, fmt.Errorf("获取提供商列表失败: %w", err)
	}

	items := make([]*dto.SocialProviderListItem, 0, len(providers))
	for _, provider := range providers {
		items = append(items, &dto.SocialProviderListItem{
			ID:          provider.ID,
			Name:        provider.Name,
			Identifier:  provider.Identifier,
			Type:        provider.Type,
			Icon:        provider.Icon,
			Status:      provider.Status,
			IsDefault:   provider.IsDefault,
			SortOrder:   provider.SortOrder,
			Description: provider.Description,
		})
	}

	return items, total, nil
}

// GetEnabledProviders 获取所有启用的提供商（供登录页按钮列表展示）。
// 所有类型都会返回，前端根据 type 字段决定交互方式——
// 浏览器流（github/google/oidc/qq/wechat）点按钮打开弹窗走 /social/auth；
// 小程序（wechat_miniprogram）点按钮弹扫码 Modal 走 /social/miniprogram/qrcode。
func (s *SocialProviderService) GetEnabledProviders() ([]*dto.PublicProviderInfo, error) {
	providers, err := s.providerDAO.GetEnabledProviders()
	if err != nil {
		return nil, fmt.Errorf("获取启用的提供商失败: %w", err)
	}

	items := make([]*dto.PublicProviderInfo, 0, len(providers))
	for _, provider := range providers {
		items = append(items, &dto.PublicProviderInfo{
			Identifier: provider.Identifier,
			Name:       provider.Name,
			Icon:       provider.Icon,
			SortOrder:  provider.SortOrder,
			IsDefault:  provider.IsDefault,
			Type:       provider.Type,
		})
	}

	return items, nil
}

// BatchOperation 批量操作提供商
func (s *SocialProviderService) BatchOperation(req *dto.SocialProviderBatchRequest) error {
	switch req.Operation {
	case "enable":
		for _, id := range req.IDs {
			provider, err := s.providerDAO.GetByID(id)
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					return fmt.Errorf("提供商 %d 不存在", id)
				}
				return fmt.Errorf("获取提供商 %d 失败: %w", id, err)
			}
			provider.Status = model.ProviderStatusEnabled
			if err := s.validateProviderConfig(provider); err != nil {
				return fmt.Errorf("提供商 %s 配置验证失败: %w", provider.Identifier, err)
			}
		}
		return s.providerDAO.BatchUpdateStatus(req.IDs, model.ProviderStatusEnabled)
	case "disable":
		return s.providerDAO.BatchUpdateStatus(req.IDs, model.ProviderStatusDisabled)
	case "delete":
		// 批量删除前检查是否有绑定账号
		for _, id := range req.IDs {
			count, err := s.accountDAO.CountByProvider(id)
			if err != nil {
				return fmt.Errorf("检查提供商 %d 的绑定账号失败: %w", id, err)
			}
			if count > 0 {
				return fmt.Errorf("提供商 %d 有 %d 个绑定账号，无法删除", id, count)
			}
		}
		return s.providerDAO.BatchDelete(req.IDs)
	default:
		return fmt.Errorf("不支持的操作: %s", req.Operation)
	}
}

// GetProviderStats 获取提供商统计信息
func (s *SocialProviderService) GetProviderStats() (*dto.SocialProviderStatsResponse, error) {
	totalProviders, err := s.providerDAO.Count(nil)
	if err != nil {
		return nil, fmt.Errorf("统计总提供商数失败: %w", err)
	}

	enabledStatus := model.ProviderStatusEnabled
	enabledProviders, err := s.providerDAO.Count(&enabledStatus)
	if err != nil {
		return nil, fmt.Errorf("统计启用提供商数失败: %w", err)
	}

	disabledStatus := model.ProviderStatusDisabled
	disabledProviders, err := s.providerDAO.Count(&disabledStatus)
	if err != nil {
		return nil, fmt.Errorf("统计禁用提供商数失败: %w", err)
	}

	totalBindings, err := s.accountDAO.GetTotalCount()
	if err != nil {
		return nil, fmt.Errorf("统计总绑定数失败: %w", err)
	}

	return &dto.SocialProviderStatsResponse{
		TotalProviders:    totalProviders,
		EnabledProviders:  enabledProviders,
		DisabledProviders: disabledProviders,
		TotalBindings:     totalBindings,
	}, nil
}

// validateProviderConfig 验证提供商配置
func (s *SocialProviderService) validateProviderConfig(provider *model.SocialProvider) error {
	if provider.Status != model.ProviderStatusEnabled {
		return nil
	}

	adapter, err := protocol.NewSocialProviderAdapter(provider, s.redis)
	if err != nil {
		return err
	}
	return adapter.ValidateConfig()
}

// toProviderResponse 转换为响应DTO
func (s *SocialProviderService) toProviderResponse(provider *model.SocialProvider) *dto.SocialProviderResponse {
	var attributes map[string]any
	if provider.Attributes != nil {
		attributes = provider.Attributes
	}

	return &dto.SocialProviderResponse{
		ID:          provider.ID,
		Name:        provider.Name,
		Identifier:  provider.Identifier,
		Type:        provider.Type,
		Description: provider.Description,
		ClientID:    provider.ClientID,
		AuthURL:     provider.AuthURL,
		TokenURL:    provider.TokenURL,
		UserInfoURL: provider.UserInfoURL,
		RedirectURL: provider.RedirectURL,
		Scopes:      provider.Scopes,
		Icon:        provider.Icon,
		SortOrder:   provider.SortOrder,
		Status:      provider.Status,
		IsDefault:   provider.IsDefault,
		Attributes:  attributes,
		Remark:      provider.Remark,
		CreatedAt:   provider.CreatedAt,
		UpdatedAt:   provider.UpdatedAt,
	}
}
