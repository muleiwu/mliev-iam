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

const (
	externalStateKeyPrefix = "external-auth:state:"
	externalTempKeyPrefix  = "external-auth:temp:"
)

type ExternalAuthService struct {
	db             *gorm.DB
	redis          *redis.Client
	connectionDAO  *dao.IdentityConnectionDAO
	userDAO        *dao.UserDao
	userService    *UserService
	jwtAuthService *JWTAuthService
}

type ExternalTempProfile struct {
	ConnectionID uint                      `json:"connectionId"`
	Profile      *protocol.ExternalProfile `json:"profile"`
	Category     model.ConnectionCategory  `json:"category"`
	Connection   string                    `json:"connection"`
	Identifier   string                    `json:"identifier"`
}

func NewExternalAuthService(db *gorm.DB, redisClient *redis.Client) *ExternalAuthService {
	return &ExternalAuthService{
		db: db, redis: redisClient, connectionDAO: dao.NewIdentityConnectionDAO(db),
		userDAO: dao.NewUserDao(db), userService: NewUserService(db),
		jwtAuthService: newOptionalJWTAuthService(),
	}
}

func (s *ExternalAuthService) GetAuthURL(identifier, sessionID string) (*dto.SocialAuthURLResponse, error) {
	return s.authURL(identifier, StateData{SessionID: sessionID})
}

func (s *ExternalAuthService) GetBindAuthURL(identifier string, userID uint) (*dto.SocialAuthURLResponse, error) {
	return s.authURL(identifier, StateData{Mode: StateModeBind, BindUserID: userID})
}

func (s *ExternalAuthService) authURL(identifier string, stateData StateData) (*dto.SocialAuthURLResponse, error) {
	connection, adapter, err := s.enabledAdapter(identifier)
	if err != nil {
		return nil, err
	}
	authURL := adapter.AuthorizationURL("")
	if authURL == "" {
		return nil, fmt.Errorf("连接 %s 不支持浏览器授权流程", connection.Identifier)
	}
	state, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	stateData.Timestamp = time.Now().Unix()
	if err := s.storeJSON(externalStateKeyPrefix+state.String(), stateData, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("保存认证 state 失败: %w", err)
	}
	authURL = adapter.AuthorizationURL(state.String())
	return &dto.SocialAuthURLResponse{AuthURL: authURL, State: state.String()}, nil
}

func (s *ExternalAuthService) ConsumeState(state string) (*StateData, bool) {
	if s.redis == nil || state == "" {
		return nil, false
	}
	ctx := context.Background()
	key := externalStateKeyPrefix + state
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		return nil, false
	}
	var stateData StateData
	if err := json.Unmarshal(data, &stateData); err != nil {
		return nil, false
	}
	return &stateData, true
}

func (s *ExternalAuthService) HandleCallback(identifier, code, sessionID string) (*dto.SocialAuthCallbackResponse, error) {
	connection, adapter, err := s.enabledAdapter(identifier)
	if err != nil {
		return nil, err
	}
	token, err := adapter.ExchangeCode(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("交换授权码失败: %w", err)
	}
	profile, err := adapter.FetchProfile(context.Background(), token)
	if err != nil {
		return nil, fmt.Errorf("读取外部用户资料失败: %w", err)
	}
	return s.completeAuthentication(connection, profile, sessionID)
}

func (s *ExternalAuthService) HandleMiniProgramLogin(identifier, code, sessionID string) (*dto.SocialAuthCallbackResponse, error) {
	connection, adapter, err := s.enabledAdapter(identifier)
	if err != nil {
		return nil, err
	}
	codeAdapter, ok := adapter.(protocol.CodeAuthenticationAdapter)
	if !ok {
		return nil, fmt.Errorf("连接 %s 不支持小程序 code 登录", identifier)
	}
	profile, err := codeAdapter.LoginWithCode(context.Background(), code)
	if err != nil {
		return nil, err
	}
	return s.completeAuthentication(connection, profile, sessionID)
}

