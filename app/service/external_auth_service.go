package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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
	SessionID    string                    `json:"sessionId,omitempty"`
	CSRFHash     string                    `json:"csrfHash"`
}

func NewExternalAuthService(db *gorm.DB, redisClient *redis.Client) *ExternalAuthService {
	return &ExternalAuthService{
		db: db, redis: redisClient, connectionDAO: dao.NewIdentityConnectionDAO(db),
		userDAO: dao.NewUserDao(db), userService: NewUserService(db),
		jwtAuthService: newOptionalJWTAuthService(),
	}
}

func (s *ExternalAuthService) GetAuthURL(identifier, sessionID, openerOrigin, browserBindingHash string) (*dto.SocialAuthURLResponse, error) {
	return s.authURL(identifier, StateData{SessionID: sessionID, OpenerOrigin: openerOrigin, BrowserBindingHash: browserBindingHash})
}

func (s *ExternalAuthService) GetBindAuthURL(identifier string, userID uint, openerOrigin string) (*dto.SocialAuthURLResponse, error) {
	return s.authURL(identifier, StateData{Mode: StateModeBind, BindUserID: userID, OpenerOrigin: openerOrigin})
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
	stateData.ConnectionIdentifier = connection.Identifier
	if err := s.storeJSON(externalStateKeyPrefix+state.String(), stateData, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("保存认证 state 失败: %w", err)
	}
	authURL = adapter.AuthorizationURL(state.String())
	return &dto.SocialAuthURLResponse{AuthURL: authURL, State: state.String()}, nil
}

func (s *ExternalAuthService) ConsumeState(ctx context.Context, state string) (*StateData, bool) {
	if s.redis == nil || state == "" {
		return nil, false
	}
	key := externalStateKeyPrefix + state
	data, err := s.redis.GetDel(ctx, key).Bytes()
	if err != nil {
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
			response, err := s.loginPrincipal(principal, sessionID == "")
			if err != nil {
				return nil, err
			}
			response.SessionID = sessionID
			return response, nil
		}
	}

	continuationID, csrf, err := s.storeContinuation(connection, profile, sessionID)
	if err != nil {
		return nil, fmt.Errorf("保存临时外部身份失败: %w", err)
	}
	return &dto.SocialAuthCallbackResponse{NeedsBind: true, ContinuationCSRF: csrf, SessionID: sessionID, ContinuationID: continuationID}, nil
}

func (s *ExternalAuthService) storeContinuation(connection *model.IdentityConnection, profile *protocol.ExternalProfile, sessionID string) (string, string, error) {
	continuationID := uuid.NewString()
	csrf, err := newCsrfToken()
	if err != nil {
		return "", "", err
	}
	csrfDigest := sha256.Sum256([]byte(csrf))
	temp := ExternalTempProfile{ConnectionID: connection.ID, Profile: profile, Category: connection.Category,
		Connection: connection.Name, Identifier: connection.Identifier, SessionID: sessionID,
		CSRFHash: base64.RawURLEncoding.EncodeToString(csrfDigest[:])}
	if err := s.storeJSON(externalTempKeyPrefix+continuationID, temp, 15*time.Minute); err != nil {
		return "", "", err
	}
	return continuationID, csrf, nil
}

