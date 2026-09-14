package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/spf13/viper"
)

func setupTestInstaller(t *testing.T) *SetupInstaller {
	t.Helper()
	t.Chdir(t.TempDir())
	viper.Reset()
	t.Cleanup(viper.Reset)
	installer, err := NewSetupInstaller("enterprise", false, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := installer.Stop(); err != nil {
			t.Error(err)
		}
	})
	return installer
}

func TestSetupSchemaAndPasswordValidation(t *testing.T) {
	installer := setupTestInstaller(t)
	schema := installer.Kit.Schema()
	if schema.BasePath != "/install" || schema.RedirectURL != "/install/enter" || len(schema.Steps) != 3 {
		t.Fatalf("unexpected schema: %+v", schema)
	}
	fields := map[string]setupkit.Field{}
	for _, step := range schema.Steps {
		for _, field := range step.Fields {
			fields[field.Key] = field
		}
	}
	for _, key := range []string{"database.mysql.tls", "database.postgresql.tls", "redis.redis.tls", "redis.redis.username", "admin.realName", "admin.phone"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing field %s", key)
		}
	}
	if fields["admin.password"].MinLength != 12 || !fields["admin.realName"].Required || fields["admin.phone"].Required {
		t.Fatal("administrator policy changed")
	}
	values := setupkit.Values{"database.driver": "sqlite", "database.sqlite.path": "config/iam.db", "redis.driver": "redis",
		"redis.redis.host": "localhost", "redis.redis.port": 6379, "redis.redis.db": 0,
		"admin.username": "admin", "admin.email": "admin@example.test", "admin.realName": "管理员",
		"admin.password": "short", "admin.confirm": "short"}
	if _, err := installer.Kit.Submit(context.Background(), values); err == nil {
		t.Fatal("short password accepted")
	}
	if installer.Kit.Status().Phase != "pending" {
		t.Fatal("invalid input started installation")
	}
}

func TestSetupSQLiteProbeDoesNotCreateDatabase(t *testing.T) {
	installer := setupTestInstaller(t)
	target := filepath.Join("config", "probe.db")
	values := setupkit.Values{"database.driver": "sqlite", "database.sqlite.path": target}
	result, err := installer.Kit.Check(context.Background(), "database", values)
	if err != nil || !result.OK {
		t.Fatalf("SQLite check failed: %+v %v", result, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("connection check created business database")
	}
	values["database.sqlite.path"] = ":memory:"
	result, err = installer.Kit.Check(context.Background(), "database", values)
	if err == nil && result.OK {
		t.Fatal("memory database accepted")
	}
}

func TestSetupRecoversInterruptedProgressWithoutReplayingInput(t *testing.T) {
	installer := setupTestInstaller(t)
	if err := installer.Stop(); err != nil {
		t.Fatal(err)
	}
	store := &setupkit.FileStore{ConfigPath: filepath.Join("config", "config.yaml")}
	id := "interrupted-installation"
	if err := store.SaveState(context.Background(), setupkit.State{
		InstallationID: id, Phase: "running", Stage: "initialize",
		Actions: []setupkit.ActionProgress{{ID: "initialize", Title: "初始化系统与管理员", Status: "running"}},
	}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewSetupInstaller("enterprise", false, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop()
	state := restarted.Kit.Status()
	if state.Phase != "failed" || state.Installed || state.InstallationID != id || state.Actions[0].Status != "failed" {
		t.Fatalf("interrupted state was not safely recovered: %+v", state)
	}
	if _, err := os.Stat("config/config.yaml"); !os.IsNotExist(err) {
		t.Fatal("recovery replayed installation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := withSetupDatabase(ctx, setupkit.ConnectionValues{Driver: "sqlite", Values: setupkit.Values{"path": "config/cancelled.db"}}, nil); err == nil {
		t.Fatal("cancelled action connected")
	}
	if _, err := os.Stat("config/cancelled.db"); !os.IsNotExist(err) {
		t.Fatal("cancelled action created database")
	}
}
