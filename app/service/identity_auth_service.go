package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type IdentityAuthService struct {
	db                *gorm.DB
	redis             *redis.Client
	providerDAO       *dao.IdentityProviderDAO
	accountDAO        *dao.UserIdentityAccountDAO
	userDAO           *dao.UserDao
	providerService   *IdentityProviderService
	userService       *UserService
	jwtAuthService    *JWTAuthService
	stateKeyPrefix    string
	tempUserKeyPrefix string
}

type TempIdentityUserInfo struct {
	ProviderID     uint                   `json:"providerId"`
	ExternalUserID string                 `json:"externalUserId"`
	UnionID        string                 `json:"unionId"`
	TenantID       string                 `json:"tenantId"`
	Nickname       string                 `json:"nickname"`
	Avatar         string                 `json:"avatar"`
	Email          string                 `json:"email"`
	Phone          string                 `json:"phone"`
	RawUserInfo    map[string]interface{} `json:"rawUserInfo"`
	AccessToken    string                 `json:"accessToken"`
	RefreshToken   string                 `json:"refreshToken"`
	TokenType      string                 `json:"tokenType"`
	ExpiresIn      int64                  `json:"expiresIn"`
}

func NewIdentityAuthService(db *gorm.DB, redisClient *redis.Client) *IdentityAuthService {
	return &IdentityAuthService{
		db:                db,
		redis:             redisClient,
		providerDAO:       dao.NewIdentityProviderDAO(db),
		accountDAO:        dao.NewUserIdentityAccountDAO(db),
		userDAO:           dao.NewUserDao(db),
		providerService:   NewIdentityProviderService(db),
		userService:       NewUserService(db),
		jwtAuthService:    newOptionalJWTAuthService(),
		stateKeyPrefix:    "identity:state:",
		tempUserKeyPrefix: "identity:temp_user:",
	}
}

func (s *IdentityAuthService) GetAuthURLWithSessionID(providerIdentifier string, sessionID string) (*dto.SocialAuthURLResponse, error) {
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}
	if !provider.IsEnabled() {
		return nil, fmt.Errorf("身份源未启用")
	}
	adapter, err := protocol.NewIdentityProviderAdapter(provider)
	if err != nil {
		return nil, fmt.Errorf("创建身份源适配器失败: %w", err)
	}

	state, err := s.generateState()
	if err != nil {
		return nil, fmt.Errorf("生成state失败: %w", err)
	}
	if err := s.storeStateWithSessionID(state, sessionID, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("存储state失败: %w", err)
	}

	return &dto.SocialAuthURLResponse{
		AuthURL: adapter.GetAuthURL(state),
		State:   state,
	}, nil
}

func (s *IdentityAuthService) HandleCallback(providerIdentifier, code, state string) (*dto.SocialAuthCallbackResponse, error) {
	sessionID, valid := s.validateAndGetSessionID(state)
	if !valid {
		return nil, fmt.Errorf("无效的state参数")
	}

	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return nil, err
	}
	if !provider.IsEnabled() {
		return nil, fmt.Errorf("身份源未启用")
	}

	adapter, err := protocol.NewIdentityProviderAdapter(provider)
	if err != nil {
		return nil, fmt.Errorf("创建身份源适配器失败: %w", err)
	}

	ctx := context.Background()
	token, err := adapter.ExchangeToken(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("交换令牌失败: %w", err)
	}
	user, err := adapter.GetUserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	return s.buildCallbackResponse(provider, user, token, sessionID)
}

