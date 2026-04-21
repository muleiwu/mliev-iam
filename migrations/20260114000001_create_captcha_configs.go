package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upCreateCaptchaConfigs, downCreateCaptchaConfigs)
}

func upCreateCaptchaConfigs(ctx context.Context, tx *sql.Tx) error {
	log.Println("[create_captcha_configs] 开始创建 captcha_configs 表...")

	// 检查表是否已存在
	var tableName string
	checkQuery := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = DATABASE() 
		AND table_name = 'captcha_configs'
	`
	err := tx.QueryRowContext(ctx, checkQuery).Scan(&tableName)

	// 如果查询出错且不是"没有行"的错误，则返回错误
	if err != nil && err != sql.ErrNoRows {
		// 可能是 SQLite 或其他数据库，尝试其他检查方式
		log.Printf("[create_captcha_configs] information_schema 查询失败，尝试直接查询表: %v", err)

		// 尝试直接查询表
		var count int
		countQuery := "SELECT COUNT(*) FROM captcha_configs LIMIT 1"
		err = tx.QueryRowContext(ctx, countQuery).Scan(&count)
		if err == nil {
			log.Println("[create_captcha_configs] 表 captcha_configs 已存在，跳过创建")
			return nil
		}
	}

	// 如果表已存在
	if tableName == "captcha_configs" {
		log.Println("[create_captcha_configs] 表 captcha_configs 已存在，跳过创建")
		return nil
	}

	log.Println("[create_captcha_configs] 表不存在，开始创建...")

	// 创建 captcha_configs 表
	createTableQuery := `
		CREATE TABLE IF NOT EXISTS captcha_configs (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL COMMENT '配置名称',
			provider VARCHAR(50) NOT NULL COMMENT '提供商类型',
			enabled BOOLEAN DEFAULT TRUE COMMENT '是否启用',
			params JSON COMMENT '参数配置',
			remark VARCHAR(255) DEFAULT '' COMMENT '备注',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
			deleted_at TIMESTAMP NULL DEFAULT NULL COMMENT '删除时间',
			INDEX idx_provider (provider),
			INDEX idx_deleted_at (deleted_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='验证码配置表';
	`

	if _, err := tx.ExecContext(ctx, createTableQuery); err != nil {
		return fmt.Errorf("创建 captcha_configs 表失败: %w", err)
	}

	log.Println("[create_captcha_configs] 表 captcha_configs 创建成功")
	return nil
}

func downCreateCaptchaConfigs(ctx context.Context, tx *sql.Tx) error {
	log.Println("[create_captcha_configs] 回滚：删除 captcha_configs 表...")

	dropTableQuery := "DROP TABLE IF EXISTS captcha_configs"
	if _, err := tx.ExecContext(ctx, dropTableQuery); err != nil {
		return fmt.Errorf("删除 captcha_configs 表失败: %w", err)
	}

	log.Println("[create_captcha_configs] 表 captcha_configs 已删除")
	return nil
}
