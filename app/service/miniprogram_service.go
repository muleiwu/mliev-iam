package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const miniTransactionPrefix = "miniprogram:transaction:"
const miniTransactionTTL = 15 * time.Minute

var ErrMiniTransaction = errors.New("登录事务无效、已使用或已过期，请重新登录")
var ErrMiniRateLimit = errors.New("请求过于频繁，请稍后重试")

type miniApplication struct {
	app           *model.Application
	connection    *model.IdentityConnection
	config        model.MiniProgramConfig
	appID, secret string
}

type miniTransaction struct {
	ClientID                                     string
	ConnectionID                                 uint
	AppID, Platform, Policy                      string
	OpenID, UnionID, Phone                       string
	UserID                                       uint
	UserAuthVersion                              *uint64
	Primary, CredentialStamp, State, ChallengeID string
	Scopes                                       []string
	Nonce, Issuer                                string
	Profile                                      dto.MiniProgramProfile
	Accepted                                     []string
	Approved                                     bool
	CreatedAt, ExpiresAt                         time.Time
}

type MiniProgramService struct {
	db       *gorm.DB
	redis    *redis.Client
	factors  *TwoFactorService
	exchange func(context.Context, *model.IdentityConnection, string) (*protocol.ExternalProfile, error)
	phone    func(context.Context, string, string, string, string) (string, error)
}

func NewMiniProgramService(db *gorm.DB, rdb *redis.Client, factors *TwoFactorService) *MiniProgramService {
	if factors == nil {
		factors = NewTwoFactorService(db, rdb, nil)
	}
	s := &MiniProgramService{db: db, redis: rdb, factors: factors, phone: newMiniWeChatClient(rdb).phone}
	s.exchange = func(ctx context.Context, c *model.IdentityConnection, code string) (*protocol.ExternalProfile, error) {
		adapter, err := BuildAuthenticationAdapter(db, c, rdb)
		if err != nil {
			return nil, errors.New("微信身份源配置不可用")
		}
		codeAdapter, ok := adapter.(protocol.CodeAuthenticationAdapter)
		if !ok {
			return nil, errors.New("身份源不支持小程序登录")
		}
		return codeAdapter.LoginWithCode(ctx, code)
	}
	return s
}

func miniApplicationFor(db *gorm.DB, clientID string, lock bool) (*miniApplication, error) {
	applicationQuery := db
	if lock {
		applicationQuery = applicationQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	app, err := dao.NewApplicationDAO(applicationQuery).GetByClientID(clientID)
	if err != nil || !app.IsActive || app.IsProtected || app.ProtocolType != "oidc" || app.ProtocolConfig["clientType"] != "public" {
		return nil, errors.New("小程序应用不可用")
	}
	config, err := model.DecodeMiniProgramConfig(app.ProtocolConfig)
	if err != nil || !config.Enabled {
		return nil, errors.New("应用未启用小程序登录")
	}
	var c model.IdentityConnection
	query := db
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&c, config.ConnectionID).Error; err != nil || c.ProviderType != "wechat_miniprogram" || c.Category != model.ConnectionCategorySocial || !c.IsEnabled() || !c.HasCapability(model.ConnectionCapabilityAuthentication) {
		return nil, errors.New("微信身份源不可用")
	}
	appID, secret, err := miniCredential(db, &c)
	if err != nil || appID == "" || secret == "" || len(appID) > 128 || len(miniPlatform(&c)) > 128 {
		return nil, errors.New("微信身份源配置不可用")
	}
	return &miniApplication{app: app, connection: &c, config: config, appID: appID, secret: secret}, nil
}

func (s *MiniProgramService) Config(clientID string) (*dto.MiniProgramConfigResponse, error) {
	app, err := miniApplicationFor(s.db, clientID, false)
	if err != nil {
		return nil, err
	}
	fields, err := NewSettingService(s.db).GetRegisterFieldsSettings()
	if err != nil {
		return nil, err
	}
	agreements, err := NewAgreementService(s.db).BuildClientAgreementLinks(app.app)
	if err != nil {
		return nil, err
	}
	return &dto.MiniProgramConfigResponse{Name: app.app.Name, Icon: app.app.Icon, AccountPolicy: app.config.AccountPolicy, RegisterFields: fields, Agreements: agreements}, nil
}