func (s *IdentityAuthService) BindIdentityAccount(userID uint, providerIdentifier, code, state string) error {
	if !s.validateState(state) {
		return fmt.Errorf("无效的state参数")
	}
	provider, err := s.providerService.GetProviderByIdentifier(providerIdentifier)
	if err != nil {
		return err
	}
	adapter, err := protocol.NewIdentityProviderAdapter(provider)
	if err != nil {
		return fmt.Errorf("创建身份源适配器失败: %w", err)
	}
	ctx := context.Background()
	token, err := adapter.ExchangeToken(ctx, code)
	if err != nil {
		return fmt.Errorf("交换令牌失败: %w", err)
	}
	identityUser, err := adapter.GetUserInfo(ctx, token)
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %w", err)
	}

	exists, err := s.accountDAO.ExistsByProviderAndExternalUserID(provider.ID, identityUser.ExternalUserID, userID)
	if err != nil {
		return fmt.Errorf("检查绑定状态失败: %w", err)
	}
	if exists {
		return fmt.Errorf("该企业身份账号已被其他用户绑定")
	}

	existingAccount, err := s.accountDAO.GetByUserAndProvider(userID, provider.ID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("查询已有绑定失败: %w", err)
	}
	if existingAccount != nil {
		return fmt.Errorf("您已绑定该身份源的其他账号")
	}

	account := &model.UserIdentityAccount{
		UserID:         userID,
		ProviderID:     provider.ID,
		ExternalUserID: identityUser.ExternalUserID,
		UnionID:        identityUser.UnionID,
		TenantID:       identityUser.TenantID,
		UserInfo:       model.JSONObject(identityUser.RawUserInfo),
	}
	account.SetTokenInfo(token.AccessToken, token.RefreshToken, token.TokenType, token.ExpiresIn)
	if err := s.accountDAO.Create(account); err != nil {
		return fmt.Errorf("创建绑定失败: %w", err)
	}
	return nil
}

func (s *IdentityAuthService) UnbindIdentityAccount(userID, accountID uint) error {
	account, err := s.accountDAO.GetByID(accountID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("身份源账号不存在")
		}
		return fmt.Errorf("获取身份源账号失败: %w", err)
	}
	if account.UserID != userID {
		return fmt.Errorf("无权解绑此账号")
	}

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
	if err := s.accountDAO.Delete(accountID); err != nil {
		return fmt.Errorf("解绑失败: %w", err)
	}
	return nil
}

