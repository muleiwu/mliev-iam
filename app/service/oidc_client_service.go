package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/examples/go-web/app/dao"
	"cnb.cool/mliev/examples/go-web/app/dto"
	"cnb.cool/mliev/examples/go-web/app/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OIDCClientService OIDC客户端服务
type OIDCClientService struct {
	clientDAO *dao.OIDCClientDAO
	db        *gorm.DB
}

// NewOIDCClientService 创建新的OIDC客户端服务
func NewOIDCClientService(db *gorm.DB) *OIDCClientService {
	return &OIDCClientService{
		clientDAO: dao.NewOIDCClientDAO(db),
		db:        db,
	}
}

// CreateClient 创建OIDC客户端
func (s *OIDCClientService) CreateClient(req *dto.CreateOIDCClientRequest, creatorID uint) (*dto.OIDCClientWithSecretResponse, error) {
	// 生成客户端ID和密钥
	clientID := s.generateClientID()
	clientSecret := s.generateClientSecret()

	// 验证请求数据
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	client := &model.OIDCClient{
		ClientID:        clientID,
		ClientSecret:    s.hashClientSecret(clientSecret),
		Name:            req.Name,
		Description:     req.Description,
		RedirectURIs:    req.RedirectURIs,
		GrantTypes:      req.GrantTypes,
		Scopes:          req.Scopes,
		ClientType:      req.ClientType,
		RequirePKCE:     req.RequirePKCE,
		CreatedByUserID: &creatorID,
		IsActive:        true,
		IsVerified:      false,
	}

	if err := s.clientDAO.Create(client); err != nil {
		return nil, fmt.Errorf("创建OIDC客户端失败: %w", err)
	}

	return &dto.OIDCClientWithSecretResponse{
		OIDCClientResponse: dto.OIDCClientResponse{
			ID:           client.ID,
			ClientID:     client.ClientID,
			Name:         client.Name,
			Description:  client.Description,
			RedirectURIs: client.RedirectURIs,
			GrantTypes:   client.GrantTypes,
			Scopes:       client.Scopes,
			ClientType:   client.ClientType,
			RequirePKCE:  client.RequirePKCE,
			IsActive:     client.IsActive,
			IsVerified:   client.IsVerified,
			CreatedAt:    client.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    client.UpdatedAt.Format(time.RFC3339),
		},
		ClientSecret: clientSecret,
	}, nil
}

// GetClient 获取OIDC客户端
func (s *OIDCClientService) GetClient(id uint) (*dto.OIDCClientResponse, error) {
	client, err := s.clientDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("OIDC客户端不存在")
		}
		return nil, fmt.Errorf("获取OIDC客户端失败: %w", err)
	}

	return s.modelToResponse(client), nil
}

// GetClientByClientID 根据ClientID获取客户端
func (s *OIDCClientService) GetClientByClientID(clientID string) (*model.OIDCClient, error) {
	client, err := s.clientDAO.GetByClientID(clientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("OIDC客户端不存在")
		}
		return nil, fmt.Errorf("获取OIDC客户端失败: %w", err)
	}

	return client, nil
}

// UpdateClient 更新OIDC客户端
func (s *OIDCClientService) UpdateClient(id uint, req *dto.UpdateOIDCClientRequest) (*dto.OIDCClientResponse, error) {
	client, err := s.clientDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("OIDC客户端不存在")
		}
		return nil, fmt.Errorf("获取OIDC客户端失败: %w", err)
	}

	// 更新字段
	if req.Name != nil {
		client.Name = *req.Name
	}
	if req.Description != nil {
		client.Description = *req.Description
	}
	if req.RedirectURIs != nil {
		client.RedirectURIs = req.RedirectURIs
	}
	if req.GrantTypes != nil {
		client.GrantTypes = req.GrantTypes
	}
	if req.Scopes != nil {
		client.Scopes = req.Scopes
	}
	if req.ClientType != nil {
		client.ClientType = *req.ClientType
	}
	if req.RequirePKCE != nil {
		client.RequirePKCE = *req.RequirePKCE
	}
	if req.IsActive != nil {
		client.IsActive = *req.IsActive
	}

	if err := s.clientDAO.Update(client); err != nil {
		return nil, fmt.Errorf("更新OIDC客户端失败: %w", err)
	}

	return s.modelToResponse(client), nil
}

// DeleteClient 删除OIDC客户端
func (s *OIDCClientService) DeleteClient(id uint) error {
	// 检查客户端是否存在
	_, err := s.clientDAO.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("OIDC客户端不存在")
		}
		return fmt.Errorf("获取OIDC客户端失败: %w", err)
	}

	// TODO: 在删除客户端前，需要撤销所有相关的令牌和授权码

	if err := s.clientDAO.Delete(id); err != nil {
		return fmt.Errorf("删除OIDC客户端失败: %w", err)
	}

	return nil
}