// Fixed windows are bounded even if a process dies between INCR and EXPIRE.
func (s *MiniProgramService) RateLimit(ctx context.Context, clientID, ip, transaction string) error {
	if s.redis == nil {
		return errors.New("登录事务服务不可用")
	}
	checks := []struct {
		key   string
		limit int64
	}{{"ip:" + miniHash(ip), 60}, {"client-ip:" + miniHash(clientID, ip), 30}}
	if transaction != "" {
		checks = append(checks, struct {
			key   string
			limit int64
		}{"transaction:" + miniHash(transaction), 20})
	}
	const script = `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],60) end; return n`
	for _, check := range checks {
		n, err := s.redis.Eval(ctx, script, []string{"miniprogram:limit:" + check.key}).Int64()
		if err != nil {
			return errors.New("登录限流服务不可用")
		}
		if n > check.limit {
			return ErrMiniRateLimit
		}
	}
	return nil
}

func (s *MiniProgramService) Login(ctx context.Context, clientID string, req dto.MiniProgramLoginRequest, issuer string) (*dto.MiniProgramResponse, error) {
	app, err := miniApplicationFor(s.db, clientID, false)
	if err != nil {
		return nil, err
	}
	rawScope := req.Scope
	if strings.TrimSpace(rawScope) == "" {
		rawScope = "openid"
	}
	scopes, err := model.ParseOIDCScopes(rawScope)
	if err != nil {
		return nil, err
	}
	if err := protocol.NewOIDCProtocolHandler(s.db).ValidateScopes(app.app, scopes); err != nil {
		return nil, err
	}
	if req.JsCode == "" || len(req.JsCode) > 512 {
		return nil, errors.New("微信登录凭证无效")
	}
	if s.redis == nil {
		return nil, ErrMiniTransaction
	}
	claimed, err := s.redis.SetNX(ctx, "miniprogram:code:"+miniHash(app.appID, req.JsCode), "1", 5*time.Minute).Result()
	if err != nil || !claimed {
		return nil, errors.New("微信登录凭证已使用，请重新调用 wx.login")
	}
	profile, err := s.exchange(ctx, app.connection, req.JsCode)
	if err != nil {
		return nil, err
	}
	tr := &miniTransaction{ClientID: clientID, ConnectionID: app.connection.ID, AppID: app.appID, Platform: miniPlatform(app.connection), Policy: app.config.AccountPolicy, Scopes: scopes, Nonce: req.Nonce, Issuer: issuer, Primary: "wechat_miniprogram", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(miniTransactionTTL)}
	if profile != nil {
		for _, id := range profile.Identifiers {
			if id.Namespace == model.IdentifierNamespaceOpenID {
				tr.OpenID = id.Value
			}
			if id.Namespace == model.IdentifierNamespaceUnionID {
				tr.UnionID = id.Value
			}
		}
	}
	if tr.OpenID == "" || len(tr.OpenID) > 255 || len(tr.UnionID) > 255 {
		return nil, errors.New("微信返回的身份标识无效")
	}
	tr.UserID, err = resolveMiniIdentity(s.db, app.connection, tr.AppID, tr.OpenID, tr.UnionID)
	if err != nil {
		recordMiniIdentityConflict(s.db, app.connection.ID, err)
		return nil, err
	}
	tr.State = "bind_required"
	if tr.UserID != 0 {
		if err := s.selectUser(tr, app); err != nil {
			return nil, err
		}
		tr.State = "consent_required"
	} else if tr.Policy == "auto_register" {
		tr.State = "phone_required"
	}
	token, err := twoFactorRandom()
	if err != nil {
		return nil, err
	}
	return s.saveResponse(ctx, token, tr)
}

func (s *MiniProgramService) selectUser(tr *miniTransaction, app *miniApplication) error {
	var user model.User
	if err := s.db.First(&user, tr.UserID).Error; err != nil || !user.CanLogin() {
		return errors.New("账号不可登录")
	}
	if _, err := NewOIDCAuthorizationService(s.db).evaluateOIDCGrant(app.app, user.ID, tr.Scopes, false); err != nil {
		return errors.New("账号无权访问该应用或请求的权限")
	}
	version := user.TwoFactorVersion
	tr.UserAuthVersion = &version
	tr.CredentialStamp = twoFactorCredentialStamp(&user)
	return nil
}

