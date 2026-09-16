package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	verificationLegacyUsername = "legacy_verified"
	verificationLegacyPassword = "legacy-password"
	verificationLegacyEmail    = "legacy@example.test"
	verificationLegacyPhone    = "13800138000"
	verificationMigratedPhone  = "+8613800138000"
)

// Both source modes run a real login script and reach migration through password login.
func newLegacyVerificationFixture(t *testing.T, mode model.MigrationSourceType, fields string) (*gorm.DB, *model.LegacyUser) {
	t.Helper()
	db := newMigratedTestDB(t)
	var script string
	if mode == model.MigrationSourceTypeDatabase {
		script = `
			var lu = input.legacyUser;
			if (passhash.sha256(input.password + lu.salt) !== lu.passwordHash) {
				return { success: false };
			}
		`
	} else {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var credentials struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&credentials) != nil ||
				credentials.Username != verificationLegacyUsername || credentials.Password != verificationLegacyPassword {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": verificationLegacyUsername,
				"email":    verificationLegacyEmail,
				"phone":    verificationLegacyPhone,
				"rawData":  map[string]any{"email_verified": true, "phone_verified": true},
			})
		}))
		t.Cleanup(server.Close)
		script = fmt.Sprintf(`
			var resp = http.request({
				method: "POST", url: %q,
				body: { username: input.identifier, password: input.password }
			});
			if (resp.status !== 200) return { success: false };
			var lu = resp.json;
		`, server.URL)
	}
	script += `return { success: true, user: { username: lu.username, ` + fields + ` } };`
	source := &model.MigrationSource{
		UUID:        "verification-source",
		Name:        "验证状态迁移源",
		Type:        mode,
		Enabled:     true,
		LoginScript: script,
	}
	require.NoError(t, dao.NewMigrationSourceDao(db).Create(source))
	if mode == model.MigrationSourceTypeAPI {
		return db, nil
	}
	email, phone := verificationLegacyEmail, verificationLegacyPhone
	legacy := &model.LegacyUser{
		SourceID:     source.ID,
		Username:     verificationLegacyUsername,
		Email:        &email,
		Phone:        &phone,
		Salt:         "salt",
		PasswordHash: legacySha256(verificationLegacyPassword, "salt"),
		RawData:      model.JSONObject{"email_verified": true, "phone_verified": true},
		Status:       model.LegacyUserStatusPending,
	}
	require.NoError(t, dao.NewLegacyUserDao(db).Create(legacy))
	return db, legacy
}

func assertMigratedContactVerification(t *testing.T, db *gorm.DB, response *dto.UserResponse, emailVerified, phoneVerified bool) {
	t.Helper()
	stored, err := dao.NewUserDao(db).GetByID(response.ID)
	require.NoError(t, err)
	require.Equal(t, verificationLegacyEmail, response.Email)
	require.Equal(t, verificationMigratedPhone, response.Phone)
	require.Equal(t, emailVerified, stored.EmailVerified())
	require.Equal(t, phoneVerified, stored.PhoneVerified())
	require.Equal(t, emailVerified, response.EmailVerified)
	require.Equal(t, phoneVerified, response.PhoneVerified)
	require.Equal(t, emailVerified || phoneVerified, response.IsVerified)
	for _, timestamps := range [][2]*time.Time{
		{stored.EmailVerifiedAt, response.EmailVerifiedAt},
		{stored.PhoneVerifiedAt, response.PhoneVerifiedAt},
	} {
		if timestamps[0] == nil {
			require.Nil(t, timestamps[1])
			continue
		}
		require.NotNil(t, timestamps[1])
		require.WithinDuration(t, *timestamps[0], *timestamps[1], 0)
		require.WithinDuration(t, time.Now(), *timestamps[0], time.Minute)
	}
	svc := NewUserService(db)
	for _, contact := range []struct {
		kind     VerificationType
		receiver string
		verified bool
	}{
		{VerificationTypeEmail, response.Email, emailVerified},
		{VerificationTypePhone, response.Phone, phoneVerified},
	} {
		for _, check := range []func(VerificationType, string) (*dto.UserResponse, error){
			svc.GetVerifiedUserForCodeLogin,
			svc.GetVerifiedUserForPasswordReset,
		} {
			user, err := check(contact.kind, contact.receiver)
			if contact.verified {
				require.NoError(t, err)
				require.Equal(t, response.ID, user.ID)
			} else {
				require.ErrorContains(t, err, "未验证")
			}
		}
	}
}

