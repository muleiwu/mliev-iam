package service

import (
	"context"
	"errors"
	"testing"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/stretchr/testify/require"
)

func TestSetupDatabaseChoicesAcrossEditions(t *testing.T) {
	for _, edition := range []string{"", "community", "enterprise", "education", "custom"} {
		t.Run(edition, func(t *testing.T) {
			installer := setupTestInstallerForEdition(t, edition)
			fields := map[string]setupkit.Field{}
			for _, field := range installer.Kit.Schema().Steps[0].Fields {
				fields[field.Key] = field
			}
			selector := fields["database.driver"]
			require.Equal(t, "mysql", selector.Default)
			require.Equal(t, []setupkit.Choice{
				{Value: "mysql", Label: "MySQL"},
				{Value: "mariadb", Label: "MariaDB"},
				{Value: "postgresql", Label: "PostgreSQL"},
				{Value: "sqlite", Label: "SQLite"},
			}, selector.Options)
			for _, driver := range []string{"mysql", "mariadb"} {
				prefix := "database." + driver + "."
				for _, key := range []string{"host", "port", "username", "password", "database", "charset", "tls"} {
					require.Contains(t, fields, prefix+key)
				}
				require.Equal(t, "localhost", fields[prefix+"host"].Default)
				require.EqualValues(t, 3306, fields[prefix+"port"].Default)
				require.Equal(t, "root", fields[prefix+"username"].Default)
				require.Equal(t, "mliev_iam", fields[prefix+"database"].Default)
				require.Equal(t, "utf8mb4", fields[prefix+"charset"].Default)
				require.Equal(t, false, fields[prefix+"tls"].Default)
				require.True(t, fields[prefix+"tls"].Advanced)
				require.Equal(t, "password", fields[prefix+"password"].Type)
			}
		})
	}
}

func TestSetupValidatesOnlySelectedDatabaseFields(t *testing.T) {
	installer := setupTestInstaller(t)
	for _, driver := range []string{"mysql", "mariadb", "postgresql", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			values := setupAgreementTestValues()
			values["database.driver"] = driver
			// A user may switch types after entering invalid connection values.
			for _, inactive := range []string{"mysql", "mariadb", "postgresql"} {
				values["database."+inactive+".port"] = -1
			}
			prefix := "database." + driver + "."
			if driver != "sqlite" {
				values[prefix+"port"] = 3306
				values[prefix+"username"] = "installer"
				values[prefix+"database"] = "iam"
			}
			// Cancellation happens after validation, before installation starts.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := installer.Kit.Submit(ctx, values)
			require.ErrorIs(t, err, context.Canceled)

			if driver != "sqlite" {
				values[prefix+"port"] = -1
				_, err = installer.Kit.Submit(ctx, values)
				var public *setupkit.PublicError
				require.True(t, errors.As(err, &public), "expected field validation, got %v", err)
				require.Equal(t, "validation", public.Code)
				require.Contains(t, public.Fields, prefix+"port")
			}
			require.Equal(t, "pending", installer.Kit.Status().Phase)
		})
	}
}