func (s *IdentityAuthService) BindWithExistingUser(bindToken, username, password string) (*dto.SocialAuthCallbackResponse, error) {
	tempUserInfo, err := s.getIdentityUserTemp(bindToken)
	if err != nil {
		return nil, err
	}
	user, err := s.userService.ValidateUserForLogin(username, password)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	exists, err := s.accountDAO.ExistsByProviderAndExternalUserID(tempUserInfo.ProviderID, tempUserInfo.ExternalUserID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("检查绑定状态失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("该企业身份账号已被其他用户绑定")
	}

	existingAccount, err := s.accountDAO.GetByUserAndProvider(user.ID, tempUserInfo.ProviderID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("查询已有绑定失败: %w", err)
	}
	if existingAccount != nil {
		return nil, fmt.Errorf("您已绑定该身份源的其他账号")
	}

	account := &model.UserIdentityAccount{
		UserID:         user.ID,
		ProviderID:     tempUserInfo.ProviderID,
		ExternalUserID: tempUserInfo.ExternalUserID,
		UnionID:        tempUserInfo.UnionID,
		TenantID:       tempUserInfo.TenantID,
		UserInfo:       model.JSONObject(tempUserInfo.RawUserInfo),
	}
	account.SetTokenInfo(tempUserInfo.AccessToken, tempUserInfo.RefreshToken, tempUserInfo.TokenType, tempUserInfo.ExpiresIn)
	if err := s.accountDAO.Create(account); err != nil {
		return nil, fmt.Errorf("创建绑定失败: %w", err)
	}
	_ = s.deleteIdentityUserTemp(bindToken)

	userResponse, err := s.userService.GetUserByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	jwtAuthService, err := ensureJWTAuthService(s.jwtAuthService)
	if err != nil {
		return nil, fmt.Errorf("JWT配置错误: %w", err)
	}
	s.jwtAuthService = jwtAuthService
	jwtToken, err := jwtAuthService.GenerateToken(userResponse)
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

func (s *IdentityAuthService) RegisterWithProvider(req *dto.SocialRegisterRequest) (*dto.SocialAuthCallbackResponse, error) {
	tempUserInfo, err := s.getIdentityUserTemp(req.BindToken)
	if err != nil {
		return nil, err
	}

	exists, err := s.userDAO.ExistsUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("用户名已存在")
	}
	exists, err = s.userDAO.ExistsEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("邮箱已被注册")
	}

	provider, err := s.providerDAO.GetByID(tempUserInfo.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("获取身份源信息失败: %w", err)
	}

	salt, passwordHash, err := s.userService.GeneratePasswordHash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var email *string
	if req.Email != "" {
		email = &req.Email
	}
	var phone *string
	if tempUserInfo.Phone != "" {
		phone = &tempUserInfo.Phone
	}
	user := &model.User{
		Username:     req.Username,
		Email:        email,
		Phone:        phone,
		RealName:     req.RealName,
		Nickname:     tempUserInfo.Nickname,
		Avatar:       tempUserInfo.Avatar,
		Salt:         salt,
		PasswordHash: passwordHash,
		Status:       model.UserStatusActive,
		IsActivated:  true,
	}

	userDAO := dao.NewUserDao(tx)
	if err := userDAO.Create(user); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	account := &model.UserIdentityAccount{
		UserID:         user.ID,
		ProviderID:     provider.ID,
		ExternalUserID: tempUserInfo.ExternalUserID,
		UnionID:        tempUserInfo.UnionID,
		TenantID:       tempUserInfo.TenantID,
		UserInfo:       model.JSONObject(tempUserInfo.RawUserInfo),
	}
	account.SetTokenInfo(tempUserInfo.AccessToken, tempUserInfo.RefreshToken, tempUserInfo.TokenType, tempUserInfo.ExpiresIn)

	accountDAO := dao.NewUserIdentityAccountDAO(tx)
	if err := accountDAO.Create(account); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("创建身份源账号绑定失败: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}
	_ = s.deleteIdentityUserTemp(req.BindToken)

	userResponse, err := s.userService.GetUserByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	if userResponse == nil {
		return nil, fmt.Errorf("获取用户信息失败：响应为空")
	}

	jwtAuthService, err := ensureJWTAuthService(s.jwtAuthService)
	if err != nil {
		return nil, fmt.Errorf("JWT配置错误: %w", err)
	}
	s.jwtAuthService = jwtAuthService
	jwtToken, err := jwtAuthService.GenerateToken(userResponse)
	if err != nil {
		return nil, fmt.Errorf("生成令牌失败: %w", err)
	}
	return &dto.SocialAuthCallbackResponse{
		Token:     jwtToken,
		User:      userResponse,
		IsNewUser: true,
		NeedsBind: false,
	}, nil
}

func (s *IdentityAuthService) GetIdentityUserTempInfo(bindToken string) (*TempIdentityUserInfo, error) {
	return s.getIdentityUserTemp(bindToken)
}

func (s *IdentityAuthService) buildCallbackResponse(provider *model.IdentityProvider, identityUser *protocol.IdentityUser, token *protocol.IdentityToken, sessionID string) (*dto.SocialAuthCallbackResponse, error) {
	account, err := s.accountDAO.GetByProviderAndExternalUserID(provider.ID, identityUser.ExternalUserID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询绑定账号失败: %w", err)
	}
	if account != nil {
		response, err := s.loginWithIdentityAccount(account)
		if err != nil {
			return nil, err
		}
		response.SessionID = sessionID
		return response, nil
	}

	bindToken := uuid.New().String()
	tempUserInfo := &TempIdentityUserInfo{
		ProviderID:     provider.ID,
		ExternalUserID: identityUser.ExternalUserID,
		UnionID:        identityUser.UnionID,
		TenantID:       identityUser.TenantID,
		Nickname:       identityUser.Nickname,
		Avatar:         identityUser.Avatar,
		Email:          identityUser.Email,
		Phone:          identityUser.Phone,
		RawUserInfo:    identityUser.RawUserInfo,
	}
	if token != nil {
		tempUserInfo.AccessToken = token.AccessToken
		tempUserInfo.RefreshToken = token.RefreshToken
		tempUserInfo.TokenType = token.TokenType
		tempUserInfo.ExpiresIn = token.ExpiresIn
	}
	if err := s.storeIdentityUserTemp(bindToken, tempUserInfo); err != nil {
		return nil, fmt.Errorf("存储临时用户信息失败: %w", err)
	}
	return &dto.SocialAuthCallbackResponse{
		NeedsBind: true,
		BindToken: bindToken,
		SessionID: sessionID,
	}, nil
}

func (s *IdentityAuthService) loginWithIdentityAccount(account *model.UserIdentityAccount) (*dto.SocialAuthCallbackResponse, error) {
	if err := s.accountDAO.UpdateLastUsed(account.ID); err != nil {
		fmt.Printf("更新身份源账号最后使用时间失败: %v\n", err)
	}
	userResponse, err := s.userService.GetUserByID(account.UserID)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	jwtAuthService, err := ensureJWTAuthService(s.jwtAuthService)
	if err != nil {
		return nil, fmt.Errorf("JWT配置错误: %w", err)
	}
	s.jwtAuthService = jwtAuthService
	jwtToken, err := jwtAuthService.GenerateToken(userResponse)
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

func (s *IdentityAuthService) generateState() (string, error) {
	v7, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return v7.String(), nil
}

func (s *IdentityAuthService) storeStateWithSessionID(state string, sessionID string, duration time.Duration) error {
	if s.redis == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	data, err := json.Marshal(StateData{SessionID: sessionID, Timestamp: time.Now().Unix()})
	if err != nil {
		return fmt.Errorf("序列化state数据失败: %w", err)
	}
	return s.redis.Set(context.Background(), s.stateKeyPrefix+state, string(data), duration).Err()
}

func (s *IdentityAuthService) validateState(state string) bool {
	_, valid := s.validateAndGetSessionID(state)
	return valid
}

func (s *IdentityAuthService) validateAndGetSessionID(state string) (string, bool) {
	if s.redis == nil {
		return "", false
	}
	data, err := s.redis.Get(context.Background(), s.stateKeyPrefix+state).Result()
	if errors.Is(err, redis.Nil) {
		return "", false
	}
	if err != nil {
		fmt.Printf("验证身份源state时Redis错误: %v\n", err)
		return "", false
	}
	if err := s.redis.Del(context.Background(), s.stateKeyPrefix+state).Err(); err != nil {
		fmt.Printf("删除身份源state时Redis错误: %v\n", err)
	}
	var stateData StateData
	if err := json.Unmarshal([]byte(data), &stateData); err != nil {
		return "", true
	}
	return stateData.SessionID, true
}

func (s *IdentityAuthService) storeIdentityUserTemp(bindToken string, tempUserInfo *TempIdentityUserInfo) error {
	if s.redis == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	data, err := json.Marshal(tempUserInfo)
	if err != nil {
		return fmt.Errorf("序列化临时用户信息失败: %w", err)
	}
	return s.redis.Set(context.Background(), s.tempUserKeyPrefix+bindToken, data, 15*time.Minute).Err()
}

func (s *IdentityAuthService) getIdentityUserTemp(bindToken string) (*TempIdentityUserInfo, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	data, err := s.redis.Get(context.Background(), s.tempUserKeyPrefix+bindToken).Result()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("绑定令牌无效或已过期")
	}
	if err != nil {
		return nil, fmt.Errorf("获取临时用户信息失败: %w", err)
	}
	var tempUserInfo TempIdentityUserInfo
	if err := json.Unmarshal([]byte(data), &tempUserInfo); err != nil {
		return nil, fmt.Errorf("解析临时用户信息失败: %w", err)
	}
	return &tempUserInfo, nil
}

func (s *IdentityAuthService) deleteIdentityUserTemp(bindToken string) error {
	if s.redis == nil {
		return nil
	}
	return s.redis.Del(context.Background(), s.tempUserKeyPrefix+bindToken).Err()
}
