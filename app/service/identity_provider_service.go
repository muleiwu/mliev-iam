package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"gorm.io/gorm"
)

type IdentityProviderService struct {
	db          *gorm.DB
	providerDAO *dao.IdentityProviderDAO
	accountDAO  *dao.UserIdentityAccountDAO
}

func NewIdentityProviderService(db *gorm.DB) *IdentityProviderService {
	return &IdentityProviderService{
		db:          db,
		providerDAO: dao.NewIdentityProviderDAO(db),
		accountDAO:  dao.NewUserIdentityAccountDAO(db),
	}
}

func (s *IdentityProviderService) CreateProvider(req *dto.CreateIdentityProviderRequest) (*dto.IdentityProviderResponse, error) {
	exists, err := s.providerDAO.ExistsByIdentifier(req.Identifier)
	if err != nil {
		return nil, fmt.Errorf("检查标识符失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("标识符 %s 已存在", req.Identifier)
	}

	provider := &model.IdentityProvider{
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

	if err := s.validateProviderConfig(provider); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}
	if err := s.providerDAO.Create(provider); err != nil {
		return nil, fmt.Errorf("创建身份源失败: %w", err)
	}
	return s.toProviderResponse(provider), nil
}

func (s *IdentityProviderService) GetProvider(id uint) (*dto.IdentityProviderResponse, error) {
	provider, err := s.providerDAO.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("身份源不存在")
		}
		return nil, fmt.Errorf("获取身份源失败: %w", err)
	}
	return s.toProviderResponse(provider), nil
}

func (s *IdentityProviderService) GetProviderByIdentifier(identifier string) (*model.IdentityProvider, error) {
	provider, err := s.providerDAO.GetByIdentifier(identifier)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("身份源不存在")
		}
		return nil, fmt.Errorf("获取身份源失败: %w", err)
	}
	return provider, nil
}

func (s *IdentityProviderService) UpdateProvider(id uint, req *dto.UpdateIdentityProviderRequest) (*dto.IdentityProviderResponse, error) {
	provider, err := s.providerDAO.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("身份源不存在")
		}
		return nil, fmt.Errorf("获取身份源失败: %w", err)
	}

	if req.Name != nil {
		provider.Name = *req.Name
	}
	if req.Identifier != nil {
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

	if err := s.validateProviderConfig(provider); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}
	if err := s.providerDAO.Update(provider); err != nil {
		return nil, fmt.Errorf("更新身份源失败: %w", err)
	}
	return s.toProviderResponse(provider), nil
}

func (s *IdentityProviderService) DeleteProvider(id uint) error {
	count, err := s.accountDAO.CountByProvider(id)
	if err != nil {
		return fmt.Errorf("检查绑定账号失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("该身份源有 %d 个绑定账号，无法删除", count)
	}
	if err := s.providerDAO.Delete(id); err != nil {
		return fmt.Errorf("删除身份源失败: %w", err)
	}
	return nil
}

func (s *IdentityProviderService) GetProviderList(page, pageSize int, status *model.IdentityProviderStatus) ([]*dto.IdentityProviderListItem, int64, error) {
	providers, total, err := s.providerDAO.List(page, pageSize, status)
	if err != nil {
		return nil, 0, fmt.Errorf("获取身份源列表失败: %w", err)
	}
	items := make([]*dto.IdentityProviderListItem, 0, len(providers))
	for _, provider := range providers {
		items = append(items, &dto.IdentityProviderListItem{
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

func (s *IdentityProviderService) GetEnabledProviders() ([]*dto.PublicIdentityProviderInfo, error) {
	providers, err := s.providerDAO.GetEnabledProviders()
	if err != nil {
		return nil, fmt.Errorf("获取启用身份源失败: %w", err)
	}
	items := make([]*dto.PublicIdentityProviderInfo, 0, len(providers))
	for _, provider := range providers {
		items = append(items, &dto.PublicIdentityProviderInfo{
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

func (s *IdentityProviderService) BatchOperation(req *dto.IdentityProviderBatchRequest) error {
	switch req.Operation {
	case "enable":
		return s.providerDAO.BatchUpdateStatus(req.IDs, model.IdentityProviderStatusEnabled)
	case "disable":
		return s.providerDAO.BatchUpdateStatus(req.IDs, model.IdentityProviderStatusDisabled)
	case "delete":
		for _, id := range req.IDs {
			count, err := s.accountDAO.CountByProvider(id)
			if err != nil {
				return fmt.Errorf("检查身份源 %d 的绑定账号失败: %w", id, err)
			}
			if count > 0 {
				return fmt.Errorf("身份源 %d 有 %d 个绑定账号，无法删除", id, count)
			}
		}
		return s.providerDAO.BatchDelete(req.IDs)
	default:
		return fmt.Errorf("不支持的操作: %s", req.Operation)
	}
}

func (s *IdentityProviderService) GetProviderStats() (*dto.IdentityProviderStatsResponse, error) {
	totalProviders, err := s.providerDAO.Count(nil)
	if err != nil {
		return nil, fmt.Errorf("统计总身份源数失败: %w", err)
	}
	enabledStatus := model.IdentityProviderStatusEnabled
	enabledProviders, err := s.providerDAO.Count(&enabledStatus)
	if err != nil {
		return nil, fmt.Errorf("统计启用身份源数失败: %w", err)
	}
	disabledStatus := model.IdentityProviderStatusDisabled
	disabledProviders, err := s.providerDAO.Count(&disabledStatus)
	if err != nil {
		return nil, fmt.Errorf("统计禁用身份源数失败: %w", err)
	}
	totalBindings, err := s.accountDAO.GetTotalCount()
	if err != nil {
		return nil, fmt.Errorf("统计总绑定数失败: %w", err)
	}
	return &dto.IdentityProviderStatsResponse{
		TotalProviders:    totalProviders,
		EnabledProviders:  enabledProviders,
		DisabledProviders: disabledProviders,
		TotalBindings:     totalBindings,
	}, nil
}

func (s *IdentityProviderService) validateProviderConfig(provider *model.IdentityProvider) error {
	adapter, err := protocol.NewIdentityProviderAdapter(provider)
	if err != nil {
		return err
	}
	return adapter.ValidateConfig()
}

func (s *IdentityProviderService) toProviderResponse(provider *model.IdentityProvider) *dto.IdentityProviderResponse {
	var attributes map[string]any
	if provider.Attributes != nil {
		attributes = provider.Attributes
	}
	return &dto.IdentityProviderResponse{
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
