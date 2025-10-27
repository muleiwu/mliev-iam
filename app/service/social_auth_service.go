package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// SocialAuthService 社会化登录认证服务
type SocialAuthService struct {
	db                *gorm.DB
	redis             *redis.Client
	providerDAO       *dao.SocialProviderDAO
	accountDAO        *dao.UserSocialAccountDAO
	userDAO           *dao.UserDao
	providerService   *SocialProviderService
	userService       *UserService
	jwtAuthService    *JWTAuthService
	stateKeyPrefix    string
	tempUserKeyPrefix string
}

// TempSocialUserInfo 临时社交用户信息（存储在Redis）
type TempSocialUserInfo struct {
	ProviderID  uint                   `json:"providerId"`
	OpenID      string                 `json:"openId"`
	UnionID     string                 `json:"unionId"`
	Nickname    string                 `json:"nickname"`
	Avatar      string                 `json:"avatar"`
	Email       string                 `json:"email"`
	RawUserInfo map[string]interface{} `json:"rawUserInfo"`
	AccessToken string                 `json:"accessToken"`
	TokenType   string                 `json:"tokenType"`
	ExpiresIn   int64                  `json:"expiresIn"`
}

// NewSocialAuthService 创建服务实例
func NewSocialAuthService(db *gorm.DB, redisClient *redis.Client) *SocialAuthService {
	return &SocialAuthService{
		db:                db,
		redis:             redisClient,
		providerDAO:       dao.NewSocialProviderDAO(db),
		accountDAO:        dao.NewUserSocialAccountDAO(db),
		userDAO:           dao.NewUserDao(db),
		providerService:   NewSocialProviderService(db),
		userService:       NewUserService(db),
		jwtAuthService:    NewJWTAuthService(),
		stateKeyPrefix:    "social:state:",
		tempUserKeyPrefix: "social:temp_user:",
	}
}

// GetAuthURL 获取OAuth授权URL
func (s *SocialAuthService) GetAuthURL(providerIdentifier string) (*dto.SocialAuthURLResponse, error) {
	// 获取提供商配置
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}

	// 检查提供商是否启用
	if !provider.IsEnabled() {
		return nil, fmt.Errorf("提供商未启用")
	}

	// 创建适配器
	adapter, err := protocol.NewSocialProviderAdapter(provider)
	if err != nil {
		return nil, fmt.Errorf("创建适配器失败: %w", err)
	}

	// 生成state参数（防CSRF）
	state, err := s.generateState()
	if err != nil {
		return nil, fmt.Errorf("生成state失败: %w", err)
	}

	// 存储state（5分钟有效期）
	if err := s.storeState(state, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("存储state失败: %w", err)
	}

	// 获取授权URL
	authURL := adapter.GetAuthURL(state)

	return &dto.SocialAuthURLResponse{
		AuthURL: authURL,
		State:   state,
	}, nil
}

// HandleCallback 处理OAuth回调
func (s *SocialAuthService) HandleCallback(providerIdentifier, code, state string) (*dto.SocialAuthCallbackResponse, error) {
	// 验证state
	if !s.validateState(state) {
		return nil, fmt.Errorf("无效的state参数")
	}

	// 获取提供商配置
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}

	// 创建适配器
	adapter, err := protocol.NewSocialProviderAdapter(provider)
	if err != nil {
		return nil, fmt.Errorf("创建适配器失败: %w", err)
	}

	// 交换授权码获取令牌
	ctx := context.Background()
	token, err := adapter.ExchangeToken(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("交换令牌失败: %w", err)
	}

	// 获取用户信息
	socialUser, err := adapter.GetUserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 调试日志
	fmt.Printf("DEBUG: socialUser.OpenID = %s\n", socialUser.OpenID)

	// 查找是否已绑定账号
	account, err := s.accountDAO.GetByProviderAndOpenID(provider.ID, socialUser.OpenID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询绑定账号失败: %w", err)
	}

	if account != nil {
		// 已绑定，直接登录
		fmt.Printf("DEBUG: 已有账号，执行登录流程\n")
		return s.loginWithSocialAccount(account, socialUser, token)
	}

	// 未绑定，返回需要绑定的响应
	fmt.Printf("DEBUG: 新用户，需要选择绑定或注册\n")

	// 生成绑定令牌
	bindToken := uuid.New().String()

	// 提取 OAuth token 信息
	var accessToken, tokenType string
	var expiresIn int64
	accessToken = token.AccessToken
	tokenType = token.TokenType
	expiresIn = int64(time.Until(token.Expiry).Seconds())

	// 存储临时社交用户信息到 Redis
	tempUserInfo := &TempSocialUserInfo{
		ProviderID:  provider.ID,
		OpenID:      socialUser.OpenID,
		UnionID:     socialUser.UnionID,
		Nickname:    socialUser.Nickname,
		Avatar:      socialUser.Avatar,
		Email:       socialUser.Email,
		RawUserInfo: socialUser.RawUserInfo,
		AccessToken: accessToken,
		TokenType:   tokenType,
		ExpiresIn:   expiresIn,
	}

	if err := s.storeSocialUserTemp(bindToken, tempUserInfo); err != nil {
		return nil, fmt.Errorf("存储临时用户信息失败: %w", err)
	}

	fmt.Printf("DEBUG: 生成 BindToken: %s\n", bindToken)

	// 返回需要绑定的响应
	return &dto.SocialAuthCallbackResponse{
		NeedsBind: true,
		BindToken: bindToken,
	}, nil
}

