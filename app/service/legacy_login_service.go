package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
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
		input["legacyUser"] = map[string]any{
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
	username := mapString(scriptUser, "username")
	if username == "" && legacyUser != nil {
		username = legacyUser.Username
	}
	if username == "" {
		username = identifier
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
		now := time.Now()
		_ = s.legacyDao.UpdateFields(legacyUser.ID, map[string]interface{}{
			"status":           model.LegacyUserStatusMigrated,
			"migrated_user_id": resp.ID,
			"migrated_at":      now,
			"migration_remark": "",
		})
	}
	return resp, nil
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