func TestLegacyLoginContactVerification(t *testing.T) {
	for _, mode := range []model.MigrationSourceType{model.MigrationSourceTypeDatabase, model.MigrationSourceTypeAPI} {
		t.Run(string(mode), func(t *testing.T) {
			for _, tc := range []struct {
				name          string
				flags         string
				emailVerified bool
				phoneVerified bool
			}{
				{"omitted", "", false, false},
				{"null", "emailVerified: null, phoneVerified: null", false, false},
				{"false", "emailVerified: false, phoneVerified: false", false, false},
				{"email only", "emailVerified: true", true, false},
				{"phone only", "phoneVerified: true", false, true},
				{"both from old fields", "emailVerified: lu.rawData.email_verified === true, phoneVerified: lu.rawData.phone_verified === true", true, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					db, legacy := newLegacyVerificationFixture(t, mode, "email: lu.email, phone: lu.phone, "+tc.flags)
					response, err := NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, verificationLegacyPassword)
					require.NoError(t, err)
					assertMigratedContactVerification(t, db, response, tc.emailVerified, tc.phoneVerified)
					if legacy != nil {
						migrated, err := dao.NewLegacyUserDao(db).GetByID(legacy.ID)
						require.NoError(t, err)
						require.Equal(t, model.LegacyUserStatusMigrated, migrated.Status)
						require.Equal(t, &response.ID, migrated.MigratedUserID)
						require.NotNil(t, migrated.MigratedAt)
					}
				})
			}
		})
	}
}

func TestLegacyLoginRejectsInvalidContactVerification(t *testing.T) {
	for _, field := range []string{"email", "phone"} {
		for _, tc := range []struct {
			name   string
			fields string
		}{
			{"string true", `email: lu.email, phone: lu.phone, ` + field + `Verified: "true"`},
			{"string false", `email: lu.email, phone: lu.phone, ` + field + `Verified: "false"`},
			{"number", `email: lu.email, phone: lu.phone, ` + field + `Verified: 1`},
			{"object", `email: lu.email, phone: lu.phone, ` + field + `Verified: {}`},
			{"array", `email: lu.email, phone: lu.phone, ` + field + `Verified: []`},
			{"missing contact", field + `Verified: true`},
			{"null contact", field + `: null, ` + field + `Verified: true`},
			{"empty contact", field + `: "", ` + field + `Verified: true`},
			{"blank contact", field + `: "   ", ` + field + `Verified: true`},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				// The legacy record has both contacts: a missing explicit value must not verify a fallback value.
				db, legacy := newLegacyVerificationFixture(t, model.MigrationSourceTypeDatabase, tc.fields)
				response, err := NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, verificationLegacyPassword)
				require.Error(t, err)
				require.Nil(t, response)
				var count int64
				require.NoError(t, db.Unscoped().Model(&model.User{}).Count(&count).Error)
				require.Zero(t, count)
				failed, err := dao.NewLegacyUserDao(db).GetByID(legacy.ID)
				require.NoError(t, err)
				require.Equal(t, model.LegacyUserStatusFailed, failed.Status)
				require.Contains(t, failed.MigrationRemark, "user."+field+"Verified")
				require.Nil(t, failed.MigratedUserID)
				require.Nil(t, failed.MigratedAt)
			})
		}
	}
}

