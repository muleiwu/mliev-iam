package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/spf13/viper"
)

type setupRouteRecorder struct {
	httpInterfaces.RouterInterface
	prefix string
	routes map[string]httpInterfaces.HandlerFunc
}

func (r *setupRouteRecorder) Group(prefix string) httpInterfaces.RouterInterface {
	return &setupRouteRecorder{prefix: r.prefix + prefix, routes: r.routes}
}
func (*setupRouteRecorder) Use(...httpInterfaces.HandlerFunc) {}
func (r *setupRouteRecorder) GET(path string, handler httpInterfaces.HandlerFunc) {
	r.routes["GET "+r.prefix+path] = handler
}
func (r *setupRouteRecorder) POST(path string, handler httpInterfaces.HandlerFunc) {
	r.routes["POST "+r.prefix+path] = handler
}

type setupEntryResponse struct {
	httpInterfaces.RouterContextInterface
	status   int
	location string
	body     []byte
}

func (*setupEntryResponse) SetHeader(string, string) {}
func (r *setupEntryResponse) Redirect(status int, location string) {
	r.status, r.location = status, location
}
func (r *setupEntryResponse) Data(status int, _ string, body []byte) { r.status, r.body = status, body }

func TestSetupPresentationAndEntryAcrossReadinessStates(t *testing.T) {
	t.Chdir(t.TempDir())
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, tc := range []struct {
		edition     string
		options     SetupOptions
		name, entry string
	}{
		{"community", SetupOptions{}, "MLiev IAM", "/admin/"},
		{"enterprise", SetupOptions{}, "MLiev IAM Enterprise", "/admin/"},
		{"education", SetupOptions{Name: "MLiev IAM 高校版", EntryPath: "/education/"}, "MLiev IAM 高校版", "/education/"},
	} {
		t.Run(tc.edition, func(t *testing.T) {
			ready := false
			installer, err := NewSetupInstaller(tc.edition, true, func() bool { return ready }, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			defer installer.Stop()
			if installer.Kit.Schema().Name != tc.name {
				t.Fatal("wrong installation branding")
			}
			router := &setupRouteRecorder{routes: map[string]httpInterfaces.HandlerFunc{}}
			installer.Register(router)
			enter := router.routes["GET /install/enter"]
			waiting := &setupEntryResponse{}
			enter(waiting)
			if waiting.status != http.StatusOK || !strings.Contains(string(waiting.body), `name="iam-setup-entry" content="`+tc.entry+`"`) || !strings.Contains(string(waiting.body), tc.name) {
				t.Fatalf("waiting page has wrong destination or branding: %s", waiting.body)
			}
			ready = true
			finished := &setupEntryResponse{}
			enter(finished)
			if finished.status != http.StatusFound || finished.location != tc.entry {
				t.Fatalf("wrong ready redirect: %+v", finished)
			}
		})
	}
}

func TestSetupEntryRejectsExternalDestinations(t *testing.T) {
	for _, entry := range []string{"https://example.test/", "//example.test/", "/\\example.test/", "/%2fexample.test/", "education/"} {
		if installer, err := NewSetupInstaller("education", true, func() bool { return true }, SetupOptions{EntryPath: entry}); err == nil {
			installer.Stop()
			t.Fatalf("accepted external or ambiguous entry %q", entry)
		}
	}
}

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
	values["admin.password"], values["admin.confirm"], values["admin.username"] = "long-password-123", "long-password-123", "invalid-account"
	if _, err := installer.Kit.Submit(context.Background(), values); err == nil {
		t.Fatal("installation accepted an account unsupported by IAM login")
	}
	if installer.Kit.Status().Phase != "pending" {
		t.Fatal("invalid account started installation")
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
