package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func setupEnvTest(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "IAM_SETUP_") {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}
}

func setupEnvInstaller(t *testing.T, edition string, installed bool) *SetupInstaller {
	t.Helper()
	installer, err := NewSetupInstaller(edition, installed, func() bool { return installed })
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, installer.Stop()) })
	return installer
}

func setupEnvFields(installer *SetupInstaller) map[string]setupkit.Field {
	fields := map[string]setupkit.Field{}
	for _, step := range installer.Kit.Schema().Steps {
		for _, field := range step.Fields {
			fields[field.Key] = field
		}
	}
	return fields
}

func TestSetupEnvironmentDefaults(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "unset", true: "empty"}[empty], func(t *testing.T) {
			setupEnvTest(t)
			if empty {
				for _, key := range []string{"DB_DRIVER", "DB_HOST", "DB_PORT", "DB_PASSWORD", "DB_FILEPATH", "REDIS_PORT", "REDIS_DB", "REDIS_TLS", "ADMIN_PASSWORD"} {
					t.Setenv("IAM_SETUP_"+key, "")
				}
			}
			fields := setupEnvFields(setupEnvInstaller(t, "community", false))
			require.Equal(t, "mysql", fields["database.driver"].Default)
			require.Equal(t, "localhost", fields["database.mysql.host"].Default)
			require.EqualValues(t, 3306, fields["database.mysql.port"].Default)
			require.Equal(t, "mliev_iam", fields["database.mysql.database"].Default)
			require.Equal(t, filepath.Join("config", "mliev_iam.db"), fields["database.sqlite.path"].Default)
			require.EqualValues(t, 6379, fields["redis.redis.port"].Default)
			require.Nil(t, fields["admin.password"].Default)
			require.Nil(t, fields["database.mysql.password"].Default)
			require.Empty(t, fields["admin.username"].HelpKey)
			require.Contains(t, fields["admin.username"].Help, "纯数字账户保留前导零")
		})
	}
}

func TestSetupEnvironmentAcrossEditionsAndDrivers(t *testing.T) {
	for _, edition := range []string{"community", "enterprise", "education"} {
		for _, driver := range []string{"mysql", "mariadb", "postgresql", "sqlite"} {
			t.Run(edition+"/"+driver, func(t *testing.T) {
				setupEnvTest(t)
				for key, value := range map[string]string{
					"DB_DRIVER": driver, "DB_HOST": "db.example.test", "DB_PORT": "5439", "DB_USERNAME": "installer",
					"DB_PASSWORD": "database-test-secret", "DB_DATABASE": "iam_preset", "DB_TLS": "true", "DB_CHARSET": "utf8",
					"DB_FILEPATH": "config/preset.db", "REDIS_HOST": "cache.example.test", "REDIS_PORT": "6388",
					"REDIS_USERNAME": "cache_user", "REDIS_PASSWORD": "redis-test-secret", "REDIS_DB": "2", "REDIS_TLS": "1",
					"ADMIN_USERNAME": "000123", "ADMIN_EMAIL": "preset@example.test", "ADMIN_REAL_NAME": "预填管理员",
					"ADMIN_PHONE": "13800000000", "ADMIN_PASSWORD": "administrator-test-secret",
				} {
					t.Setenv("IAM_SETUP_"+key, value)
				}
				installer := setupEnvInstaller(t, edition, false)
				fields := setupEnvFields(installer)
				require.Equal(t, driver, fields["database.driver"].Default)
				for _, network := range []string{"mysql", "mariadb", "postgresql"} {
					prefix := "database." + network + "."
					for key, want := range map[string]any{"host": "db.example.test", "port": float64(5439), "username": "installer", "password": "database-test-secret", "database": "iam_preset", "tls": true} {
						require.Equal(t, want, fields[prefix+key].Default, prefix+key)
					}
				}
				require.Equal(t, "utf8", fields["database.mysql.charset"].Default)
				require.Equal(t, "utf8", fields["database.mariadb.charset"].Default)
				for key, want := range map[string]any{
					"database.sqlite.path": "config/preset.db", "redis.redis.host": "cache.example.test", "redis.redis.port": float64(6388),
					"redis.redis.username": "cache_user", "redis.redis.password": "redis-test-secret", "redis.redis.db": float64(2), "redis.redis.tls": true,
					"admin.username": "000123", "admin.email": "preset@example.test", "admin.realName": "预填管理员", "admin.phone": "13800000000",
					"admin.password": "administrator-test-secret", "admin.confirm": "administrator-test-secret",
				} {
					require.Equal(t, want, fields[key].Default, key)
				}
				schema, err := json.Marshal(installer.Kit.Schema())
				require.NoError(t, err)
				require.NotContains(t, string(schema), "IAM_SETUP_")
				t.Setenv("IAM_SETUP_DB_HOST", "changed.example.test")
				require.Equal(t, "db.example.test", setupEnvFields(installer)["database.mysql.host"].Default)
				require.Equal(t, "pending", installer.Kit.Status().Phase)
			})
		}
	}
}

func TestSetupEnvironmentInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"DB_DRIVER", "oracle"}, {"DB_PORT", "65536"}, {"DB_PORT", "not-a-number"}, {"DB_TLS", "enabled"},
		{"DB_CHARSET", "unknown"}, {"REDIS_DB", "-1"}, {"REDIS_PORT", "0"}, {"REDIS_TLS", "invalid"},
		{"ADMIN_USERNAME", "invalid-name"}, {"ADMIN_EMAIL", "invalid-email"}, {"ADMIN_PASSWORD", "short"},
		{"ADMIN_PHONE", strings.Repeat("1", 21)}, {"ADMIN_REAL_NAME", strings.Repeat("名", 51)},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			setupEnvTest(t)
			t.Setenv("IAM_SETUP_DB_DRIVER", "sqlite") // Inactive network presets are also validated.
			t.Setenv("IAM_SETUP_"+tc.key, tc.value)
			installer, err := NewSetupInstaller("enterprise", false, func() bool { return false })
			if installer != nil {
				t.Cleanup(func() { require.NoError(t, installer.Stop()) })
			}
			require.ErrorContains(t, err, "IAM_SETUP_"+tc.key)
			if tc.key == "ADMIN_PASSWORD" {
				require.NotContains(t, err.Error(), tc.value)
			}
			require.NoFileExists(t, filepath.Join("config", "config.yaml"))
		})
	}
}

func TestSetupEnvironmentUserOverridesAndClearing(t *testing.T) {
	setupEnvTest(t)
	for key, value := range map[string]string{
		"DB_DRIVER": "sqlite", "DB_FILEPATH": "config/preset.db", "ADMIN_USERNAME": "preset_admin",
		"ADMIN_EMAIL": "preset@example.test", "ADMIN_REAL_NAME": "预填管理员", "ADMIN_PASSWORD": "administrator-test-secret",
	} {
		t.Setenv("IAM_SETUP_"+key, value)
	}
	installer := setupEnvInstaller(t, "enterprise", false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Cancellation occurs after normalization and before checks/actions.
	_, err := installer.Kit.Submit(ctx, setupkit.Values{})
	require.ErrorIs(t, err, context.Canceled)
	_, err = installer.Kit.Submit(ctx, setupkit.Values{"admin.username": "replacement", "admin.phone": "", "database.mysql.port": -1})
	require.ErrorIs(t, err, context.Canceled)
	for _, key := range []string{"admin.username", "admin.email", "admin.password", "admin.confirm", "admin.realName", "database.sqlite.path"} {
		_, err := installer.Kit.Submit(ctx, setupkit.Values{key: ""})
		var public *setupkit.PublicError
		require.ErrorAs(t, err, &public)
		require.Contains(t, public.Fields, key)
		require.NotContains(t, public.Error(), "administrator-test-secret")
	}
	_, err = installer.Kit.Submit(ctx, setupkit.Values{"admin.password": "changed-password-123"})
	var public *setupkit.PublicError
	require.ErrorAs(t, err, &public)
	require.Contains(t, public.Fields, "admin.confirm")
	result, err := installer.Kit.Check(t.Context(), "database", setupkit.Values{"database.sqlite.path": "config/replacement.db", "database.mysql.port": -1})
	require.NoError(t, err)
	require.True(t, result.OK)
	require.NoFileExists(t, "config/preset.db")
	require.NoFileExists(t, "config/replacement.db")
	community := setupEnvInstaller(t, "community", false)
	_, err = community.Kit.Submit(ctx, setupkit.Values{})
	require.ErrorAs(t, err, &public)
	require.Contains(t, public.Fields, "setupkit.agreements.mit-license")
}

func TestSetupInstalledIgnoresEnvironment(t *testing.T) {
	setupEnvTest(t)
	for key, value := range map[string]string{
		"DB_DRIVER": "unsupported", "DB_PORT": "invalid", "REDIS_TLS": "invalid", "REDIS_PASSWORD": "redis-test-secret",
		"DB_PASSWORD": "database-test-secret", "ADMIN_USERNAME": "invalid-name", "ADMIN_PASSWORD": "short",
	} {
		t.Setenv("IAM_SETUP_"+key, value)
	}
	installer := setupEnvInstaller(t, "enterprise", true)
	require.True(t, installer.Kit.Status().Installed)
	fields := setupEnvFields(installer)
	for _, key := range []string{"database.mysql.password", "redis.redis.password", "admin.password", "admin.confirm"} {
		require.Nil(t, fields[key].Default)
	}
	require.Equal(t, "mysql", fields["database.driver"].Default)
}
