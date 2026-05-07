package migrations

import "embed"

// FS 打包了所有方言的 goose SQL 迁移。Runner 根据 database.driver 选对应子目录。
//
//go:embed all:mysql all:postgresql all:sqlite
var FS embed.FS