// BindSocialAccount 绑定社交账号（已登录用户）
func (s *SocialAuthService) BindSocialAccount(userID uint, providerIdentifier, code, state string) error {
	// 验证state
	if !s.validateState(state) {
		return fmt.Errorf("无效的state参数")
	}

	// 获取提供商配置
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return err
	}

	// 创建适配器
	adapter, err := protocol.NewSocialProviderAdapter(provider)
	if err != nil {
		return fmt.Errorf("创建适配器失败: %w", err)
	}

	// 交换授权码获取令牌
	ctx := context.Background()
	token, err := adapter.ExchangeToken(ctx, code)
	if err != nil {
		return fmt.Errorf("交换令牌失败: %w", err)
	}

	// 获取用户信息
	socialUser, err := adapter.GetUserInfo(ctx, token)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 检查该社交账号是否已被其他用户绑定
	exists, err := s.accountDAO.ExistsByProviderAndOpenID(provider.ID, socialUser.OpenID, userID)
	if err != nil {
		return fmt.Errorf("检查绑定状态失败: %w", err)
	}
	if exists {
		return fmt.Errorf("该社交账号已被其他用户绑定")
	}

	// 检查用户是否已绑定该提供商
	existingAccount, err := s.accountDAO.GetByUserAndProvider(userID, provider.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("查询已有绑定失败: %w", err)
	}
	if existingAccount != nil {
		return fmt.Errorf("您已绑定该平台的其他账号")
	}

	// 创建绑定
	account := &model.UserSocialAccount{
		UserID:     userID,
		ProviderID: provider.ID,
		OpenID:     socialUser.OpenID,
		UnionID:    socialUser.UnionID,
		UserInfo:   model.JSONObject(socialUser.RawUserInfo),
	}
	account.SetTokenInfo(
		token.AccessToken,
		token.RefreshToken,
		token.TokenType,
		int64(time.Until(token.Expiry).Seconds()),
	)

	if err := s.accountDAO.Create(account); err != nil {
		return fmt.Errorf("创建绑定失败: %w", err)
	}

	return nil
}

// UnbindSocialAccount 解绑社交账号
func (s *SocialAuthService) UnbindSocialAccount(userID, accountID uint) error {
	// 获取社交账号
	account, err := s.accountDAO.GetByID(accountID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("社交账号不存在")
		}
		return fmt.Errorf("获取社交账号失败: %w", err)
	}

	// 验证所有权
	if account.UserID != userID {
		return fmt.Errorf("无权解绑此账号")
	}

	// 检查是否为最后一个登录方式
	// 如果用户没有设置密码且只有一个社交账号，不允许解绑
	user, err := s.userDAO.GetByID(userID)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %w", err)
	}

	accountCount, err := s.accountDAO.CountByUser(userID)
	if err != nil {
		return fmt.Errorf("统计账号数失败: %w", err)
	}

	if user.PasswordHash == "" && accountCount <= 1 {
		return fmt.Errorf("这是您唯一的登录方式，无法解绑。请先设置密码或绑定其他账号")
	}

	// 删除绑定
	if err := s.accountDAO.Delete(accountID); err != nil {
		return fmt.Errorf("解绑失败: %w", err)
	}

	return nil
}

