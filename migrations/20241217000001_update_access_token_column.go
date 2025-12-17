package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upUpdateAccessTokenColumn, downUpdateAccessTokenColumn)
}

func upUpdateAccessTokenColumn(ctx context.Context, tx *sql.Tx) error {
	log.Println("[update_access_token_column] 开始升级 access_token 存储方案...")

	// 1. 删除旧的唯一索引（如果存在）
	log.Println("[update_access_token_column] 删除旧的唯一索引...")
	_, err := tx.ExecContext(ctx, `ALTER TABLE oidc_tokens DROP INDEX access_token`)
	if err != nil {
		// 索引可能不存在，继续执行
		log.Printf("[update_access_token_column] 删除索引警告（可忽略）: %v", err)
	}

	// 2. 修改 access_token 列为 TEXT 类型
	log.Println("[update_access_token_column] 修改 access_token 列为 TEXT...")
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE oidc_tokens 
		MODIFY COLUMN access_token TEXT NOT NULL
	`)
	if err != nil {
		log.Printf("[update_access_token_column] 修改 access_token 列失败: %v", err)
		return err
	}

	// 3. 添加 access_token_hash 列
	log.Println("[update_access_token_column] 添加 access_token_hash 列...")
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE oidc_tokens 
		ADD COLUMN access_token_hash VARCHAR(64) NOT NULL DEFAULT '' AFTER access_token
	`)
	if err != nil {
		log.Printf("[update_access_token_column] 添加 access_token_hash 列失败: %v", err)
		return err
	}

	// 4. 为现有记录计算 hash（如果有数据）
	log.Println("[update_access_token_column] 为现有记录计算 hash...")
	rows, err := tx.QueryContext(ctx, `SELECT id, access_token FROM oidc_tokens WHERE access_token_hash = ''`)
	if err != nil {
		log.Printf("[update_access_token_column] 查询现有记录失败: %v", err)
		return err
	}
	defer rows.Close()

	updateStmt, err := tx.PrepareContext(ctx, `UPDATE oidc_tokens SET access_token_hash = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	count := 0
	for rows.Next() {
		var id int64
		var accessToken string
		if err := rows.Scan(&id, &accessToken); err != nil {
			log.Printf("[update_access_token_column] 扫描记录失败: %v", err)
			continue
		}

		// 计算 SHA256 hash
		hash := sha256.Sum256([]byte(accessToken))
		hashStr := hex.EncodeToString(hash[:])

		// 更新记录
		if _, err := updateStmt.ExecContext(ctx, hashStr, id); err != nil {
			log.Printf("[update_access_token_column] 更新记录 %d 失败: %v", id, err)
			continue
		}
		count++
	}
	log.Printf("[update_access_token_column] 已为 %d 条记录计算 hash", count)

	// 5. 为 access_token_hash 添加唯一索引
	log.Println("[update_access_token_column] 为 access_token_hash 添加唯一索引...")
	_, err = tx.ExecContext(ctx, `
		CREATE UNIQUE INDEX idx_access_token_hash ON oidc_tokens(access_token_hash)
	`)
	if err != nil {
		log.Printf("[update_access_token_column] 添加索引失败: %v", err)
		return err
	}

	log.Println("[update_access_token_column] 升级完成")
	return nil
}

func downUpdateAccessTokenColumn(ctx context.Context, tx *sql.Tx) error {
	log.Println("[update_access_token_column] 开始回滚...")

	// 1. 删除 hash 索引
	log.Println("[update_access_token_column] 删除 access_token_hash 索引...")
	_, err := tx.ExecContext(ctx, `DROP INDEX idx_access_token_hash ON oidc_tokens`)
	if err != nil {
		log.Printf("[update_access_token_column] 删除索引失败: %v", err)
		return err
	}

	// 2. 删除 access_token_hash 列
	log.Println("[update_access_token_column] 删除 access_token_hash 列...")
	_, err = tx.ExecContext(ctx, `ALTER TABLE oidc_tokens DROP COLUMN access_token_hash`)
	if err != nil {
		log.Printf("[update_access_token_column] 删除列失败: %v", err)
		return err
	}

	// 3. 回滚 access_token 为 VARCHAR(255)
	// 注意：如果有超过 255 字符的数据，这步会失败
	log.Println("[update_access_token_column] 回滚 access_token 为 VARCHAR(255)...")
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE oidc_tokens 
		MODIFY COLUMN access_token VARCHAR(255) NOT NULL
	`)
	if err != nil {
		log.Printf("[update_access_token_column] 回滚 access_token 列失败: %v", err)
		return err
	}

	// 4. 重新添加唯一索引
	log.Println("[update_access_token_column] 重新添加 access_token 唯一索引...")
	_, err = tx.ExecContext(ctx, `
		CREATE UNIQUE INDEX access_token ON oidc_tokens(access_token)
	`)
	if err != nil {
		log.Printf("[update_access_token_column] 添加索引失败: %v", err)
		return err
	}

	log.Println("[update_access_token_column] 回滚完成")
	return nil
}