func (s *ExternalAuthService) GetTempProfile(continuationID string) (*ExternalTempProfile, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	data, err := s.redis.Get(context.Background(), externalTempKeyPrefix+continuationID).Bytes()
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

func validateContinuationCSRF(temp *ExternalTempProfile, csrf string) bool {
	if temp == nil || temp.CSRFHash == "" || csrf == "" {
		return false
	}
	digest := sha256.Sum256([]byte(csrf))
	actual := base64.RawURLEncoding.EncodeToString(digest[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(temp.CSRFHash)) == 1
}

func (s *ExternalAuthService) lockContinuation(ctx context.Context, continuationID, csrf string) (*ExternalTempProfile, func(), error) {
	if s.redis == nil || continuationID == "" {
		return nil, nil, errors.New("continuation 无效或已过期")
	}
	lockKey := externalTempKeyPrefix + continuationID + ":lock"
	lockToken := uuid.NewString()
	locked, err := s.redis.SetNX(ctx, lockKey, lockToken, 30*time.Second).Result()
	if err != nil || !locked {
		return nil, nil, errors.New("continuation 正在处理，请稍后重试")
	}
	release := func() {
		const releaseScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
		_ = s.redis.Eval(context.Background(), releaseScript, []string{lockKey}, lockToken).Err()
	}
	temp, err := s.GetTempProfile(continuationID)
	if err != nil || !validateContinuationCSRF(temp, csrf) {
		release()
		return nil, nil, errors.New("continuation 校验失败或已过期")
	}
	return temp, release, nil
}

func (s *ExternalAuthService) consumeContinuation(ctx context.Context, continuationID string) error {
	result, err := s.redis.GetDel(ctx, externalTempKeyPrefix+continuationID).Result()
	if err != nil || result == "" {
		return errors.New("continuation 已被使用")
	}
	return nil
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

func (s *ExternalAuthService) BindWithExistingUser(ctx context.Context, continuationID, csrf, username, password string) (*dto.SocialAuthCallbackResponse, error) {
	temp, release, err := s.lockContinuation(ctx, continuationID, csrf)
	if err != nil {
		return nil, err
	}
	defer release()
	user, err := s.userService.ValidateUserForLogin(username, password)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if user.TwoFactorEnabled {
		return &dto.SocialAuthCallbackResponse{User: user, SessionID: temp.SessionID, PendingContinuationID: continuationID, PendingContinuationCSRF: csrf}, nil
	}
	if err := s.consumeContinuation(ctx, continuationID); err != nil {
		return nil, err
	}
	if err := s.attachProfile(s.db, temp.ConnectionID, user.ID, temp.Profile); err != nil {
		return nil, err
	}
	response, err := s.loginUser(user.ID, false, temp.SessionID == "")
	if response != nil {
		response.SessionID = temp.SessionID
	}
	return response, err
}

func (s *ExternalAuthService) Register(ctx context.Context, continuationID string, req *dto.SocialRegisterRequest) (*dto.SocialAuthCallbackResponse, error) {
	temp, release, err := s.lockContinuation(ctx, continuationID, req.ContinuationCSRF)
	if err != nil {
		return nil, err
	}
	defer release()
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
	if err := s.consumeContinuation(ctx, continuationID); err != nil {
		return nil, err
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
	response, err := s.loginUser(user.ID, true, temp.SessionID == "")
	if response != nil {
		response.SessionID = temp.SessionID
	}
	return response, err
}

func (s *ExternalAuthService) CheckContinuationRateLimit(ctx context.Context, continuationID, clientIP, account string) error {
	if s.redis == nil {
		return errors.New("Redis 未初始化")
	}
	digestKey := func(value string) string {
		digest := sha256.Sum256([]byte(value))
		return base64.RawURLEncoding.EncodeToString(digest[:12])
	}
	checks := []struct {
		key string
		max int64
	}{
		{"external-auth:limit:continuation:" + digestKey(continuationID), 10},
		{"external-auth:limit:ip:" + digestKey(clientIP), 30},
		{"external-auth:limit:account:" + digestKey(account), 10},
	}
	for _, check := range checks {
		count, err := s.redis.Incr(ctx, check.key).Result()
		if err != nil {
			return err
		}
		if count == 1 {
			_ = s.redis.Expire(ctx, check.key, 5*time.Minute).Err()
		}
		if count > check.max {
			return errors.New("尝试次数过多，请稍后重试")
		}
	}
	return nil
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
	if _, err := primaryLoginSubject(profile); err != nil {
		return nil, err
	}
	return NewExternalPrincipalResolver(s.db).Resolve(connectionID, profilePrincipalIdentifiers(profile), false)
}

func (s *ExternalAuthService) attachProfile(db *gorm.DB, connectionID, userID uint, profile *protocol.ExternalProfile) error {
	if _, err := primaryLoginSubject(profile); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		resolver := NewExternalPrincipalResolver(tx)
		principal, err := resolver.Resolve(connectionID, profilePrincipalIdentifiers(profile), false)
		if err == nil && principal != nil {
			if principal.UserID != nil && *principal.UserID != userID {
				return fmt.Errorf("该外部账号已被其他用户绑定")
			}
			principal.UserID = &userID
			principal.Status = model.PrincipalStatusActive
			return tx.Save(principal).Error
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
		principal = &model.ExternalPrincipal{
			ConnectionID: connectionID, UserID: &userID, Status: model.PrincipalStatusActive,
			DisplayName: profile.DisplayName, Avatar: profile.Avatar, Email: profile.Email,
			Phone: profile.Phone, RawProfile: model.JSONObject(profile.Raw),
		}
		if err := tx.Create(principal).Error; err != nil {
			return err
		}
		return resolver.EnsureIdentifiers(tx, principal, profilePrincipalIdentifiers(profile))
	})
}

func profilePrincipalIdentifiers(profile *protocol.ExternalProfile) []PrincipalIdentifierInput {
	if profile == nil {
		return nil
	}
	result := make([]PrincipalIdentifierInput, 0, len(profile.Identifiers))
	for _, identifier := range profile.Identifiers {
		result = append(result, PrincipalIdentifierInput{Namespace: identifier.Namespace, Value: identifier.Value, Primary: identifier.Primary})
	}
	return result
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

func (s *ExternalAuthService) loginPrincipal(principal *model.ExternalPrincipal, issueToken bool) (*dto.SocialAuthCallbackResponse, error) {
	if principal.UserID == nil {
		return nil, fmt.Errorf("外部身份尚未绑定本地用户")
	}
	return s.loginUser(*principal.UserID, false, issueToken)
}

func (s *ExternalAuthService) loginUser(userID uint, isNew, issueToken bool) (*dto.SocialAuthCallbackResponse, error) {
	userModel, err := s.userDAO.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if !userModel.CanLogin() {
		return nil, fmt.Errorf("用户未激活、已禁用或会话已撤销")
	}
	userResponse, err := s.userService.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	response := &dto.SocialAuthCallbackResponse{User: userResponse, IsNewUser: isNew}
	return response, nil
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

// CompleteTwoFactorBinding attaches a pending external identity only after the
// local account has proved its second factor; no password is retained in Redis.
func (s *ExternalAuthService) CompleteTwoFactorBinding(ctx context.Context, continuationID, csrf string, proof *TwoFactorProof) error {
	if proof == nil {
		return ErrTwoFactorRequired
	}
	temp, release, err := s.lockContinuation(ctx, continuationID, csrf)
	if err != nil {
		return err
	}
	defer release()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, proof.userID); err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, proof.userID).Error; err != nil {
			return err
		}
		if !user.CanLogin() || !proof.validFor(&user) {
			return ErrTwoFactorExpired
		}
		return s.attachProfile(tx, temp.ConnectionID, user.ID, temp.Profile)
	})
	if err != nil {
		return err
	}
	return s.consumeContinuation(ctx, continuationID)
}