// loginWithSocialAccount 使用社交账号登录
func (s *SocialAuthService) loginWithSocialAccount(account *model.UserSocialAccount, socialUser *protocol.SocialUser, token interface{}) (*dto.SocialAuthCallbackResponse, error) {
	// 更新最后使用时间
	if err := s.accountDAO.UpdateLastUsed(account.ID); err != nil {
		// 记录日志，但不影响登录
		fmt.Printf("更新最后使用时间失败: %v\n", err)
	}

	// 获取用户信息
	userResponse, err := s.userService.GetUserByID(account.UserID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 生成JWT令牌
	jwtToken, err := s.jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		return nil, fmt.Errorf("生成令牌失败: %w", err)
	}

	return &dto.SocialAuthCallbackResponse{
		Token:     jwtToken,
		User:      userResponse,
		IsNewUser: false,
		NeedsBind: false,
	}, nil
}

// registerWithSocialAccount 使用社交账号注册
func (s *SocialAuthService) registerWithSocialAccount(provider *model.SocialProvider, socialUser *protocol.SocialUser, token interface{}) (*dto.SocialAuthCallbackResponse, error) {
	// 开启事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 创建新用户
	username := s.generateUsername(socialUser)
	fmt.Printf("DEBUG: 生成用户名: %s\n", username)

	user := &model.User{
		Username:    username,
		Nickname:    socialUser.Nickname,
		Avatar:      socialUser.Avatar,
		Email:       socialUser.Email,
		Status:      model.UserStatusActive,
		IsActivated: true,
	}

	userDAO := dao.NewUserDao(tx)
	if err := userDAO.Create(user); err != nil {
		tx.Rollback()
		fmt.Printf("DEBUG: 创建用户失败: %v\n", err)
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	fmt.Printf("DEBUG: 用户创建成功，ID: %d\n", user.ID)

	// 创建社交账号绑定
	// 从 token 中提取信息
	var accessToken, refreshToken, tokenType string
	var expiresIn int64

	// 类型断言获取 OAuth token
	type tokenWithFields interface {
		AccessToken() string
		RefreshToken() string
		TokenType() string
		Expiry() time.Time
	}

	// 尝试使用 *oauth2.Token
	if oauthToken, ok := token.(*oauth2.Token); ok {
		accessToken = oauthToken.AccessToken
		refreshToken = oauthToken.RefreshToken
		tokenType = oauthToken.TokenType
		expiresIn = int64(time.Until(oauthToken.Expiry).Seconds())
	}

	account := &model.UserSocialAccount{
		UserID:     user.ID,
		ProviderID: provider.ID,
		OpenID:     socialUser.OpenID,
		UnionID:    socialUser.UnionID,
		UserInfo:   model.JSONObject(socialUser.RawUserInfo),
	}
	account.SetTokenInfo(accessToken, refreshToken, tokenType, expiresIn)

	accountDAO := dao.NewUserSocialAccountDAO(tx)
	if err := accountDAO.Create(account); err != nil {
		tx.Rollback()
		fmt.Printf("DEBUG: 创建社交账号绑定失败: %v\n", err)
		return nil, fmt.Errorf("创建社交账号绑定失败: %w", err)
	}

	fmt.Printf("DEBUG: 社交账号绑定创建成功，ID: %d\n", account.ID)

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		fmt.Printf("DEBUG: 提交事务失败: %v\n", err)
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	fmt.Printf("DEBUG: 事务提交成功\n")

	// 生成用户响应
	fmt.Printf("DEBUG: 准备获取用户信息，用户ID: %d\n", user.ID)
	userResponse, err := s.userService.GetUserByID(user.ID)
	if err != nil {
		fmt.Printf("DEBUG: 获取用户信息失败: %v\n", err)
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	if userResponse == nil {
		fmt.Printf("DEBUG: GetUserByID 返回了 nil 响应但没有错误\n")
		return nil, fmt.Errorf("获取用户信息失败：响应为空")
	}

	fmt.Printf("DEBUG: 用户信息获取成功，用户ID: %d, 用户名: %s\n", userResponse.ID, userResponse.Username)

	// 生成JWT令牌
	jwtToken, err := s.jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		fmt.Printf("DEBUG: 生成令牌失败: %v\n", err)
		return nil, fmt.Errorf("生成令牌失败: %w", err)
	}

	fmt.Printf("DEBUG: JWT令牌生成成功\n")

	response := &dto.SocialAuthCallbackResponse{
		Token:     jwtToken,
		User:      userResponse,
		IsNewUser: true,
		NeedsBind: false,
	}

	fmt.Printf("DEBUG: 准备返回注册响应，响应对象地址: %p\n", response)
	return response, nil
}

// generateState 生成随机state
func (s *SocialAuthService) generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// storeState 存储state到Redis
func (s *SocialAuthService) storeState(state string, duration time.Duration) error {
	ctx := context.Background()
	key := s.stateKeyPrefix + state

	// 存储state到Redis，使用过期时间作为TTL
	// value存储当前时间戳，主要用于验证
	return s.redis.Set(ctx, key, time.Now().Unix(), duration).Err()
}

// validateState 验证state
func (s *SocialAuthService) validateState(state string) bool {
	ctx := context.Background()
	key := s.stateKeyPrefix + state

	// 尝试从Redis获取state
	_, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		// state不存在或已过期
		return false
	}
	if err != nil {
		// Redis错误，为安全起见返回false
		fmt.Printf("验证state时Redis错误: %v\n", err)
		return false
	}

	// 验证成功后删除state（一次性使用）
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		fmt.Printf("删除state时Redis错误: %v\n", err)
		// 即使删除失败，也认为验证成功，因为state存在
	}

	return true
}

