package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/migrations"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newMigratedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := migrations.RunGooseMigrations(sqlDB, "sqlite", nil); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func legacySha256(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(password + salt))
	return hex.EncodeToString(h.Sum(nil))
}

// TestLegacyLoginDatabaseModePromotesUser 覆盖"数据库导入 → 校验旧哈希 → 自动迁移进主表"全链路。
func TestLegacyLoginDatabaseModePromotesUser(t *testing.T) {
	db := newMigratedTestDB(t)

	const (
		username = "legacy_alice"
		password = "s3cr3t"
		salt     = "abc123"
	)

	// 1) 建一个 database 类型迁移源，登录脚本用 sha256(password+salt) 校验旧密码。
	sourceDao := dao.NewMigrationSourceDao(db)
	source := &model.MigrationSource{
		UUID:    "test-uuid-1",
		Name:    "旧系统",
		Type:    model.MigrationSourceTypeDatabase,
		Enabled: true,
		LoginScript: `
			var lu = input.legacyUser;
			if (passhash.sha256(input.password + lu.salt) === lu.passwordHash) {
				return { success: true, user: { username: lu.username, email: lu.email } };
			}
			return { success: false };
		`,
	}
	if err := sourceDao.Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// 2) 模拟一次性导入：插入一条 legacy_users 记录（旧 SHA256+salt 哈希）。
	email := "alice@old.example"
	legacyDao := dao.NewLegacyUserDao(db)
	lu := &model.LegacyUser{
		SourceID:     source.ID,
		Username:     username,
		Email:        &email,
		PasswordHash: legacySha256(password, salt),
		Salt:         salt,
		Status:       model.LegacyUserStatusPending,
	}
	if err := legacyDao.Create(lu); err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	// 3) 走登录主入口（本地查无此人 → 触发旧系统登录钩子）。
	userSvc := NewUserService(db)
	resp, err := userSvc.ValidateUserForLogin(username, password)
	if err != nil {
		t.Fatalf("expected legacy login success, got error: %v", err)
	}
	if resp == nil || resp.Username != username {
		t.Fatalf("unexpected user response: %+v", resp)
	}

	// 4) 用户应已落库到主 users 表，且密码用 argon2id 重哈希（再次登录走标准路径）。
	promoted, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload promoted user: %v", err)
	}
	if !strings.HasPrefix(promoted.PasswordHash, "$argon2id$") {
		t.Fatalf("expected argon2id hash after promotion, got %q", promoted.PasswordHash)
	}
	resp2, err := userSvc.ValidateUserForLogin(username, password)
	if err != nil {
		t.Fatalf("expected native login after promotion, got: %v", err)
	}
	if resp2.ID != resp.ID {
		t.Fatalf("expected same user id, got %d vs %d", resp2.ID, resp.ID)
	}

	// 5) legacy_users 记录应标记为已迁移并回填 migrated_user_id。
	migrated, err := legacyDao.GetByID(lu.ID)
	if err != nil {
		t.Fatalf("reload legacy user: %v", err)
	}
	if migrated.Status != model.LegacyUserStatusMigrated {
		t.Fatalf("expected status migrated, got %s", migrated.Status)
	}
	if migrated.MigratedUserID == nil || *migrated.MigratedUserID != resp.ID {
		t.Fatalf("expected migrated_user_id=%d, got %v", resp.ID, migrated.MigratedUserID)
	}

	// 6) 错误密码应失败（无源认领）。
	if _, err := userSvc.ValidateUserForLogin("ghost_user", "whatever"); err == nil {
		t.Fatal("expected error for unknown user, got nil")
	}
}

// TestLegacyLoginPhoneCandidateMatch 覆盖"旧系统存裸号 → 用 E.164 登录 → 候选集命中并迁移"，
// 同时验证 promoteUser 会把 legacy_external_id 写入用户扩展属性。
func TestLegacyLoginPhoneCandidateMatch(t *testing.T) {
	db := newMigratedTestDB(t)

	const (
		barePhone  = "13800138000"
		password   = "p@ssw0rd"
		salt       = "saltxyz"
		externalID = "ext-999"
	)

	// database 类型迁移源，登录脚本用 sha256(password+salt) 校验旧密码。
	sourceDao := dao.NewMigrationSourceDao(db)
	source := &model.MigrationSource{
		UUID:    "test-uuid-phone",
		Name:    "旧系统-手机",
		Type:    model.MigrationSourceTypeDatabase,
		Enabled: true,
		LoginScript: `
			var lu = input.legacyUser;
			if (passhash.sha256(input.password + lu.salt) === lu.passwordHash) {
				return { success: true, user: { username: lu.username } };
			}
			return { success: false };
		`,
	}
	if err := sourceDao.Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// 旧系统里手机号是裸号格式（无国家码）。
	phone := barePhone
	legacyDao := dao.NewLegacyUserDao(db)
	lu := &model.LegacyUser{
		SourceID:     source.ID,
		Username:     "legacy_phone_user",
		Phone:        &phone,
		ExternalID:   externalID,
		PasswordHash: legacySha256(password, salt),
		Salt:         salt,
		Status:       model.LegacyUserStatusPending,
	}
	if err := legacyDao.Create(lu); err != nil {
		t.Fatalf("create legacy user: %v", err)
	}

	// 用 E.164 形式登录（前端规范化后的形式），应命中裸号候选匹配。
	userSvc := NewUserService(db)
	resp, err := userSvc.ValidateUserForLogin("+86"+barePhone, password)
	if err != nil {
		t.Fatalf("expected legacy phone login success, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected user response, got nil")
	}

	// 迁移后的用户应带上旧系统身份映射属性。
	promoted, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload promoted user: %v", err)
	}
	if got, _ := promoted.Attributes["legacy_external_id"].(string); got != externalID {
		t.Fatalf("expected legacy_external_id=%q, got %v", externalID, promoted.Attributes["legacy_external_id"])
	}

	// legacy_users 记录应标记为已迁移。
	migrated, err := legacyDao.GetByID(lu.ID)
	if err != nil {
		t.Fatalf("reload legacy user: %v", err)
	}
	if migrated.Status != model.LegacyUserStatusMigrated {
		t.Fatalf("expected status migrated, got %s", migrated.Status)
	}
}

