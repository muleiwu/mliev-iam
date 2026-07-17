package main

import (
	"embed"

	"cnb.cool/mliev/iam/mliev-iam/config"
	"cnb.cool/mliev/open/go-web/cmd"
	"github.com/muleiwu/gomander"
)

//go:embed all:templates
var templateFS embed.FS

//go:embed all:static
var staticFs embed.FS

func main() {
	gomander.Run(func() {
		cmd.Start(
			cmd.WithTemplateFs(templateFS),
			cmd.WithWebStaticFs(staticFs),
			cmd.WithApp(config.App{}),
		)
	})
}