// generateUsername 生成用户名
func (s *SocialAuthService) generateUsername(socialUser *protocol.SocialUser) string {
	// 基于OpenID生成唯一用户名
	baseUsername := "user_" + socialUser.OpenID
	if len(baseUsername) > 50 {
		baseUsername = baseUsername[:50]
	}

	// 检查用户名是否已存在
	_, err := s.userDAO.GetByUsername(baseUsername)
	if err == gorm.ErrRecordNotFound {
		return baseUsername
	}

	// 如果已存在，添加时间戳
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s_%d", baseUsername[:40], timestamp)
}

// ==================== 临时社交用户信息管理 ====================

// storeSocialUserTemp 存储临时社交用户信息到Redis
func (s *SocialAuthService) storeSocialUserTemp(bindToken string, tempUserInfo *TempSocialUserInfo) error {
	ctx := context.Background()
	key := s.tempUserKeyPrefix + bindToken

	// 序列化为JSON
	data, err := json.Marshal(tempUserInfo)
	if err != nil {
		return fmt.Errorf("序列化临时用户信息失败: %w", err)
	}

	// 存储到Redis，15分钟过期
	return s.redis.Set(ctx, key, data, 15*time.Minute).Err()
}

// getSocialUserTemp 从Redis获取临时社交用户信息
func (s *SocialAuthService) getSocialUserTemp(bindToken string) (*TempSocialUserInfo, error) {
	ctx := context.Background()
	key := s.tempUserKeyPrefix + bindToken

	// 从Redis获取
	data, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("绑定令牌无效或已过期")
	}
	if err != nil {
		return nil, fmt.Errorf("获取临时用户信息失败: %w", err)
	}

	// 反序列化
	var tempUserInfo TempSocialUserInfo
	if err := json.Unmarshal([]byte(data), &tempUserInfo); err != nil {
		return nil, fmt.Errorf("解析临时用户信息失败: %w", err)
	}

	return &tempUserInfo, nil
}

// deleteSocialUserTemp 删除临时社交用户信息
func (s *SocialAuthService) deleteSocialUserTemp(bindToken string) error {
	ctx := context.Background()
	key := s.tempUserKeyPrefix + bindToken
	return s.redis.Del(ctx, key).Err()
}

// ==================== 绑定和注册新方法 ====================

// BindWithExistingUser 用社交账号绑定到已有账户
func (s *SocialAuthService) BindWithExistingUser(bindToken, username, password string) (*dto.SocialAuthCallbackResponse, error) {
	// 获取临时社交用户信息
	tempUserInfo, err := s.getSocialUserTemp(bindToken)
	if err != nil {
		return nil, err
	}

	// 验证用户凭据
	user, err := s.userService.ValidateUserForLogin(username, password)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// 检查该社交账号是否已被其他用户绑定
	exists, err := s.accountDAO.ExistsByProviderAndOpenID(tempUserInfo.ProviderID, tempUserInfo.OpenID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("检查绑定状态失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("该社交账号已被其他用户绑定")
	}

	// 检查用户是否已绑定该提供商
	existingAccount, err := s.accountDAO.GetByUserAndProvider(user.ID, tempUserInfo.ProviderID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询已有绑定失败: %w", err)
	}
	if existingAccount != nil {
		return nil, fmt.Errorf("您已绑定该平台的其他账号")
	}

	// 创建绑定
	account := &model.UserSocialAccount{
		UserID:     user.ID,
		ProviderID: tempUserInfo.ProviderID,
		OpenID:     tempUserInfo.OpenID,
		UnionID:    tempUserInfo.UnionID,
		UserInfo:   model.JSONObject(tempUserInfo.RawUserInfo),
	}
	account.SetTokenInfo(tempUserInfo.AccessToken, "", tempUserInfo.TokenType, tempUserInfo.ExpiresIn)

	if err := s.accountDAO.Create(account); err != nil {
		return nil, fmt.Errorf("创建绑定失败: %w", err)
	}

	// 删除临时信息
	s.deleteSocialUserTemp(bindToken)

	// 生成用户响应
	userResponse, err := s.userService.GetUserByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 生成JWT令牌
	jwtToken, err := s.jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		return nil, fmt.Errorf("生成令牌失败: %w", err)
	}

	return &dto.SocialAuthCallbackResponse{
		Token:     jwtToken,
		User:      userResponse,
		IsNewUser: false,
		NeedsBind: false,
	}, nil
}

