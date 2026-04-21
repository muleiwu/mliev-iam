// Package jwt 提供 mliev-iam 项目内的 JWT 签发/验证能力，是项目唯一仍保留在 internal/pkg 下的自研组件。
//
// 对外 API 划分为两类：
//
//  1. 轻量 JWT 工具（jwt.go）：
//     - JWTService：面向 *dto.UserResponse 的管理员登录令牌签发/校验（HMAC-SHA256）。
//     - Claims：管理员令牌的结构化声明。
//     - NewJWTService(secretKey, issuer) / GenerateToken(user, ttlHours) / ValidateToken(tokenStr) / RefreshToken
//
//  2. OIDC 多密钥管理（key_manager*.go）：
//     - KeyManager：多算法（RSA / HMAC / ECDSA）密钥抽象。
//     - RSAKeyManager / HMACKeyManager 具体实现。
//     - KeyManagerFactory：根据算法字符串创建对应 KeyManager。
//     - 上层由 app/service.ApplicationKeyService 使用，支持按应用维度轮转密钥对，并为 /jwks 端点导出公钥。
//
// 配置入口（config.go）：
//
//	Config 结构体 + GetConfig() 封装 JWT_SECRET_KEY / JWT_ISSUER / JWT_ACCESS_TOKEN_TTL / JWT_REFRESH_TOKEN_TTL 环境变量。
//	本包不依赖项目任何其它模块（app/、internal/helper、internal/interfaces 等都已解耦），
//	所有外部依赖仅限 gorm / gsr / golang-jwt / crypto 等标准/第三方库，便于未来抽取为独立 SDK。
//
// 依赖边界：
//
//   - 允许：gorm.io/gorm、crypto/*、encoding/*、errors、fmt、time、strings、os、strconv、
//     github.com/golang-jwt/jwt/v5
//   - 禁止：任何 cnb.cool/mliev/iam/mliev-iam/app/* 或 cnb.cool/mliev/iam/mliev-iam/internal/*
//     （Config 读取只走 os.Getenv，不经过项目 helper 层）
package jwt
