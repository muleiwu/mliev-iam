package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/script"
	"gorm.io/gorm"
)

// ErrLegacyLoginNotMatched 表示没有任何迁移源认领该次登录。
var ErrLegacyLoginNotMatched = errors.New("no legacy source matched")

// LegacyLoginService 处理"本地查无此人"时的旧系统登录与即时迁移。
//
//   - api 模式：脚本调用旧系统 API 校验。
//   - database 模式：先查 legacy_users 未迁移记录，脚本校验其旧密码哈希。
//
// 任一源校验通过即把用户迁移进主 users 表（用本次明文密码以 bcrypt 重哈希），
// 旧哈希永不进主表。
type LegacyLoginService struct {
	db          *gorm.DB
	sourceDao   *dao.MigrationSourceDao
	legacyDao   *dao.LegacyUserDao
	userService *UserService
	engine      *script.Engine
}

func NewLegacyLoginService(db *gorm.DB) *LegacyLoginService {
	return &LegacyLoginService{
		db:          db,
		sourceDao:   dao.NewMigrationSourceDao(db),
		legacyDao:   dao.NewLegacyUserDao(db),
		userService: NewUserService(db),
		engine:      script.NewEngine(0),
	}
}

// TryLegacyLogin 按 sort 顺序遍历启用的迁移源，首个校验通过者胜出并完成迁移。
// 全部不匹配时返回 ErrLegacyLoginNotMatched。
func (s *LegacyLoginService) TryLegacyLogin(identifier, password string) (*dto.UserResponse, error) {
	sources, err := s.sourceDao.ListEnabled()
	if err != nil {
		return nil, ErrLegacyLoginNotMatched
	}

	for i := range sources {
		source := &sources[i]
		resp, matched, err := s.trySource(source, identifier, password)
		if err != nil {
			// 单个源出错不阻断其余源（记录在迁移源本身的调试里）。
			continue
		}
		if matched {
			return resp, nil
		}
	}
	return nil, ErrLegacyLoginNotMatched
}

// trySource 用单个迁移源尝试登录。matched=false 表示该源未认领/校验未过。
func (s *LegacyLoginService) trySource(source *model.MigrationSource, identifier, password string) (*dto.UserResponse, bool, error) {
	if source.LoginScript == "" {
		return nil, false, nil
	}

	input := map[string]any{
		"identifier": identifier,
		"password":   password,
	}

	var legacyUser *model.LegacyUser
	if source.Type == model.MigrationSourceTypeDatabase {
		lu, err := s.legacyDao.GetPendingByIdentifier(source.ID, identifier)
		if err != nil {
			// 该源下没有此旧用户，跳过。
			return nil, false, nil
		}
		legacyUser = lu
		input["legacyUser"] = legacyUserScriptInput(lu)
	}

	out, err := s.engine.RunForMap(context.Background(), source.LoginScript, input)
	if err != nil {
		return nil, false, err
	}
	if !asBool(out["success"]) {
		return nil, false, nil
	}

	scriptUser, _ := out["user"].(map[string]any)
	resp, err := s.promoteUser(source, legacyUser, identifier, password, scriptUser)
	if err != nil {
		if legacyUser != nil {
			_ = s.legacyDao.UpdateFields(legacyUser.ID, map[string]interface{}{
				"status":           model.LegacyUserStatusFailed,
				"migration_remark": err.Error(),
			})
		}
		return nil, false, err
	}
	return resp, true, nil
}

// promoteUser 把通过校验的旧用户写入主 users 表，并回填 legacy_users 迁移状态。
// 用本次明文密码生成 bcrypt 哈希，旧哈希不进主表。
func (s *LegacyLoginService) promoteUser(source *model.MigrationSource, legacyUser *model.LegacyUser, identifier, password string, scriptUser map[string]any) (*dto.UserResponse, error) {
	candidate := mapString(scriptUser, "username")
	if candidate == "" && legacyUser != nil {
		candidate = legacyUser.Username
	}
	if candidate == "" {
		candidate = identifier
	}
	username, err := s.resolveMigrationUsername(candidate)
	if err != nil {
		return nil, err
	}

	req := &dto.CreateUserRequest{
		Username: username,
		Email:    mapString(scriptUser, "email"),
		Phone:    mapString(scriptUser, "phone"),
		Nickname: mapString(scriptUser, "nickname"),
		RealName: mapString(scriptUser, "realName"),
		Avatar:   mapString(scriptUser, "avatar"),
		Password: password,
		Remark:   fmt.Sprintf("从迁移源「%s」迁移", source.Name),
	}
	if legacyUser != nil {
		if req.Email == "" {
			req.Email = derefString(legacyUser.Email)
		}
		if req.Phone == "" {
			req.Phone = derefString(legacyUser.Phone)
		}
		if req.Nickname == "" {
			req.Nickname = legacyUser.Nickname
		}
		// 记录旧系统身份映射：业务系统可经 OIDC userinfo 拿到 legacy_external_id 完成用户映射。
		req.Attributes = map[string]interface{}{
			"legacy_external_id": legacyUser.ExternalID,
			"legacy_source_id":   source.ID,
		}
	}

	resp, err := s.userService.CreateUser(req)
	if err != nil {
		return nil, fmt.Errorf("迁移用户落库失败: %w", err)
	}

	if legacyUser != nil {
		_ = markLegacyMigrated(s.legacyDao, legacyUser.ID, resp.ID)
	}
	return resp, nil
}

