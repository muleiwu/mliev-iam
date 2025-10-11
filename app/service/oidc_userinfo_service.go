package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// OIDCUserInfoService OIDC用户信息服务
type OIDCUserInfoService struct {
	tokenDAO    *dao.OIDCTokenDAO
	userService *UserService
	db          *gorm.DB
}

// NewOIDCUserInfoService 创建新的OIDC用户信息服务
func NewOIDCUserInfoService(db *gorm.DB) *OIDCUserInfoService {
	return &OIDCUserInfoService{
		tokenDAO:    dao.NewOIDCTokenDAO(db),
		userService: NewUserService(db),
		db:          db,
	}
}

// GetUserInfo 获取用户信息
func (s *OIDCUserInfoService) GetUserInfo(accessToken string) (*dto.UserInfoResponse, error) {
	// 1. 验证访问令牌
	token, err := s.tokenDAO.GetValidByAccessToken(accessToken)
	if err != nil {
		return nil, errors.New("invalid_token")
	}

	// 2. 检查作用域
	if !s.hasScope(token.Scopes, "openid") {
		return nil, errors.New("insufficient_scope")
	}

	// 3. 获取用户信息
	user, err := s.userService.GetUserModelByID(token.UserID)
	if err != nil {
		return nil, errors.New("user_not_found")
	}

	// 4. 构建用户信息响应
	return s.buildUserInfoResponse(user, token.Scopes), nil
}

// buildUserInfoResponse 构建用户信息响应
func (s *OIDCUserInfoService) buildUserInfoResponse(user *model.User, scopes []string) *dto.UserInfoResponse {
	response := &dto.UserInfoResponse{
		Sub: strconv.FormatUint(uint64(user.ID), 10),
	}

	// 根据作用域返回相应的信息
	if s.hasScope(scopes, "profile") {
		response.Name = user.RealName
		response.Nickname = user.Nickname
		response.PreferredUsername = user.Username
		response.Picture = user.Avatar
		response.UpdatedAt = user.UpdatedAt.Unix()

		// 从扩展属性中获取其他profile信息
		if user.Attributes != nil {
			if givenName, ok := user.Attributes["given_name"].(string); ok {
				response.GivenName = givenName
			}
			if familyName, ok := user.Attributes["family_name"].(string); ok {
				response.FamilyName = familyName
			}
			if middleName, ok := user.Attributes["middle_name"].(string); ok {
				response.MiddleName = middleName
			}
			if website, ok := user.Attributes["website"].(string); ok {
				response.Website = website
			}
			if gender, ok := user.Attributes["gender"].(string); ok {
				response.Gender = gender
			}
			if birthdate, ok := user.Attributes["birthdate"].(string); ok {
				response.Birthdate = birthdate
			}
			if zoneinfo, ok := user.Attributes["zoneinfo"].(string); ok {
				response.Zoneinfo = zoneinfo
			}
			if locale, ok := user.Attributes["locale"].(string); ok {
				response.Locale = locale
			}
		}
	}

	if s.hasScope(scopes, "email") {
		response.Email = user.Email
		response.EmailVerified = user.IsVerified
	}

	if s.hasScope(scopes, "phone") {
		response.PhoneNumber = user.Phone
		// 可以添加phone_number_verified字段
		if user.Attributes != nil {
			if phoneVerified, ok := user.Attributes["phone_number_verified"].(bool); ok {
				response.PhoneNumberVerified = phoneVerified
			}
		}
	}

	if s.hasScope(scopes, "address") {
		// 从扩展属性中获取地址信息
		if user.Attributes != nil {
			if addressData, ok := user.Attributes["address"]; ok {
				if addressBytes, err := json.Marshal(addressData); err == nil {
					var address dto.Address
					if json.Unmarshal(addressBytes, &address) == nil {
						response.Address = &address
					}
				}
			}
		}
	}

	return response
}

// hasScope 检查是否有指定作用域
func (s *OIDCUserInfoService) hasScope(scopes []string, targetScope string) bool {
	for _, scope := range scopes {
		if scope == targetScope {
			return true
		}
	}
	return false
}

// OIDCDiscoveryService OIDC发现服务
type OIDCDiscoveryService struct {
	baseURL string
}

// NewOIDCDiscoveryService 创建新的OIDC发现服务
func NewOIDCDiscoveryService(baseURL string) *OIDCDiscoveryService {
	return &OIDCDiscoveryService{
		baseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

// GetDiscoveryDocument 获取OIDC发现文档
func (s *OIDCDiscoveryService) GetDiscoveryDocument() *dto.OIDCDiscoveryResponse {
	return &dto.OIDCDiscoveryResponse{
		Issuer:                            s.baseURL,
		AuthorizationEndpoint:             fmt.Sprintf("%s/oidc/authorize", s.baseURL),
		TokenEndpoint:                     fmt.Sprintf("%s/oidc/token", s.baseURL),
		UserInfoEndpoint:                  fmt.Sprintf("%s/oidc/userinfo", s.baseURL),
		JwksURI:                           fmt.Sprintf("%s/oidc/jwks", s.baseURL),
		RevocationEndpoint:                fmt.Sprintf("%s/oidc/revoke", s.baseURL),
		IntrospectionEndpoint:             fmt.Sprintf("%s/oidc/introspect", s.baseURL),
		ResponseTypesSupported:            []string{"code", "token", "id_token", "code token", "code id_token", "token id_token", "code token id_token"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256", "HS256"},
		ScopesSupported:                   []string{"openid", "profile", "email", "phone", "address", "offline_access"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "none"},
		ClaimsSupported: []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce",
			"name", "given_name", "family_name", "middle_name", "nickname",
			"preferred_username", "profile", "picture", "website", "gender",
			"birthdate", "zoneinfo", "locale", "updated_at",
			"email", "email_verified",
			"phone_number", "phone_number_verified",
			"address",
		},
		CodeChallengeMethodsSupported: []string{"plain", "S256"},
	}
}

// OIDCManagementService OIDC管理服务（已废弃，使用ApplicationService代替）
type OIDCManagementService struct {
	appService  *ApplicationService
	tokenDAO    *dao.OIDCTokenDAO
	authCodeDAO *dao.AuthorizationCodeDAO
	db          *gorm.DB
}

// NewOIDCManagementService 创建新的OIDC管理服务
func NewOIDCManagementService(db *gorm.DB) *OIDCManagementService {
	return &OIDCManagementService{
		appService:  NewApplicationService(db),
		tokenDAO:    dao.NewOIDCTokenDAO(db),
		authCodeDAO: dao.NewAuthorizationCodeDAO(db),
		db:          db,
	}
}

// GetStats 获取OIDC统计信息
func (s *OIDCManagementService) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 使用ApplicationService获取统计
	appStats, err := s.appService.GetStats()
	if err != nil {
		return nil, err
	}

	stats["active_clients"] = appStats.ActiveApplications
	stats["inactive_clients"] = appStats.InactiveApplications
	stats["active_tokens"] = appStats.ActiveTokens

	return stats, nil
}

// CleanupExpiredData 清理过期数据
func (s *OIDCManagementService) CleanupExpiredData() error {
	// 清理过期的授权码
	if err := s.authCodeDAO.DeleteExpired(); err != nil {
		return fmt.Errorf("清理过期授权码失败: %w", err)
	}

	// 清理过期的令牌
	if err := s.tokenDAO.DeleteExpired(); err != nil {
		return fmt.Errorf("清理过期令牌失败: %w", err)
	}

	return nil
}

// 辅助函数
func boolPtr(b bool) *bool {
	return &b
}
