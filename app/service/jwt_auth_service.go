package service

import (
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"gorm.io/gorm"
)

// JWTAuthService JWT认证服务
type JWTAuthService struct {
	jwtService *jwt.JWTService
	config     *jwt.Config
}

// NewJWTAuthService 创建JWT认证服务实例
func NewJWTAuthService() *JWTAuthService {
	jwtAuthService, err := NewJWTAuthServiceWithError()
	if err != nil {
		panic(err)
	}
	return jwtAuthService
}

// NewJWTAuthServiceWithError 创建 JWT 认证服务实例，并将配置错误显式返回给调用方。
func NewJWTAuthServiceWithError() (*JWTAuthService, error) {
	jwtConfig, err := jwt.LoadConfig()
	if err != nil {
		return nil, err
	}
	jwtService := jwt.NewJWTService(jwtConfig.SecretKey, jwtConfig.Issuer)

	return &JWTAuthService{
		jwtService: jwtService,
		config:     jwtConfig,
	}, nil
}

func newOptionalJWTAuthService() *JWTAuthService {
	jwtAuthService, err := NewJWTAuthServiceWithError()
	if err != nil {
		return nil
	}
	return jwtAuthService
}

func ensureJWTAuthService(jwtAuthService *JWTAuthService) (*JWTAuthService, error) {
	if jwtAuthService != nil {
		return jwtAuthService, nil
	}
	return NewJWTAuthServiceWithError()
}

// GenerateAdminToken 为管理员会话签发内部 JWT，role 固定为 "admin"。
// 仅允许在已校验用户具备管理员身份后调用（见 admin AuthController.Callback）。
func (s *JWTAuthService) GenerateAdminToken(user *dto.UserResponse, sessionIDs ...string) (string, error) {
	if len(sessionIDs) != 1 || sessionIDs[0] == "" {
		return "", fmt.Errorf("缺少登录会话")
	}
	return s.generateSessionToken(user, sessionIDs[0], "admin")
}

// GenerateUserToken 为普通用户会话（个人中心）签发内部 JWT，role 固定为 "user"。
// 显式绑定普通用户角色与持久化会话，禁止签发无会话标识的凭据。
func (s *JWTAuthService) GenerateUserToken(user *dto.UserResponse, sessionIDs ...string) (string, error) {
	if len(sessionIDs) != 1 || sessionIDs[0] == "" {
		return "", fmt.Errorf("缺少登录会话")
	}
	return s.generateSessionToken(user, sessionIDs[0], "user")
}

// ValidateToken 验证JWT token
func (s *JWTAuthService) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return s.jwtService.ValidateToken(tokenString)
}

// RefreshToken 刷新JWT token
func (s *JWTAuthService) RefreshToken(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}
	var token string
	err = helper.GetDatabase().Transaction(func(tx *gorm.DB) error {
		if err := dao.NewLoginSessionDAO(tx).Lock(claims.SessionID, claims.UserID); err != nil {
			return err
		}
		token, err = s.jwtService.RefreshToken(tokenString, s.config.GetAccessTokenHours())
		if err != nil {
			return err
		}
		return dao.NewLoginSessionDAO(tx).Extend(claims.SessionID, time.Now().Add(s.config.AccessTokenTTL))
	})
	return token, err
}

// GetJWTService 获取JWT服务实例（如果需要更细粒度的控制）
func (s *JWTAuthService) GetJWTService() *jwt.JWTService {
	return s.jwtService
}

// GetConfig 获取JWT配置
func (s *JWTAuthService) GetConfig() *jwt.Config {
	return s.config
}

// Signing and extending the login share the revocation lock. A signed token is
// returned only when the persistent login remains valid and the write commits.
func (s *JWTAuthService) generateSessionToken(user *dto.UserResponse, sid, role string) (string, error) {
	if user == nil {
		return "", fmt.Errorf("用户为空")
	}
	var token string
	err := helper.GetDatabase().Transaction(func(tx *gorm.DB) error {
		if err := dao.NewLoginSessionDAO(tx).Lock(sid, user.ID); err != nil {
			return err
		}
		var err error
		token, err = s.jwtService.GenerateToken(jwt.TokenSubject{UserID: user.ID, Username: user.Username, Role: role, SessionID: sid}, s.config.GetAccessTokenHours())
		if err != nil {
			return err
		}
		claims, err := s.jwtService.ValidateToken(token)
		if err != nil {
			return err
		}
		return dao.NewLoginSessionDAO(tx).Extend(sid, claims.ExpiresAt.Time)
	})
	if err != nil {
		return "", err
	}
	return token, nil
}
