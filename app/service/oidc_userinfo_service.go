package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"github.com/muleiwu/gsr"
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
	return s.BuildUserInfoResponse(user, token.Scopes), nil
}

// BuildUserInfoResponse 构建用户信息响应
func (s *OIDCUserInfoService) BuildUserInfoResponse(user *model.User, scopes []string) *dto.UserInfoResponse {
	// 使用 UUID 作为 sub 和 id 字段
	response := &dto.UserInfoResponse{
		Sub: user.UUID,
		Id:  user.UUID,
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

		// 按配置透出旧系统用户 ID（legacy external_id），供业务系统完成新旧身份映射。
		// 读取 nil map 安全，无需额外判空。
		if exposeLegacyExternalID() {
			if legacyID, ok := user.Attributes["legacy_external_id"].(string); ok && legacyID != "" {
				response.LegacyExternalID = legacyID
			}
		}
	}

	if s.hasScope(scopes, "email") {
		if user.Email != nil {
			response.Email = *user.Email
		}
		// 优先从扩展属性覆盖，否则由 EmailVerifiedAt 派生
		response.EmailVerified = user.EmailVerified()
		if user.Attributes != nil {
			if emailVerified, ok := user.Attributes["email_verified"].(bool); ok {
				response.EmailVerified = emailVerified
			}
		}
	}

	if s.hasScope(scopes, "phone") {
		if user.Phone != nil {
			response.PhoneNumber = *user.Phone
		}
		response.PhoneNumberVerified = user.PhoneVerified()
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

// exposeLegacyExternalID 是否在 userinfo 的 profile scope 中透出 legacy_external_id（默认 false）。
// 用 container.Get（而非 helper.GetEnv 的 MustGet）读取，env 未注册时安全回落为默认值，
// 便于直接构造 BuildUserInfoResponse 的单元测试运行。
func exposeLegacyExternalID() bool {
	env, err := container.Get[gsr.Enver]()
	if err != nil || env == nil {
		return false
	}
	return env.GetBool("oidc.expose_legacy_external_id", false)
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
//
// 该响应需与服务端实际能力严格一致：
//   - response_types_supported 只声明已实现的单值类型（hybrid flow 尚未实现）
//   - grant_types_supported 显式声明，避免依赖 OIDC Discovery 1.0 默认值
//     (默认为 ["authorization_code", "implicit"]，会遗漏本服务支持的
//     refresh_token / client_credentials)
//   - code_challenge_methods_supported 跟随 oidc.pkce.allow_plain 动态生成，
//     plain 默认禁用时只声明 S256
func (s *OIDCDiscoveryService) GetDiscoveryDocument() *dto.OIDCDiscoveryResponse {
	codeChallengeMethods := []string{"S256"}
	if pkcePlainAllowed() {
		codeChallengeMethods = append(codeChallengeMethods, "plain")
	}

	return &dto.OIDCDiscoveryResponse{
		Issuer:                            s.baseURL,
		AuthorizationEndpoint:             fmt.Sprintf("%s/authorize", s.baseURL),
		TokenEndpoint:                     fmt.Sprintf("%s/token", s.baseURL),
		UserInfoEndpoint:                  fmt.Sprintf("%s/userinfo", s.baseURL),
		JwksURI:                           fmt.Sprintf("%s/jwks", s.baseURL),
		RevocationEndpoint:                fmt.Sprintf("%s/revoke", s.baseURL),
		IntrospectionEndpoint:             fmt.Sprintf("%s/introspect", s.baseURL),
		ResponseTypesSupported:            []string{"code", "token", "id_token"},
		ResponseModesSupported:            []string{"query", "fragment"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token", "client_credentials", "implicit"},
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
		CodeChallengeMethodsSupported: codeChallengeMethods,
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
