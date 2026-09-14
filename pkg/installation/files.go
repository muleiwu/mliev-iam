// Package installation manages the durable files needed to bootstrap IAM.
package installation

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

func ConfigPath() string {
	if path := viper.ConfigFileUsed(); path != "" {
		return path
	}
	return filepath.Join("config", "config.yaml")
}

// AtomicWrite persists a complete file on the same filesystem, with restricted
// permissions. A failed write never truncates the previous configuration.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".iam-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		defer d.Close()
		_ = d.Sync()
	}
	return nil
}

func PersistSettings(values map[string]any) error {
	next := viper.New()
	if err := next.MergeConfigMap(viper.AllSettings()); err != nil {
		return err
	}
	for key, value := range values {
		next.Set(key, value)
	}
	data, err := yaml.Marshal(next.AllSettings())
	if err != nil {
		return err
	}
	path := ConfigPath()
	if err := AtomicWrite(path, data); err != nil {
		return err
	}
	viper.SetConfigFile(path)
	for key, value := range values {
		viper.Set(key, value)
	}
	return nil
}

// EnsureCredentialKey never regenerates a lost key on an installed deployment.
func EnsureCredentialKey(installed bool) error {
	if os.Getenv(credentialcrypto.EnvironmentKey) != "" || os.Getenv(credentialcrypto.EnvironmentKeys) != "" {
		_, err := credentialcrypto.FromEnvironment()
		return err
	}
	path := filepath.Join(filepath.Dir(ConfigPath()), "identity-credentials.key")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !installed {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		data = []byte(base64.StdEncoding.EncodeToString(key))
		if err = createSecret(path, data); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			data, err = os.ReadFile(path)
		}
	}
	if err != nil {
		return fmt.Errorf("凭据主密钥不可用，请恢复环境变量或 %s: %w", path, err)
	}
	encoded := strings.TrimSpace(string(data))
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return errors.New("凭据主密钥文件格式错误")
	}
	return os.Setenv(credentialcrypto.EnvironmentKey, encoded)
}

func createSecret(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// PendingID binds a retried installation to the initial data it created. It
// cannot authorize taking over a database initialized by another installation.
func PendingID() (string, error) {
	path := filepath.Join(filepath.Dir(ConfigPath()), "installation-id")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte(uuid.NewString())
		if err = createSecret(path, data); errors.Is(err, os.ErrExist) {
			data, err = os.ReadFile(path)
		}
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if _, err := uuid.Parse(value); err != nil {
		return "", errors.New("安装标识文件无效")
	}
	return value, nil
}