func (s *MiniProgramService) saveResponse(ctx context.Context, token string, tr *miniTransaction) (*dto.MiniProgramResponse, error) {
	ttl := time.Until(tr.ExpiresAt)
	if ttl <= 0 {
		return nil, ErrMiniTransaction
	}
	data, err := json.Marshal(tr)
	if err != nil {
		return nil, err
	}
	if err = s.redis.Set(ctx, miniTransactionPrefix+miniHash(token), data, ttl).Err(); err != nil {
		return nil, ErrMiniTransaction
	}
	response := &dto.MiniProgramResponse{State: tr.State, TransactionToken: token, ExpiresIn: int64(ttl.Seconds()), Scope: strings.Join(tr.Scopes, " ")}
	if tr.State == "two_factor_required" {
		challenge, err := s.factors.Read(ctx, tr.ChallengeID, token)
		if err != nil {
			return nil, err
		}
		response.TwoFactorChallenge, err = s.factors.View(ctx, challenge)
		if err != nil {
			return nil, err
		}
		methods := response.TwoFactorChallenge.Methods[:0]
		for _, m := range response.TwoFactorChallenge.Methods {
			if m.Method != "passkey" {
				methods = append(methods, m)
			}
		}
		response.TwoFactorChallenge.Methods = methods
		available := false
		for _, m := range methods {
			available = available || m.Available
		}
		if !available {
			return nil, errors.New("当前二次验证方式无法在小程序完成，请通过网页管理验证方式")
		}
	}
	return response, nil
}