// resolveMigrationUsername 为迁移用户确定主表账户名。
// 保留符合账户格式且未被占用的旧用户名，包括纯数字。
// 含 "@" 的邮箱等不符合格式或已占用的名称改用生成账户名，仍可用手机号/邮箱登录。
func (s *LegacyLoginService) resolveMigrationUsername(candidate string) (string, error) {
	if candidate != "" && utils.ValidateUsernameFormat(candidate) == nil {
		exists, err := dao.NewUserDao(s.db).ExistsUsername(candidate)
		if err == nil && !exists {
			return candidate, nil
		}
	}
	return s.userService.GenerateAvailableUsername()
}

// legacyUserScriptInput 把旧用户记录转成登录脚本的 input.legacyUser 结构。
func legacyUserScriptInput(lu *model.LegacyUser) map[string]any {
	return map[string]any{
		"username":     lu.Username,
		"externalId":   lu.ExternalID,
		"email":        derefString(lu.Email),
		"phone":        derefString(lu.Phone),
		"nickname":     lu.Nickname,
		"passwordHash": lu.PasswordHash,
		"salt":         lu.Salt,
		"rawData":      map[string]any(lu.RawData),
	}
}

// VerifyMigratedUserPassword 用主表用户关联的已迁移旧用户凭据校验密码。
// 场景：验证码认领迁移创建的无密码账号，用户后续仍拿旧密码做密码登录——
// 旧密码哈希还留在 legacy_users 里，走迁移源脚本校验即可。
// 校验通过返回 true，调用方应把该明文密码重哈希补进主表，之后走原生路径。
func (s *LegacyLoginService) VerifyMigratedUserPassword(userID uint, identifier, password string) bool {
	legacyUser, err := s.legacyDao.GetMigratedByUserID(userID)
	if err != nil {
		return false
	}
	source, err := s.sourceDao.GetByID(legacyUser.SourceID)
	if err != nil || !source.Enabled || source.Type != model.MigrationSourceTypeDatabase || source.LoginScript == "" {
		return false
	}

	input := map[string]any{
		"identifier": identifier,
		"password":   password,
		"legacyUser": legacyUserScriptInput(legacyUser),
	}
	out, err := s.engine.RunForMap(context.Background(), source.LoginScript, input)
	if err != nil {
		return false
	}
	return asBool(out["success"])
}

// ============ 已验证联系方式认领：验证码登录 / 注册时的免密迁移 ============

// HasPendingCodeLoginUser 判断该联系方式是否命中可认领的未迁移旧用户（发验证码前的预检查用）。
func (s *LegacyLoginService) HasPendingCodeLoginUser(verificationType VerificationType, receiver string) bool {
	source, legacyUser := s.findClaimable(verificationType, receiver)
	return source != nil && legacyUser != nil
}

// findClaimable 按 sort 顺序在启用的 database 源里找首个匹配该联系方式的未迁移旧用户。
// 只按已验证归属的渠道列（email/phone）匹配。api 源没有本地旧数据，无法认领。
func (s *LegacyLoginService) findClaimable(verificationType VerificationType, receiver string) (*model.MigrationSource, *model.LegacyUser) {
	sources, err := s.sourceDao.ListEnabled()
	if err != nil {
		return nil, nil
	}
	for i := range sources {
		source := &sources[i]
		if source.Type != model.MigrationSourceTypeDatabase {
			continue
		}
		var (
			legacyUser *model.LegacyUser
			findErr    error
		)
		switch verificationType {
		case VerificationTypeEmail:
			legacyUser, findErr = s.legacyDao.GetPendingByEmail(source.ID, receiver)
		case VerificationTypePhone:
			legacyUser, findErr = s.legacyDao.GetPendingByPhone(source.ID, receiver)
		default:
			return nil, nil
		}
		if findErr == nil && legacyUser != nil {
			return source, legacyUser
		}
	}
	return nil, nil
}

