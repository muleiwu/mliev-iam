package config

import (
	"reflect"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
)

type setupAssembly struct {
	edition string
	options service.SetupOptions
}

func (*setupAssembly) Type() reflect.Type { return reflect.TypeFor[*service.SetupInstaller]() }
func (*setupAssembly) DependsOn() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[readyState]()}
}
func (s *setupAssembly) Assembly() (any, error) {
	cfg := helper.GetConfig()
	return service.NewSetupInstaller(s.edition, cfg.GetBool("app.installed", false), func() bool { return cfg.GetBool("app.ready", false) }, s.options)
}
