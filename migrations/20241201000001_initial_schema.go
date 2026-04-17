package migrations

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"cnb.cool/mliev/iam/mliev-iam/internal/helper"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upInitialSchema, downInitialSchema)
}

// upInitialSchema 初始化 schema
//
// 行为：
//   - 仅在 database.auto_migrate=true 时调用 GORM AutoMigrate 建表（开发环境或首次安装场景）
//   - 生产环境（默认 database.auto_migrate=false）预期表已由安装向导或运维提前建好，
//     本迁移退化为校验表存在的哨兵。若表缺失，返回错误终止启动，避免在服务已上线后
//     才被动地发现 schema 不一致。
func upInitialSchema(ctx context.Context, tx *sql.Tx) error {
	db := getExternalDB()
	if db == nil {
		log.Println("[initial_schema] 外部 GORM 连接未设置，跳过（goose 由 install 流程触发）")
		return nil
	}

	allowAutoMigrate := helper.GetHelper().GetConfig().GetBool("database.auto_migrate", false)
	if !allowAutoMigrate {
		if tx != nil {
			var exists int
			// 以 users 表是否存在作为 schema 健康探针；不存在时直接中止，交由人工处理
			err := tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM information_schema.tables WHERE table_name = 'users'").Scan(&exists)
			if err == nil && exists == 0 {
				return errors.New("[initial_schema] schema 未初始化且 database.auto_migrate=false；请运行安装向导或手工执行 DDL")
			}
		}
		log.Println("[initial_schema] database.auto_migrate=false，跳过 AutoMigrate，假定 schema 已就绪")
		return nil
	}

	log.Println("[initial_schema] database.auto_migrate=true，执行 GORM AutoMigrate（仅推荐开发/首次安装）")
	models := helper.GetHelper().GetConfig().Get("database.migration", []any{}).([]any)
	if err := db.AutoMigrate(models...); err != nil {
		return err
	}
	log.Println("[initial_schema] AutoMigrate 完成")
	return nil
}

func downInitialSchema(ctx context.Context, tx *sql.Tx) error {
	return nil
}
