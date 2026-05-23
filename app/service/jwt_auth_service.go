package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
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

// GenerateToken 生成JWT token
func (s *JWTAuthService) GenerateToken(user *dto.UserResponse) (string, error) {
	return s.jwtService.GenerateToken(jwt.TokenSubject{
		UserID:   user.ID,
		Username: user.Username,
	}, s.config.GetAccessTokenHours())
}

// ValidateToken 验证JWT token
func (s *JWTAuthService) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return s.jwtService.ValidateToken(tokenString)
}

// RefreshToken 刷新JWT token
func (s *JWTAuthService) RefreshToken(tokenString string) (string, error) {
	return s.jwtService.RefreshToken(tokenString, s.config.GetAccessTokenHours())
}

// GetJWTService 获取JWT服务实例（如果需要更细粒度的控制）
func (s *JWTAuthService) GetJWTService() *jwt.JWTService {
	return s.jwtService
}

// GetConfig 获取JWT配置
func (s *JWTAuthService) GetConfig() *jwt.Config {
	return s.config
}