// ListClients 获取OIDC客户端列表
func (s *OIDCClientService) ListClients(offset, limit int, isActive *bool) ([]*dto.OIDCClientResponse, int64, error) {
	clients, total, err := s.clientDAO.List(offset, limit, isActive)
	if err != nil {
		return nil, 0, fmt.Errorf("获取OIDC客户端列表失败: %w", err)
	}

	responses := make([]*dto.OIDCClientResponse, len(clients))
	for i, client := range clients {
		responses[i] = s.modelToResponse(client)
	}

	return responses, total, nil
}

// ValidateClient 验证客户端
func (s *OIDCClientService) ValidateClient(clientID, clientSecret string) (*model.OIDCClient, error) {
	client, err := s.GetClientByClientID(clientID)
	if err != nil {
		return nil, err
	}

	if !client.IsActive {
		return nil, errors.New("客户端已被禁用")
	}

	// 对于公开客户端，不需要验证客户端密钥
	if client.IsPublicClient() {
		return client, nil
	}

	// 对于机密客户端，需要验证客户端密钥
	if clientSecret == "" {
		return nil, errors.New("机密客户端需要提供客户端密钥")
	}

	if !s.verifyClientSecret(clientSecret, client.ClientSecret) {
		return nil, errors.New("客户端密钥错误")
	}

	return client, nil
}

// ValidateRedirectURI 验证重定向URI
func (s *OIDCClientService) ValidateRedirectURI(client *model.OIDCClient, redirectURI string) error {
	if !client.IsRedirectURIValid(redirectURI) {
		return errors.New("无效的重定向URI")
	}
	return nil
}

// ValidateGrantType 验证授权类型
func (s *OIDCClientService) ValidateGrantType(client *model.OIDCClient, grantType model.GrantType) error {
	if !client.HasGrantType(grantType) {
		return fmt.Errorf("客户端不支持授权类型: %s", grantType)
	}
	return nil
}

// ValidateResponseType 验证响应类型
func (s *OIDCClientService) ValidateResponseType(client *model.OIDCClient, responseType model.ResponseType) error {
	if !client.HasResponseType(responseType) {
		return fmt.Errorf("客户端不支持响应类型: %s", responseType)
	}
	return nil
}

// ValidateScopes 验证作用域
func (s *OIDCClientService) ValidateScopes(client *model.OIDCClient, requestedScopes []string) error {
	for _, scope := range requestedScopes {
		if !client.HasScope(scope) {
			return fmt.Errorf("客户端不支持作用域: %s", scope)
		}
	}
	return nil
}

// 私有方法

// generateClientID 生成客户端ID
func (s *OIDCClientService) generateClientID() string {
	return uuid.New().String()
}

// generateClientSecret 生成客户端密钥
func (s *OIDCClientService) generateClientSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

// hashClientSecret 哈希客户端密钥
func (s *OIDCClientService) hashClientSecret(secret string) string {
	hash := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}

// verifyClientSecret 验证客户端密钥
func (s *OIDCClientService) verifyClientSecret(secret, hashedSecret string) bool {
	hash := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:]) == hashedSecret
}

// validateCreateRequest 验证创建请求
func (s *OIDCClientService) validateCreateRequest(req *dto.CreateOIDCClientRequest) error {
	if len(req.RedirectURIs) == 0 {
		return errors.New("至少需要一个重定向URI")
	}

	if len(req.GrantTypes) == 0 {
		return errors.New("至少需要一个授权类型")
	}

	if len(req.Scopes) == 0 {
		return errors.New("至少需要一个作用域")
	}

	// 验证作用域必须包含openid
	hasOpenID := false
	for _, scope := range req.Scopes {
		if scope == "openid" {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		return errors.New("OIDC客户端的作用域必须包含openid")
	}

	return nil
}

// modelToResponse 将模型转换为响应
func (s *OIDCClientService) modelToResponse(client *model.OIDCClient) *dto.OIDCClientResponse {
	return &dto.OIDCClientResponse{
		ID:           client.ID,
		ClientID:     client.ClientID,
		Name:         client.Name,
		Description:  client.Description,
		RedirectURIs: client.RedirectURIs,
		GrantTypes:   client.GrantTypes,
		Scopes:       client.Scopes,
		ClientType:   client.ClientType,
		RequirePKCE:  client.RequirePKCE,
		IsActive:     client.IsActive,
		IsVerified:   client.IsVerified,
		CreatedAt:    client.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    client.UpdatedAt.Format(time.RFC3339),
	}
}