func (s *ExternalAuthService) completeAuthentication(connection *model.IdentityConnection, profile *protocol.ExternalProfile, sessionID string) (*dto.SocialAuthCallbackResponse, error) {
	principal, err := s.findPrincipal(connection.ID, profile)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if principal != nil {
		if principal.Status == model.PrincipalStatusBlocked {
			return nil, fmt.Errorf("该企业身份已被停用")
		}
		if principal.UserID != nil {
			if err := s.refreshPrincipal(principal, profile, true); err != nil {
				return nil, err
			}
			response, err := s.loginPrincipal(principal)
			if err != nil {
				return nil, err
			}
			response.SessionID = sessionID
			return response, nil
		}
	}

	bindToken := uuid.NewString()
	temp := ExternalTempProfile{
		ConnectionID: connection.ID, Profile: profile, Category: connection.Category,
		Connection: connection.Name, Identifier: connection.Identifier,
	}
	if err := s.storeJSON(externalTempKeyPrefix+bindToken, temp, 15*time.Minute); err != nil {
		return nil, fmt.Errorf("保存临时外部身份失败: %w", err)
	}
	return &dto.SocialAuthCallbackResponse{NeedsBind: true, BindToken: bindToken, SessionID: sessionID}, nil
}

func (s *ExternalAuthService) GetTempProfile(bindToken string) (*ExternalTempProfile, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	data, err := s.redis.Get(context.Background(), externalTempKeyPrefix+bindToken).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("绑定令牌无效或已过期")
	}
	if err != nil {
		return nil, err
	}
	var temp ExternalTempProfile
	if err := json.Unmarshal(data, &temp); err != nil {
		return nil, err
	}
	return &temp, nil
}

func (s *ExternalAuthService) BindWithCode(userID uint, identifier, code string) error {
	connection, adapter, err := s.enabledAdapter(identifier)
	if err != nil {
		return err
	}
	token, err := adapter.ExchangeCode(context.Background(), code)
	if err != nil {
		return err
	}
	profile, err := adapter.FetchProfile(context.Background(), token)
	if err != nil {
		return err
	}
	return s.attachProfile(s.db, connection.ID, userID, profile)
}

func (s *ExternalAuthService) BindWithExistingUser(bindToken, username, password string) (*dto.SocialAuthCallbackResponse, error) {
	temp, err := s.GetTempProfile(bindToken)
	if err != nil {
		return nil, err
	}
	user, err := s.userService.ValidateUserForLogin(username, password)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if err := s.attachProfile(s.db, temp.ConnectionID, user.ID, temp.Profile); err != nil {
		return nil, err
	}
	_ = s.deleteTemp(bindToken)
	return s.loginUser(user.ID, false)
}

func (s *ExternalAuthService) Register(req *dto.SocialRegisterRequest) (*dto.SocialAuthCallbackResponse, error) {
	temp, err := s.GetTempProfile(req.BindToken)
	if err != nil {
		return nil, err
	}
	agreementService := NewAgreementService(s.db)
	appID, err := agreementService.ResolveApplicationIDFromSession(context.Background(), req.SessionID, s.redis)
	if err != nil {
		return nil, err
	}
	required, err := agreementService.ListRequiredRegistrationAgreements(appID)
	if err != nil {
		return nil, err
	}
	accepted := req.AcceptedAgreementKeys
	if len(accepted) == 0 && req.AgreePolicy {
		accepted = AgreementKeys(required)
	}
	if err := agreementService.EnsureAgreementsAccepted(required, accepted); err != nil {
		return nil, err
	}
	if exists, err := s.userDAO.ExistsUsername(req.Username); err != nil || exists {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("用户名已存在")
	}
	if exists, err := s.userDAO.ExistsEmail(req.Email); err != nil || exists {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("邮箱已被注册")
	}
	salt, passwordHash, err := s.userService.GeneratePasswordHash(req.Password)
	if err != nil {
		return nil, err
	}
	email := req.Email
	var phone *string
	if temp.Profile.Phone != "" {
		phone = &temp.Profile.Phone
	}
	user := &model.User{
		Username: req.Username, Email: &email, Phone: phone, RealName: req.RealName,
		Nickname: temp.Profile.DisplayName, Avatar: temp.Profile.Avatar, Salt: salt,
		PasswordHash: passwordHash, Status: model.UserStatusActive, IsActivated: true,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.NewUserDao(tx).Create(user); err != nil {
			return err
		}
		if err := s.attachProfile(tx, temp.ConnectionID, user.ID, temp.Profile); err != nil {
			return err
		}
		return NewAgreementService(tx).RecordAcceptances(user.ID, appID, required)
	})
	if err != nil {
		return nil, fmt.Errorf("注册外部身份用户失败: %w", err)
	}
	_ = s.deleteTemp(req.BindToken)
	return s.loginUser(user.ID, true)
}

