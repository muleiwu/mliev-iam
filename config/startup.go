package config

import (
	"context"
	"fmt"
	"reflect"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/migrations"
	"cnb.cool/mliev/iam/mliev-iam/pkg/extension"
	"cnb.cool/mliev/iam/mliev-iam/pkg/installation"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	redisAssembly "cnb.cool/mliev/open/go-web/pkg/server/redis/assembly"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type bootstrapRedis struct{ redisAssembly.Redis }

func (r *bootstrapRedis) Assembly() (any, error) {
	if !helper.GetConfig().GetBool("app.installed", false) {
		return (*redis.Client)(nil), nil
	}
	return r.Redis.Assembly()
}

type readyState struct{}
type startupChecks struct{ modules []extension.Module }

func (*startupChecks) Type() reflect.Type { return reflect.TypeFor[readyState]() }
func (*startupChecks) DependsOn() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[gsr.Provider](), reflect.TypeFor[gsr.Logger](), reflect.TypeFor[*gorm.DB](), reflect.TypeFor[*redis.Client](), reflect.TypeFor[gsr.Cacher]()}
}

// Checks run as an assembly because go-web aborts on assembly errors, whereas
// a Server.Run error is merely logged and would otherwise still start HTTP.
func (s *startupChecks) Assembly() (any, error) {
	cfg := helper.GetConfig()
	cfg.Set("app.ready", false)
	if !cfg.GetBool("app.installed", false) {
		return readyState{}, nil
	}
	if passkey := service.ReadPasskeyConfig(); passkey.Enabled {
		if _, err := passkey.WebAuthn(); err != nil {
			return nil, err
		}
	}
	if err := installation.EnsureCredentialKey(true); err != nil {
		return nil, err
	}
	if err := (&migrations.Server{}).Run(); err != nil {
		return nil, err
	}
	if err := (&service.SystemApplicationServer{}).Run(); err != nil {
		return nil, err
	}
	for _, module := range s.modules {
		if module.Startup != nil {
			if err := module.Startup(context.Background(), helper.GetDatabase(), cfg.GetString("database.driver", "mysql")); err != nil {
				return nil, fmt.Errorf("启动模块 %s: %w", module.Name, err)
			}
		}
	}
	cfg.Set("app.ready", true)
	return readyState{}, nil
}