// newPendingLegacyPhoneUser 建一个启用的 database 迁移源和一条带裸号手机的 pending 旧用户。
func newPendingLegacyPhoneUser(t *testing.T, db *gorm.DB, barePhone, externalID string) *model.LegacyUser {
	t.Helper()
	source := &model.MigrationSource{
		UUID:    "test-uuid-claim",
		Name:    "旧系统-认领",
		Type:    model.MigrationSourceTypeDatabase,
		Enabled: true,
	}
	if err := dao.NewMigrationSourceDao(db).Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	phone := barePhone
	lu := &model.LegacyUser{
		SourceID:   source.ID,
		Username:   "legacy_claim_user",
		Phone:      &phone,
		ExternalID: externalID,
		Status:     model.LegacyUserStatusPending,
	}
	if err := dao.NewLegacyUserDao(db).Create(lu); err != nil {
		t.Fatalf("create legacy user: %v", err)
	}
	return lu
}

// TestPromoteByVerifiedPhone 覆盖"验证码登录：主表无此手机号 → 认领旧用户免密自动注册"。
func TestPromoteByVerifiedPhone(t *testing.T) {
	db := newMigratedTestDB(t)

	const (
		barePhone  = "15385390860"
		e164Phone  = "+8615385390860"
		externalID = "ext-claim-1"
	)
	lu := newPendingLegacyPhoneUser(t, db, barePhone, externalID)

	legacySvc := NewLegacyLoginService(db)
	if !legacySvc.HasPendingCodeLoginUser(VerificationTypePhone, e164Phone) {
		t.Fatal("expected claimable legacy user for E.164 phone")
	}
	if !legacySvc.HasPendingCodeLoginUser(VerificationTypePhone, barePhone) {
		t.Fatal("expected claimable legacy user for bare phone")
	}

	resp, err := legacySvc.PromoteByVerifiedReceiver(VerificationTypePhone, e164Phone)
	if err != nil {
		t.Fatalf("promote by verified phone: %v", err)
	}

	promoted, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload promoted user: %v", err)
	}
	if promoted.Phone == nil || *promoted.Phone != e164Phone {
		t.Fatalf("expected phone %s, got %v", e164Phone, promoted.Phone)
	}
	if promoted.PhoneVerifiedAt == nil {
		t.Fatal("expected phone marked verified")
	}
	if promoted.PasswordHash != "" {
		t.Fatalf("expected passwordless account, got hash %q", promoted.PasswordHash)
	}
	if got, _ := promoted.Attributes["legacy_external_id"].(string); got != externalID {
		t.Fatalf("expected legacy_external_id=%q, got %v", externalID, promoted.Attributes["legacy_external_id"])
	}

	// 无密码账号不允许密码登录。
	if _, err := NewUserService(db).ValidateUserForLogin(e164Phone, "anything"); err == nil {
		t.Fatal("expected password login to fail for passwordless claimed user")
	}

	// legacy 记录应已迁移，且不再可认领。
	migrated, err := dao.NewLegacyUserDao(db).GetByID(lu.ID)
	if err != nil {
		t.Fatalf("reload legacy user: %v", err)
	}
	if migrated.Status != model.LegacyUserStatusMigrated {
		t.Fatalf("expected status migrated, got %s", migrated.Status)
	}
	if migrated.MigratedUserID == nil || *migrated.MigratedUserID != resp.ID {
		t.Fatalf("expected migrated_user_id=%d, got %v", resp.ID, migrated.MigratedUserID)
	}
	if legacySvc.HasPendingCodeLoginUser(VerificationTypePhone, e164Phone) {
		t.Fatal("expected no claimable legacy user after promotion")
	}
}

// TestLinkOnRegistration 覆盖"新用户注册（手机验证码通过）→ 自动关联 pending 旧用户"。
func TestLinkOnRegistration(t *testing.T) {
	db := newMigratedTestDB(t)

	const (
		barePhone  = "15385390860"
		e164Phone  = "+8615385390860"
		externalID = "ext-link-1"
	)
	lu := newPendingLegacyPhoneUser(t, db, barePhone, externalID)

	resp, err := NewUserService(db).CreateUser(&dto.CreateUserRequest{
		Username: "fresh_user",
		Password: "p@ssw0rd",
		Phone:    e164Phone,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := NewLegacyLoginService(db).LinkOnRegistration(resp.ID, "", e164Phone); err != nil {
		t.Fatalf("link on registration: %v", err)
	}

	linked, err := dao.NewUserDao(db).GetByID(resp.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got, _ := linked.Attributes["legacy_external_id"].(string); got != externalID {
		t.Fatalf("expected legacy_external_id=%q, got %v", externalID, linked.Attributes["legacy_external_id"])
	}

	migrated, err := dao.NewLegacyUserDao(db).GetByID(lu.ID)
	if err != nil {
		t.Fatalf("reload legacy user: %v", err)
	}
	if migrated.Status != model.LegacyUserStatusMigrated {
		t.Fatalf("expected status migrated, got %s", migrated.Status)
	}
	if migrated.MigratedUserID == nil || *migrated.MigratedUserID != resp.ID {
		t.Fatalf("expected migrated_user_id=%d, got %v", resp.ID, migrated.MigratedUserID)
	}
}