func (s *MiniProgramService) withTransaction(ctx context.Context, clientID, token string, action func(*miniTransaction, *miniApplication) (*dto.MiniProgramResponse, error)) (*dto.MiniProgramResponse, error) {
	if s.redis == nil || len(token) != 43 {
		return nil, ErrMiniTransaction
	}
	key := miniTransactionPrefix + miniHash(token)
	lock, err := twoFactorRandom()
	if err != nil {
		return nil, err
	}
	ok, err := s.redis.SetNX(ctx, key+":lock", lock, 30*time.Second).Result()
	if err != nil || !ok {
		return nil, ErrMiniTransaction
	}
	defer s.redis.Eval(context.Background(), `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{key + ":lock"}, lock)
	raw, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, ErrMiniTransaction
	}
	var tr miniTransaction
	if json.Unmarshal(raw, &tr) != nil || tr.ClientID != clientID || !tr.ExpiresAt.After(time.Now()) {
		return nil, ErrMiniTransaction
	}
	app, err := miniApplicationFor(s.db, clientID, false)
	if err != nil {
		return nil, err
	}
	if err := checkMiniContext(&tr, app); err != nil {
		return nil, err
	}
	response, err := action(&tr, app)
	if err != nil {
		recordMiniIdentityConflict(s.db, tr.ConnectionID, err)
		return nil, err
	}
	if response != nil {
		return response, nil
	}
	return s.saveResponse(ctx, token, &tr)
}

func checkMiniContext(tr *miniTransaction, app *miniApplication) error {
	if tr.ConnectionID != app.connection.ID || tr.AppID != app.appID || tr.Platform != miniPlatform(app.connection) || tr.Policy != app.config.AccountPolicy {
		return ErrMiniTransaction
	}
	return nil
}

func (s *MiniProgramService) Phone(ctx context.Context, clientID, token, code string) (*dto.MiniProgramResponse, error) {
	return s.withTransaction(ctx, clientID, token, func(tr *miniTransaction, app *miniApplication) (*dto.MiniProgramResponse, error) {
		if tr.State != "phone_required" || tr.Policy != "auto_register" || code == "" {
			return nil, ErrMiniTransaction
		}
		phone, err := s.phone(ctx, app.appID, app.secret, tr.OpenID, code)
		if err != nil {
			return nil, err
		}
		tr.Phone = phone
		var users []model.User
		if err := s.db.Where("phone = ?", phone).Limit(2).Find(&users).Error; err != nil {
			return nil, err
		}
		if len(users) > 1 {
			return nil, &weChatConflictError{miniHash("phone", phone)}
		}
		tr.State = "profile_required"
		if len(users) == 1 {
			user := users[0]
			if !user.CanLogin() {
				return nil, errors.New("手机号对应的账号不可登录")
			}
			tr.State = "bind_required"
			if user.PhoneVerified() {
				tr.UserID = user.ID
				tr.Primary = "phone"
				if err := s.selectUser(tr, app); err != nil {
					return nil, err
				}
				tr.State = "consent_required"
			}
		}
		return nil, nil
	})
}

func (s *MiniProgramService) Bind(ctx context.Context, clientID, token, username, password, ip string) (*dto.MiniProgramResponse, error) {
	return s.withTransaction(ctx, clientID, token, func(tr *miniTransaction, app *miniApplication) (*dto.MiniProgramResponse, error) {
		if tr.UserID != 0 || (tr.State != "bind_required" && tr.State != "phone_required" && tr.State != "profile_required") {
			return nil, ErrMiniTransaction
		}
		if err := NewExternalAuthService(s.db, s.redis).CheckContinuationRateLimit(ctx, token, ip, username); err != nil {
			return nil, ErrMiniRateLimit
		}
		user, err := NewUserService(s.db).ValidateUserForLogin(username, password)
		if err != nil {
			return nil, errors.New("账号或密码错误")
		}
		tr.UserID = user.ID
		tr.Primary = "password"
		if err := s.selectUser(tr, app); err != nil {
			return nil, err
		}
		tr.State = "consent_required"
		return nil, nil
	})
}

func (s *MiniProgramService) Complete(ctx context.Context, clientID, token string, req dto.MiniProgramCompleteRequest, ip, ua string) (*dto.MiniProgramResponse, error) {
	return s.withTransaction(ctx, clientID, token, func(tr *miniTransaction, app *miniApplication) (*dto.MiniProgramResponse, error) {
		if tr.State != "profile_required" && tr.State != "consent_required" {
			return nil, ErrMiniTransaction
		}
		if !req.ApproveScopes {
			return nil, errors.New("请确认应用请求的授权范围")
		}
		if _, err := NewAgreementService(s.db).ValidateRegistrationAcceptance(&app.app.ID, req.AcceptedAgreementKeys); err != nil {
			return nil, err
		}
		tr.Profile = req.Profile
		tr.Accepted = req.AcceptedAgreementKeys
		tr.Approved = true
		if tr.UserID == 0 {
			if tr.Policy != "auto_register" || tr.Phone == "" {
				return nil, ErrMiniTransaction
			}
			if err := s.validateProfile(tr); err != nil {
				return nil, err
			}
		} else {
			var u model.User
			if err := s.db.First(&u, tr.UserID).Error; err != nil || !miniUserProofValid(tr, &u) {
				return nil, ErrMiniTransaction
			}
			if u.TwoFactorEnabled {
				challenge, err := s.factors.BeginLogin(ctx, u.ID, "miniprogram", tr.Primary, "", false, token)
				if err != nil {
					return nil, err
				}
				if challenge == nil {
					return nil, ErrMiniTransaction
				}
				tr.ChallengeID = challenge.ID
				tr.State = "two_factor_required"
				return nil, nil
			}
		}
		return s.finish(ctx, token, tr, ip, ua, nil)
	})
}

func (s *MiniProgramService) validateProfile(tr *miniTransaction) error {
	// Username is generated server-side; phone is obtained exclusively from WeChat.
	req := &dto.CreateUserRequest{Username: "generated", Phone: tr.Phone, Email: tr.Profile.Email, RealName: tr.Profile.RealName, Nickname: tr.Profile.Nickname}
	return NewSettingService(s.db).ValidateRegisterFields(req)
}

func (s *MiniProgramService) Factor(ctx context.Context, clientID, token string, req dto.MiniProgramFactorRequest, send bool, ip, ua string) (*dto.MiniProgramResponse, error) {
	return s.withTransaction(ctx, clientID, token, func(tr *miniTransaction, app *miniApplication) (*dto.MiniProgramResponse, error) {
		if tr.State != "two_factor_required" || req.Method == "passkey" {
			return nil, ErrMiniTransaction
		}
		challenge, err := s.factors.Read(ctx, tr.ChallengeID, token)
		if err != nil {
			return nil, err
		}
		if challenge.Kind != "miniprogram" || challenge.UserID != tr.UserID || challenge.PrimaryMethod != tr.Primary || tr.UserAuthVersion == nil || challenge.Version != *tr.UserAuthVersion {
			return nil, ErrMiniTransaction
		}
		if send {
			_, err = s.factors.SendCode(ctx, challenge, req.Method)
			return nil, err
		}
		proof, err := s.factors.Verify(ctx, challenge, req.Method, req.Code, ip)
		if err != nil {
			return nil, err
		}
		response, err := s.finish(ctx, token, tr, ip, ua, proof)
		// The second factor was consumed. A failed commit requires fresh authentication.
		if err != nil {
			_ = s.redis.Del(ctx, miniTransactionPrefix+miniHash(token)).Err()
		}
		return response, err
	})
}

func (s *MiniProgramService) finish(ctx context.Context, token string, tr *miniTransaction, ip, ua string, proof *TwoFactorProof) (*dto.MiniProgramResponse, error) {
	if !tr.Approved || !tr.ExpiresAt.After(time.Now()) {
		return nil, ErrMiniTransaction
	}
	var tokens *dto.TokenResponse
	isNew := tr.UserID == 0
	err := s.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		app, err := miniApplicationFor(db, tr.ClientID, true)
		if err != nil {
			return err
		}
		if err = checkMiniContext(tr, app); err != nil {
			return err
		}
		resolved, err := resolveMiniIdentity(db, app.connection, tr.AppID, tr.OpenID, tr.UnionID)
		if err != nil {
			return err
		}
		if tr.Primary == "wechat_miniprogram" && tr.UserID != 0 && resolved != tr.UserID {
			return ErrMiniTransaction
		}
		if resolved != 0 && resolved != tr.UserID {
			return &weChatConflictError{miniHash("openid", tr.AppID, tr.OpenID)}
		}
		agreements, err := NewAgreementService(db).ValidateRegistrationAcceptance(&app.app.ID, tr.Accepted)
		if err != nil {
			return err
		}
		var user model.User
		if isNew {
			if tr.Policy != "auto_register" || tr.Phone == "" {
				return ErrMiniTransaction
			}
			if err := (&MiniProgramService{db: db}).validateProfile(tr); err != nil {
				return err
			}
			var count int64
			if err := db.Model(&model.User{}).Where("phone = ?", tr.Phone).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return errors.New("手机号已注册，请重新登录关联已有账号")
			}
			if tr.Profile.Email != "" {
				if err := db.Model(&model.User{}).Where("email = ?", tr.Profile.Email).Count(&count).Error; err != nil {
					return err
				}
				if count != 0 {
					return errors.New("邮箱已存在，请绑定已有账号")
				}
			}
			username, err := NewUserService(db).GenerateAvailableUsername()
			if err != nil {
				return err
			}
			now := time.Now()
			user = model.User{Username: username, Phone: &tr.Phone, PhoneVerifiedAt: &now, RealName: tr.Profile.RealName, Nickname: tr.Profile.Nickname, Status: model.UserStatusActive, IsActivated: true}
			if tr.Profile.Email != "" {
				user.Email = &tr.Profile.Email
			}
			if err = db.Create(&user).Error; err != nil {
				return errors.New("账号创建冲突，请重新登录")
			}
		} else {
			if err := dao.LockLoginUser(db, tr.UserID); err != nil {
				return err
			}
			// A locking read also avoids a stale consistent-read snapshot on MySQL.
			if err = db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, tr.UserID).Error; err != nil {
				return err
			}
			if !miniUserProofValid(tr, &user) {
				return ErrMiniTransaction
			}
			if tr.Primary == "phone" && (user.Phone == nil || *user.Phone != tr.Phone || !user.PhoneVerified()) {
				return ErrMiniTransaction
			}
		}
		auth := NewOIDCAuthorizationService(db)
		if _, err = auth.evaluateOIDCGrant(app.app, user.ID, tr.Scopes, false); err != nil {
			return errors.New("账号无权访问该应用或请求的权限")
		}
		session, err := NewLoginSessionService(db).CreateWithProof(user.ID, tr.Primary, ip, ua, time.Now().Add(time.Hour), proof)
		if err != nil {
			return err
		}
		digest := miniHash(token)
		if err = db.Model(session).Update("native_transaction_hash", digest).Error; err != nil {
			return ErrMiniTransaction
		}
		if err = attachMiniIdentity(db, app.connection, tr.AppID, tr.OpenID, tr.UnionID, user.ID); err != nil {
			return err
		}
		if err = NewAgreementService(db).RecordAcceptances(user.ID, &app.app.ID, agreements); err != nil {
			return err
		}
		if !tr.ExpiresAt.After(time.Now()) {
			return ErrMiniTransaction
		}
		tokens, err = auth.GenerateTokens(app.app, user.ID, tr.Scopes, tr.Nonce, tr.Issuer, session.ID)
		if err != nil {
			return err
		}
		if err = NewUserService(db).RecordSuccessfulLogin(user.ID, ip); err != nil {
			return err
		}
		return db.Model(&model.ExternalPrincipal{}).Where("connection_id = ? AND user_id = ?", tr.ConnectionID, user.ID).Updates(map[string]any{"last_login_at": time.Now(), "login_count": gorm.Expr("login_count + 1")}).Error
	})
	if err != nil {
		return nil, err
	}
	_ = s.redis.Del(ctx, miniTransactionPrefix+miniHash(token)).Err()
	return &dto.MiniProgramResponse{State: "authenticated", Tokens: tokens, IsNewUser: isNew}, nil
}

// Link removal and factor changes bump TwoFactorVersion without necessarily
// changing passwords or contacts. A continuation cannot outlive that proof.
func miniUserProofValid(tr *miniTransaction, user *model.User) bool {
	return tr.UserAuthVersion != nil && *tr.UserAuthVersion == user.TwoFactorVersion &&
		user.ID == tr.UserID && user.CanLogin() && twoFactorCredentialStamp(user) == tr.CredentialStamp &&
		(user.SessionRevokedAt == nil || tr.CreatedAt.After(*user.SessionRevokedAt))
}
