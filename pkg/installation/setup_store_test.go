package installation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/google/uuid"
)

func TestSetupStoreMigratesLegacyIDAndPreservesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	id := uuid.NewString()
	os.WriteFile(filepath.Join(dir, "installation-id"), []byte(id), 0600)
	os.WriteFile(path, []byte("app:\n  installed: false\nhttp:\n  addr: ':9090'\n"), 0600)
	store := &SetupStore{FileStore: setupkit.FileStore{ConfigPath: path}}
	ctx := context.Background()
	state, err := store.Load(ctx)
	if err != nil || state.InstallationID != id {
		t.Fatalf("legacy ID not reused: %+v %v", state, err)
	}
	state.Phase = "failed"
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, setupkit.Values{"database.driver": "sqlite", "database.filepath": "iam.db", "redis.tls": true}); err != nil {
		t.Fatal(err)
	}
	state, err = store.Load(ctx)
	if err != nil || !state.Installed || state.Phase != "installed" {
		t.Fatalf("commit must override stale journal: %+v %v", state, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ":9090") {
		t.Fatal("unrelated configuration lost")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration must be private")
	}
}

func TestSetupStoreRefusesConflictingInstallationAndCancelledWrite(t *testing.T) {
	dir := t.TempDir()
	store := &SetupStore{FileStore: setupkit.FileStore{ConfigPath: filepath.Join(dir, "config.yaml")}}
	ctx := context.Background()
	os.WriteFile(filepath.Join(dir, "installation-id"), []byte(uuid.NewString()), 0600)
	if err := store.SaveState(ctx, setupkit.State{InstallationID: uuid.NewString(), Phase: "failed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx); err == nil {
		t.Fatal("conflicting installation IDs accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Commit(cancelled, setupkit.Values{}); err == nil {
		t.Fatal("cancelled commit succeeded")
	}
	if _, err := os.Stat(store.ConfigPath); !os.IsNotExist(err) {
		t.Fatal("cancelled commit wrote config")
	}
}

func TestSetupStoreHonorsEnvironmentInstalledState(t *testing.T) {
	store := &SetupStore{FileStore: setupkit.FileStore{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}, InstalledAtStartup: true}
	state, err := store.Load(context.Background())
	if err != nil || !state.Installed {
		t.Fatalf("environment deployment reopened: %+v %v", state, err)
	}
}