// PromoteByVerifiedReceiver 用已通过验证码证明归属的邮箱/手机号认领旧用户并迁移进主表。
// 与 TryLegacyLogin 不同：此路径拿不到明文密码，验证码即归属证明，因此创建的是
// 无密码账号（PasswordHash 为空，密码登录必然失败，只能验证码登录或后续设置密码），
// 对应渠道直接标记已验证。
func (s *LegacyLoginService) PromoteByVerifiedReceiver(verificationType VerificationType, receiver string) (*dto.UserResponse, error) {
	source, legacyUser := s.findClaimable(verificationType, receiver)
	if source == nil || legacyUser == nil {
		return nil, ErrLegacyLoginNotMatched
	}

	user, err := s.buildClaimedUser(source, legacyUser, verificationType, receiver)
	if err != nil {
		return nil, err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.NewUserDao(tx).Create(user); err != nil {
			return fmt.Errorf("迁移用户落库失败: %w", err)
		}
		return markLegacyMigrated(dao.NewLegacyUserDao(tx), legacyUser.ID, user.ID)
	})
	if err != nil {
		_ = s.legacyDao.UpdateFields(legacyUser.ID, map[string]interface{}{
			"status":           model.LegacyUserStatusFailed,
			"migration_remark": err.Error(),
		})
		return nil, err
	}

	resp := dto.ConvertToUserResponse(user)
	return &resp, nil
}

// buildClaimedUser 由旧用户记录构造待落库的主表用户（认领路径）。
// 旧用户名不符合账户格式或已被占用时改用生成的账户名；
// 未经验证的另一路联系方式仅在不与主表冲突时顺带带入。
func (s *LegacyLoginService) buildClaimedUser(source *model.MigrationSource, legacyUser *model.LegacyUser, verificationType VerificationType, receiver string) (*model.User, error) {
	userDao := dao.NewUserDao(s.db)

	username, err := s.resolveMigrationUsername(legacyUser.Username)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &model.User{
		Username:    username,
		Nickname:    legacyUser.Nickname,
		Status:      model.UserStatusActive,
		IsActivated: true,
		Remark:      fmt.Sprintf("从迁移源「%s」迁移", source.Name),
		Attributes: model.JSONObject{
			"legacy_external_id": legacyUser.ExternalID,
			"legacy_source_id":   source.ID,
		},
	}

	switch verificationType {
	case VerificationTypePhone:
		phone, err := utils.NormalizePhoneToE164(receiver, utils.DefaultPhoneRegion)
		if err != nil {
			return nil, err
		}
		user.Phone = &phone
		user.PhoneVerifiedAt = &now
		if email := derefString(legacyUser.Email); email != "" {
			if exists, err := userDao.ExistsEmail(email); err == nil && !exists {
				user.Email = &email
			}
		}
	case VerificationTypeEmail:
		email := receiver
		user.Email = &email
		user.EmailVerifiedAt = &now
		if rawPhone := derefString(legacyUser.Phone); rawPhone != "" {
			if phone, err := utils.NormalizePhoneToE164(rawPhone, utils.DefaultPhoneRegion); err == nil {
				if exists, err := userDao.ExistsPhone(phone); err == nil && !exists {
					user.Phone = &phone
				}
			}
		}
	default:
		return nil, fmt.Errorf("不支持的认领方式: %s", verificationType)
	}
	return user, nil
}

// LinkOnRegistration 注册成功后，用已通过验证码验证的邮箱/手机号认领匹配的未迁移旧用户：
// 把旧系统身份映射写入用户扩展属性，并回填 legacy_users 迁移状态。无匹配时为空操作。
// 需在注册事务内调用（用事务的 *gorm.DB 构造本服务），保证关联与建号原子。
func (s *LegacyLoginService) LinkOnRegistration(userID uint, verifiedEmail, verifiedPhone string) error {
	var (
		source     *model.MigrationSource
		legacyUser *model.LegacyUser
	)
	if verifiedPhone != "" {
		source, legacyUser = s.findClaimable(VerificationTypePhone, verifiedPhone)
	}
	if legacyUser == nil && verifiedEmail != "" {
		source, legacyUser = s.findClaimable(VerificationTypeEmail, verifiedEmail)
	}
	if legacyUser == nil {
		return nil
	}

	userDao := dao.NewUserDao(s.db)
	user, err := userDao.GetByID(userID)
	if err != nil {
		return err
	}
	attrs := user.Attributes
	if attrs == nil {
		attrs = model.JSONObject{}
	}
	attrs["legacy_external_id"] = legacyUser.ExternalID
	attrs["legacy_source_id"] = source.ID
	if err := userDao.UpdateById(userID, map[string]interface{}{"attributes": attrs}); err != nil {
		return err
	}
	return markLegacyMigrated(s.legacyDao, legacyUser.ID, userID)
}

// markLegacyMigrated 回填 legacy_users 迁移状态。
func markLegacyMigrated(legacyDao *dao.LegacyUserDao, legacyID, userID uint) error {
	now := time.Now()
	return legacyDao.UpdateFields(legacyID, map[string]interface{}{
		"status":           model.LegacyUserStatusMigrated,
		"migrated_user_id": userID,
		"migrated_at":      now,
		"migration_remark": "",
	})
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		return asConfigString(v)
	}
	return ""
}

func asBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
}