func TestLegacyLoginContactVerificationRollsBack(t *testing.T) {
	for _, table := range []string{"users", "legacy_users"} {
		t.Run(table, func(t *testing.T) {
			db, legacy := newLegacyVerificationFixture(t, model.MigrationSourceTypeDatabase,
				"email: lu.email, phone: lu.phone, emailVerified: true, phoneVerified: true")
			const callback = "test:deny_migration_update"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != table {
					return
				}
				if table == "legacy_users" {
					updates, ok := tx.Statement.Dest.(map[string]interface{})
					if !ok || updates["status"] != model.LegacyUserStatusMigrated {
						return // Allow the failure reason to be recorded after rollback.
					}
				}
				tx.AddError(errors.New("injected migration write failure"))
			}))
			response, err := NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, verificationLegacyPassword)
			require.NoError(t, db.Callback().Update().Remove(callback))
			require.Error(t, err)
			require.Nil(t, response)
			var count int64
			require.NoError(t, db.Unscoped().Model(&model.User{}).Count(&count).Error)
			require.Zero(t, count)
			failed, err := dao.NewLegacyUserDao(db).GetByID(legacy.ID)
			require.NoError(t, err)
			require.Equal(t, model.LegacyUserStatusFailed, failed.Status)
			require.Contains(t, failed.MigrationRemark, "injected migration write failure")
			require.Nil(t, failed.MigratedUserID)
			require.Nil(t, failed.MigratedAt)

			// Retrying after storage recovers creates one complete, verified account.
			response, err = NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, verificationLegacyPassword)
			require.NoError(t, err)
			assertMigratedContactVerification(t, db, response, true, true)
		})
	}
}

func TestLegacyLoginVerificationRequiresCorrectPassword(t *testing.T) {
	for _, mode := range []model.MigrationSourceType{model.MigrationSourceTypeDatabase, model.MigrationSourceTypeAPI} {
		t.Run(string(mode), func(t *testing.T) {
			db, legacy := newLegacyVerificationFixture(t, mode,
				"email: lu.email, phone: lu.phone, emailVerified: true, phoneVerified: true")
			_, err := NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, "wrong-password")
			require.Error(t, err)
			var count int64
			require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
			require.Zero(t, count)
			if legacy != nil {
				unchanged, err := dao.NewLegacyUserDao(db).GetByID(legacy.ID)
				require.NoError(t, err)
				require.Equal(t, model.LegacyUserStatusPending, unchanged.Status)
			}
		})
	}
}

func TestLegacyPasswordBackfillPreservesContactVerification(t *testing.T) {
	for _, kind := range []VerificationType{VerificationTypeEmail, VerificationTypePhone} {
		t.Run(string(kind), func(t *testing.T) {
			phoneClaim := kind == VerificationTypePhone
			fields := fmt.Sprintf("email: lu.email, phone: lu.phone, emailVerified: %t, phoneVerified: %t", phoneClaim, !phoneClaim)
			db, _ := newLegacyVerificationFixture(t, model.MigrationSourceTypeDatabase, fields)
			receiver := verificationLegacyEmail
			if phoneClaim {
				receiver = verificationMigratedPhone
			}
			claimed, err := NewLegacyLoginService(db).PromoteByVerifiedReceiver(kind, receiver)
			require.NoError(t, err)
			assertMigratedContactVerification(t, db, claimed, !phoneClaim, phoneClaim)
			response, err := NewUserService(db).ValidateUserForLogin(verificationLegacyUsername, verificationLegacyPassword)
			require.NoError(t, err)
			assertMigratedContactVerification(t, db, response, !phoneClaim, phoneClaim)
			if phoneClaim {
				require.WithinDuration(t, *claimed.PhoneVerifiedAt, *response.PhoneVerifiedAt, 0)
			} else {
				require.WithinDuration(t, *claimed.EmailVerifiedAt, *response.EmailVerifiedAt, 0)
			}
		})
	}
}