func (s *ExternalAuthService) ListUserIdentities(userID uint) ([]dto.ExternalIdentityResponse, error) {
	var principals []model.ExternalPrincipal
	if err := s.db.Preload("Connection").Where("user_id = ?", userID).Order("created_at ASC").Find(&principals).Error; err != nil {
		return nil, err
	}
	result := make([]dto.ExternalIdentityResponse, 0, len(principals))
	for _, principal := range principals {
		var identifiers []model.ExternalIdentifier
		if err := s.db.Where("principal_id = ?", principal.ID).Order("is_primary DESC, id ASC").Find(&identifiers).Error; err != nil {
			return nil, err
		}
		grouped := make(map[string][]string)
		for _, identifier := range identifiers {
			grouped[string(identifier.Namespace)] = append(grouped[string(identifier.Namespace)], identifier.Value)
		}
		connection := principal.Connection
		if connection == nil {
			continue
		}
		result = append(result, dto.ExternalIdentityResponse{
			ID: principal.ID, ConnectionID: connection.ID, Connection: connection.Name,
			Identifier: connection.Identifier, Category: connection.Category, ProviderType: connection.ProviderType,
			Icon: connection.Icon, Status: principal.Status, DisplayName: principal.DisplayName,
			Avatar: principal.Avatar, Email: principal.Email, Phone: principal.Phone,
			Identifiers: grouped, LastLoginAt: principal.LastLoginAt, LoginCount: principal.LoginCount,
			CreatedAt: principal.CreatedAt,
		})
	}
	return result, nil
}

func (s *ExternalAuthService) Unbind(userID, principalID uint) error {
	var principal model.ExternalPrincipal
	if err := s.db.First(&principal, principalID).Error; err != nil {
		return fmt.Errorf("外部身份不存在")
	}
	if principal.UserID == nil || *principal.UserID != userID {
		return fmt.Errorf("无权解绑该外部身份")
	}
	user, err := s.userDAO.GetByID(userID)
	if err != nil {
		return err
	}
	var count int64
	if err := s.db.Model(&model.ExternalPrincipal{}).Where("user_id = ? AND status = ?", userID, model.PrincipalStatusActive).Count(&count).Error; err != nil {
		return err
	}
	if user.PasswordHash == "" && count <= 1 {
		return fmt.Errorf("这是您唯一的登录方式，请先设置密码或绑定其他账号")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("principal_id = ?", principalID).Delete(&model.ExternalIdentifier{}).Error; err != nil {
			return err
		}
		return tx.Delete(&principal).Error
	})
}

func (s *ExternalAuthService) enabledAdapter(identifier string) (*model.IdentityConnection, protocol.AuthenticationAdapter, error) {
	connection, err := s.connectionDAO.GetByIdentifier(identifier)
	if err != nil {
		return nil, nil, fmt.Errorf("身份连接不存在")
	}
	if !connection.IsEnabled() || !connection.HasCapability(model.ConnectionCapabilityAuthentication) {
		return nil, nil, fmt.Errorf("身份连接未启用认证能力")
	}
	adapter, err := BuildAuthenticationAdapter(s.db, connection, s.redis)
	if err != nil {
		return nil, nil, err
	}
	if err := adapter.ValidateConfig(); err != nil {
		return nil, nil, err
	}
	return connection, adapter, nil
}

func (s *ExternalAuthService) findPrincipal(connectionID uint, profile *protocol.ExternalProfile) (*model.ExternalPrincipal, error) {
	primary, err := primaryLoginSubject(profile)
	if err != nil {
		return nil, err
	}
	var principal model.ExternalPrincipal
	err = s.db.Joins("JOIN external_identifiers ei ON ei.principal_id = external_principals.id").
		Where("ei.connection_id = ? AND ei.namespace = ? AND ei.value = ?", connectionID, model.IdentifierNamespaceLoginSubject, primary.Value).
		First(&principal).Error
	if err != nil {
		return nil, err
	}
	return &principal, nil
}

