package installation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func installationEnvironment(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("IDENTITY_CREDENTIALS_KEY", "")
	t.Setenv("IDENTITY_CREDENTIALS_KEYS", "")
}

func TestConfigSavedOnlyToSelectedPath(t *testing.T) {
	installationEnvironment(t)
	if err := PersistSettings(map[string]any{"app.installed": true, "database.driver": "sqlite"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("config.yaml"); !os.IsNotExist(err) {
		t.Fatal("unexpected shadow configuration")
	}
	data, err := os.ReadFile(filepath.Join("config", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "installed: true") {
		t.Fatal("installation state not persisted")
	}
	info, _ := os.Stat(ConfigPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions are not restricted")
	}
	loaded := viper.New()
	loaded.SetConfigFile(ConfigPath())
	if err := loaded.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if !loaded.GetBool("app.installed") {
		t.Fatal("state lost across reload")
	}
}

func TestExistingConfigPathPreservedAndFailedWriteDoesNotChangeState(t *testing.T) {
	installationEnvironment(t)
	viper.SetConfigFile("custom.yaml")
	if err := PersistSettings(map[string]any{"app.installed": false}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("cannot-replace-directory", 0700); err != nil {
		t.Fatal(err)
	}
	viper.SetConfigFile("cannot-replace-directory")
	if err := PersistSettings(map[string]any{"app.installed": true}); err == nil {
		t.Fatal("expected write failure")
	}
	if viper.GetBool("app.installed") {
		t.Fatal("failed write changed installation state")
	}
	data, _ := os.ReadFile("custom.yaml")
	if !strings.Contains(string(data), "installed: false") {
		t.Fatal("previous configuration changed")
	}
}

func TestCredentialKeySurvivesRestartAndIsNeverRegenerated(t *testing.T) {
	installationEnvironment(t)
	if err := EnsureCredentialKey(false); err != nil {
		t.Fatal(err)
	}
	first := os.Getenv("IDENTITY_CREDENTIALS_KEY")
	if first == "" {
		t.Fatal("key was not generated")
	}
	t.Setenv("IDENTITY_CREDENTIALS_KEY", "")
	if err := EnsureCredentialKey(true); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("IDENTITY_CREDENTIALS_KEY") != first {
		t.Fatal("key changed across restart")
	}
	t.Setenv("IDENTITY_CREDENTIALS_KEY", "")
	if err := os.Remove("config/identity-credentials.key"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCredentialKey(true); err == nil {
		t.Fatal("missing installed key must fail")
	}
	if _, err := os.Stat("config/identity-credentials.key"); !os.IsNotExist(err) {
		t.Fatal("lost key was regenerated")
	}
}
