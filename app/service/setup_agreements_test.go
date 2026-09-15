package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	setupkit "cnb.cool/mliev/open/setup-kit"
)

func TestSetupLicenseMatchesRepository(t *testing.T) {
	license, err := os.ReadFile(filepath.Join("..", "..", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := setupLicenseFS.ReadFile("setup_license.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, license) {
		t.Fatal("setup_license.txt must be an exact copy of the repository LICENSE")
	}
}

func TestSetupAgreementsByEdition(t *testing.T) {
	for _, edition := range []string{"", "community", "enterprise", "education", "custom"} {
		t.Run(edition, func(t *testing.T) {
			installer := setupTestInstallerForEdition(t, edition)
			schema := installer.Kit.Schema()
			if len(schema.Steps) != 3 {
				t.Fatal("agreements must not replace or add configuration steps")
			}
			if edition != "" && edition != "community" {
				if len(schema.Agreements) != 0 {
					t.Fatalf("non-community edition has agreements: %+v", schema.Agreements)
				}
				return
			}
			if len(schema.Agreements) != 1 {
				t.Fatalf("expected one community agreement, got %d", len(schema.Agreements))
			}
			license, err := setupLicenseFS.ReadFile("setup_license.txt")
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(license)
			want := setupkit.AgreementDocument{
				ID: "mit-license", Title: "开源版授权协议（MIT License）", Format: "txt",
				Content: string(license), Digest: hex.EncodeToString(digest[:]),
			}
			if schema.Agreements[0] != want {
				t.Fatalf("unexpected community agreement: %+v", schema.Agreements[0])
			}
		})
	}
}

func setupAgreementTestValues() setupkit.Values {
	return setupkit.Values{
		"database.driver": "sqlite", "database.sqlite.path": "config/iam.db", "redis.driver": "redis",
		"redis.redis.host": "localhost", "redis.redis.port": 6379, "redis.redis.db": 0,
		"admin.username": "admin", "admin.email": "admin@example.test", "admin.realName": "管理员",
		"admin.password": "long-password-123", "admin.confirm": "long-password-123",
	}
}

func TestSetupCommunityRequiresCurrentAgreementBeforeInstallation(t *testing.T) {
	const key = "setupkit.agreements.mit-license"
	for _, edition := range []string{"", "community"} {
		t.Run(edition, func(t *testing.T) {
			installer := setupTestInstallerForEdition(t, edition)
			for _, tc := range []struct {
				name    string
				receipt any
				code    string
			}{
				{"missing", nil, "validation"},
				{"unchecked", false, "validation"},
				{"boolean", true, "validation"},
				{"outdated", strings.Repeat("0", 64), "agreement_changed"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					values := setupAgreementTestValues()
					if tc.receipt != nil {
						values[key] = tc.receipt
					}
					_, err := installer.Kit.Submit(t.Context(), values)
					var pub *setupkit.PublicError
					if !errors.As(err, &pub) || pub.Code != tc.code || len(pub.Fields) != 1 || pub.Fields[key] == "" {
						t.Fatalf("expected agreement-only rejection, got %v", err)
					}
					state := installer.Kit.Status()
					if state.Phase != "pending" || state.Installed || state.InstallationID != "" || len(state.Actions) != 0 {
						t.Fatalf("invalid agreement started installation: %+v", state)
					}
					files, err := os.ReadDir("config")
					if err != nil || len(files) != 0 {
						t.Fatalf("invalid agreement wrote installation files: %v %v", files, err)
					}
				})
			}
		})
	}
}

func TestSetupAgreementValidationAllowsValidSubmissionByEdition(t *testing.T) {
	for _, edition := range []string{"", "community", "enterprise", "education", "custom"} {
		t.Run(edition, func(t *testing.T) {
			installer := setupTestInstallerForEdition(t, edition)
			values := setupAgreementTestValues()
			for _, agreement := range installer.Kit.Schema().Agreements {
				values["setupkit.agreements."+agreement.ID] = agreement.Digest
			}
			// Submit checks the context after validating input. Cancel here to
			// prove validation succeeds without contacting external services.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := installer.Kit.Submit(ctx, values); !errors.Is(err, context.Canceled) {
				t.Fatalf("valid installation input did not pass validation: %v", err)
			}
		})
	}
}
