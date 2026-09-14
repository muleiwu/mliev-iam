package installation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	setupkit "cnb.cool/mliev/open/setup-kit"
	"github.com/google/uuid"
)

// SetupStore uses the selected configuration file as the completion boundary.
// Runtime configuration remains an immutable startup snapshot until restart.
type SetupStore struct {
	setupkit.FileStore
	InstalledAtStartup bool
}

func (s *SetupStore) Load(ctx context.Context) (setupkit.State, error) {
	state, err := s.FileStore.Load(ctx)
	if err != nil {
		return state, err
	}
	if s.InstalledAtStartup {
		state.Installed, state.Phase = true, "installed"
	}
	if state.Installed {
		return state, nil
	}
	legacy, err := os.ReadFile(filepath.Join(filepath.Dir(s.ConfigPath), "installation-id"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	id := strings.TrimSpace(string(legacy))
	if _, err := uuid.Parse(id); err != nil {
		return state, fmt.Errorf("安装标识文件无效")
	}
	if state.InstallationID != "" && state.InstallationID != id {
		return state, fmt.Errorf("安装进度与原安装标识不一致，请恢复原配置")
	}
	state.InstallationID = id
	return state, nil
}