// RegisterWithSocial 用社交账号注册新用户
func (s *SocialAuthService) RegisterWithSocial(bindToken string) (*dto.SocialAuthCallbackResponse, error) {
	// 获取临时社交用户信息
	tempUserInfo, err := s.getSocialUserTemp(bindToken)
	if err != nil {
		return nil, err
	}

	// 获取提供商信息
	provider, err := s.providerDAO.GetByID(tempUserInfo.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("获取提供商信息失败: %w", err)
	}

	// 构造 SocialUser 对象
	socialUser := &protocol.SocialUser{
		OpenID:      tempUserInfo.OpenID,
		UnionID:     tempUserInfo.UnionID,
		Nickname:    tempUserInfo.Nickname,
		Avatar:      tempUserInfo.Avatar,
		Email:       tempUserInfo.Email,
		RawUserInfo: tempUserInfo.RawUserInfo,
	}

	// 开启事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 创建新用户
	username := s.generateUsername(socialUser)
	fmt.Printf("DEBUG: 生成用户名: %s\n", username)

	user := &model.User{
		Username:    username,
		Nickname:    socialUser.Nickname,
		Avatar:      socialUser.Avatar,
		Email:       socialUser.Email,
		Status:      model.UserStatusActive,
		IsActivated: true,
	}

	userDAO := dao.NewUserDao(tx)
	if err := userDAO.Create(user); err != nil {
		tx.Rollback()
		fmt.Printf("DEBUG: 创建用户失败: %v\n", err)
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	fmt.Printf("DEBUG: 用户创建成功，ID: %d\n", user.ID)

	// 创建社交账号绑定
	account := &model.UserSocialAccount{
		UserID:     user.ID,
		ProviderID: provider.ID,
		OpenID:     socialUser.OpenID,
		UnionID:    socialUser.UnionID,
		UserInfo:   model.JSONObject(socialUser.RawUserInfo),
	}
	account.SetTokenInfo(tempUserInfo.AccessToken, "", tempUserInfo.TokenType, tempUserInfo.ExpiresIn)

	accountDAO := dao.NewUserSocialAccountDAO(tx)
	if err := accountDAO.Create(account); err != nil {
		tx.Rollback()
		fmt.Printf("DEBUG: 创建社交账号绑定失败: %v\n", err)
		return nil, fmt.Errorf("创建社交账号绑定失败: %w", err)
	}

	fmt.Printf("DEBUG: 社交账号绑定创建成功，ID: %d\n", account.ID)

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		fmt.Printf("DEBUG: 提交事务失败: %v\n", err)
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	fmt.Printf("DEBUG: 事务提交成功\n")

	// 删除临时信息
	s.deleteSocialUserTemp(bindToken)

	// 生成用户响应
	fmt.Printf("DEBUG: 准备获取用户信息，用户ID: %d\n", user.ID)
	userResponse, err := s.userService.GetUserByID(user.ID)
	if err != nil {
		fmt.Printf("DEBUG: 获取用户信息失败: %v\n", err)
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	if userResponse == nil {
		fmt.Printf("DEBUG: GetUserByID 返回了 nil 响应但没有错误\n")
		return nil, fmt.Errorf("获取用户信息失败：响应为空")
	}

	fmt.Printf("DEBUG: 用户信息获取成功，用户ID: %d, 用户名: %s\n", userResponse.ID, userResponse.Username)

	// 生成JWT令牌
	jwtToken, err := s.jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		fmt.Printf("DEBUG: 生成令牌失败: %v\n", err)
		return nil, fmt.Errorf("生成令牌失败: %w", err)
	}

	fmt.Printf("DEBUG: JWT令牌生成成功\n")

	response := &dto.SocialAuthCallbackResponse{
		Token:     jwtToken,
		User:      userResponse,
		IsNewUser: true,
		NeedsBind: false,
	}

	fmt.Printf("DEBUG: 准备返回注册响应，响应对象地址: %p\n", response)
	return response, nil
}
