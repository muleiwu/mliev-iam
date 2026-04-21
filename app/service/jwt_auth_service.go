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
	jwtConfig := jwt.GetConfig()
	jwtService := jwt.NewJWTService(jwtConfig.SecretKey, jwtConfig.Issuer)

	return &JWTAuthService{
		jwtService: jwtService,
		config:     jwtConfig,
	}
}

// GenerateToken 生成JWT token
func (s *JWTAuthService) GenerateToken(user *dto.UserResponse) (string, error) {
	return s.jwtService.GenerateToken(user, s.config.GetAccessTokenHours())
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
