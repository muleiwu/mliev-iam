package autoload

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cnb.cool/mliev/open/go-web/pkg/container"
	envImpl "cnb.cool/mliev/open/go-web/pkg/server/env/impl"
	"github.com/muleiwu/gsr"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type databaseEnvProvider struct{}

func (databaseEnvProvider) Type() reflect.Type { return reflect.TypeFor[gsr.Enver]() }
func (databaseEnvProvider) Build() any         { return &envImpl.Env{} }
func (databaseEnvProvider) Priority() int      { return 0 }

func TestDatabaseRuntimeDriverPreservesSavedType(t *testing.T) {
	for _, tc := range []struct {
		name, saved, override, runtime string
		installed                      bool
	}{
		{"mariadb config", "mariadb", "", "mysql", true},
		{"mariadb environment", "postgresql", "mariadb", "mysql", true},
		{"environment takes precedence", "mariadb", "postgresql", "postgresql", true},
		{"mysql", "mysql", "", "mysql", true},
		{"postgresql", "postgresql", "", "postgresql", true},
		{"sqlite", "sqlite", "", "sqlite", true},
		{"pending mariadb config", "mariadb", "", "memory", false},
		{"pending mariadb environment", "mysql", "mariadb", "memory", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			container.ResetAll()
			viper.Reset()
			t.Cleanup(container.ResetAll)
			t.Cleanup(viper.Reset)
			for _, key := range []string{"DB_HOST", "DB_USERNAME", "DB_PASSWORD", "DB_DATABASE", "DB_CHARSET", "DB_FILEPATH"} {
				t.Setenv(key, "")
			}
			t.Setenv("DB_DRIVER", tc.override)
			path := filepath.Join(t.TempDir(), "config.yaml")
			original := []byte(fmt.Sprintf("app:\n  installed: %t\ndatabase:\n  driver: %s\n  host: db.example.test\n  port: 3307\n  username: iam\n  password: test-password\n  database: iam\n  charset: utf8\n  tls: true\n  filepath: config/iam.db\n", tc.installed, tc.saved))
			require.NoError(t, os.WriteFile(path, original, 0600))
			viper.SetConfigFile(path)
			require.NoError(t, viper.ReadInConfig())
			container.Register(databaseEnvProvider{})

			config := (Database{}).InitConfig()
			require.Equal(t, tc.runtime, config["database.driver"])
			host := "db.example.test"
			if tc.runtime == "sqlite" {
				host = "config/iam.db"
			}
			require.Equal(t, host, config["database.host"])
			require.Equal(t, 3307, config["database.port"])
			require.Equal(t, "iam", config["database.username"])
			require.Equal(t, "iam", config["database.dbname"])
			require.Equal(t, "test-password", config["database.password"])
			require.Equal(t, "utf8", config["database.charset"])
			require.Equal(t, true, config["database.tls"])
			require.Equal(t, tc.saved, viper.GetString("database.driver"))
			saved, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, saved)
		})
	}
}
