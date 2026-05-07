package autoload

import (
	"embed"
)

type StaticFs struct {
}

func (receiver StaticFs) InitConfig() map[string]any {
	return map[string]any{
		// 实际静态资源在 main.go 中通过 cmd.WithTemplateFs / cmd.WithWebStaticFs 注入，
		// 并由 cmd.Start 写入 config.Set("static.fs", staticFs) 覆盖此占位。
		"static.fs": map[string]embed.FS{},
	}
}