func (s *ExternalAuthService) attachProfile(db *gorm.DB, connectionID, userID uint, profile *protocol.ExternalProfile) error {
	primary, err := primaryLoginSubject(profile)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var existingIdentifier model.ExternalIdentifier
		err := tx.Where("connection_id = ? AND namespace = ? AND value = ?", connectionID, model.IdentifierNamespaceLoginSubject, primary.Value).First(&existingIdentifier).Error
		if err == nil {
			var principal model.ExternalPrincipal
			if err := tx.First(&principal, existingIdentifier.PrincipalID).Error; err != nil {
				return err
			}
			if principal.UserID != nil && *principal.UserID != userID {
				return fmt.Errorf("该外部账号已被其他用户绑定")
			}
			principal.UserID = &userID
			principal.Status = model.PrincipalStatusActive
			return tx.Save(&principal).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var userConnectionCount int64
		if err := tx.Model(&model.ExternalPrincipal{}).Where("connection_id = ? AND user_id = ?", connectionID, userID).Count(&userConnectionCount).Error; err != nil {
			return err
		}
		if userConnectionCount > 0 {
			return fmt.Errorf("您已绑定该连接的其他外部账号")
		}
		principal := &model.ExternalPrincipal{
			ConnectionID: connectionID, UserID: &userID, Status: model.PrincipalStatusActive,
			DisplayName: profile.DisplayName, Avatar: profile.Avatar, Email: profile.Email,
			Phone: profile.Phone, RawProfile: model.JSONObject(profile.Raw),
		}
		if err := tx.Create(principal).Error; err != nil {
			return err
		}
		for _, identifier := range profile.Identifiers {
			if identifier.Value == "" {
				continue
			}
			externalIdentifier := model.ExternalIdentifier{
				ConnectionID: connectionID, PrincipalID: principal.ID, Namespace: identifier.Namespace,
				Value: identifier.Value, IsPrimary: identifier.Primary,
			}
			if err := tx.Create(&externalIdentifier).Error; err != nil {
				if identifier.Primary {
					return err
				}
			}
		}
		return nil
	})
}

func (s *ExternalAuthService) refreshPrincipal(principal *model.ExternalPrincipal, profile *protocol.ExternalProfile, loggedIn bool) error {
	updates := map[string]any{
		"display_name": profile.DisplayName, "avatar": profile.Avatar, "email": profile.Email,
		"phone": profile.Phone, "raw_profile": model.JSONObject(profile.Raw),
	}
	if loggedIn {
		updates["last_login_at"] = time.Now()
		updates["login_count"] = gorm.Expr("login_count + 1")
	}
	return s.db.Model(principal).Updates(updates).Error
}

func (s *ExternalAuthService) loginPrincipal(principal *model.ExternalPrincipal) (*dto.SocialAuthCallbackResponse, error) {
	if principal.UserID == nil {
		return nil, fmt.Errorf("外部身份尚未绑定本地用户")
	}
	return s.loginUser(*principal.UserID, false)
}

func (s *ExternalAuthService) loginUser(userID uint, isNew bool) (*dto.SocialAuthCallbackResponse, error) {
	userResponse, err := s.userService.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	jwtService, err := ensureJWTAuthService(s.jwtAuthService)
	if err != nil {
		return nil, err
	}
	s.jwtAuthService = jwtService
	token, err := jwtService.GenerateUserToken(userResponse)
	if err != nil {
		return nil, err
	}
	return &dto.SocialAuthCallbackResponse{Token: token, User: userResponse, IsNewUser: isNew}, nil
}

func primaryLoginSubject(profile *protocol.ExternalProfile) (*protocol.ExternalProfileIdentifier, error) {
	if profile == nil {
		return nil, fmt.Errorf("外部用户资料为空")
	}
	var primary *protocol.ExternalProfileIdentifier
	for index := range profile.Identifiers {
		identifier := &profile.Identifiers[index]
		if identifier.Namespace == model.IdentifierNamespaceLoginSubject && identifier.Primary && identifier.Value != "" {
			if primary != nil {
				return nil, fmt.Errorf("外部用户资料包含多个主 login_subject")
			}
			primary = identifier
		}
	}
	if primary == nil {
		return nil, fmt.Errorf("外部用户资料缺少主 login_subject")
	}
	return primary, nil
}

func (s *ExternalAuthService) storeJSON(key string, value any, ttl time.Duration) error {
	if s.redis == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.redis.Set(context.Background(), key, data, ttl).Err()
}

func (s *ExternalAuthService) deleteTemp(bindToken string) error {
	if s.redis == nil {
		return nil
	}
	return s.redis.Del(context.Background(), externalTempKeyPrefix+bindToken).Err()
}
