package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddUserUUID, downAddUserUUID)
}

// DBType represents the database type
type DBType int

const (
	DBTypeUnknown DBType = iota
	DBTypeMySQL
	DBTypePostgres
	DBTypeSQLite
)

// detectDBType detects the database type from the connection
func detectDBType(ctx context.Context, tx *sql.Tx) DBType {
	// Try PostgreSQL version query
	var version string
	err := tx.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	if err == nil {
		versionLower := strings.ToLower(version)
		if strings.Contains(versionLower, "postgresql") {
			return DBTypePostgres
		}
		if strings.Contains(versionLower, "mysql") || strings.Contains(versionLower, "mariadb") {
			return DBTypeMySQL
		}
	}

	// Try SQLite version query
	err = tx.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version)
	if err == nil {
		return DBTypeSQLite
	}

	// Try MySQL specific query
	err = tx.QueryRowContext(ctx, "SELECT @@version").Scan(&version)
	if err == nil {
		return DBTypeMySQL
	}

	return DBTypeUnknown
}

// columnExists checks if a column exists in a table (multi-database compatible)
func columnExistsMultiDB(ctx context.Context, tx *sql.Tx, dbType DBType, tableName, columnName string) (bool, error) {
	var query string
	var args []interface{}

	switch dbType {
	case DBTypePostgres:
		query = `
			SELECT COUNT(*) FROM information_schema.columns 
			WHERE table_name = $1 AND column_name = $2
		`
		args = []interface{}{tableName, columnName}
	case DBTypeMySQL:
		query = `
			SELECT COUNT(*) FROM information_schema.COLUMNS 
			WHERE TABLE_SCHEMA = DATABASE() 
			AND TABLE_NAME = ? 
			AND COLUMN_NAME = ?
		`
		args = []interface{}{tableName, columnName}
	case DBTypeSQLite:
		// SQLite: use PRAGMA table_info
		rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dfltValue interface{}
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
				return false, err
			}
			if name == columnName {
				return true, nil
			}
		}
		return false, rows.Err()
	default:
		return false, fmt.Errorf("unsupported database type")
	}

	var count int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// indexExists checks if an index exists on a table (multi-database compatible)
func indexExistsMultiDB(ctx context.Context, tx *sql.Tx, dbType DBType, tableName, indexName string) (bool, error) {
	var query string
	var args []interface{}

	switch dbType {
	case DBTypePostgres:
		query = `
			SELECT COUNT(*) FROM pg_indexes 
			WHERE tablename = $1 AND indexname = $2
		`
		args = []interface{}{tableName, indexName}
	case DBTypeMySQL:
		query = `
			SELECT COUNT(*) FROM information_schema.STATISTICS 
			WHERE TABLE_SCHEMA = DATABASE() 
			AND TABLE_NAME = ? 
			AND INDEX_NAME = ?
		`
		args = []interface{}{tableName, indexName}
	case DBTypeSQLite:
		query = `
			SELECT COUNT(*) FROM sqlite_master 
			WHERE type = 'index' AND tbl_name = ? AND name = ?
		`
		args = []interface{}{tableName, indexName}
	default:
		return false, fmt.Errorf("unsupported database type")
	}

	var count int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func upAddUserUUID(ctx context.Context, tx *sql.Tx) error {
	log.Println("[add_user_uuid] Starting migration to add UUID column to users table...")

	// Detect database type
	dbType := detectDBType(ctx, tx)
	log.Printf("[add_user_uuid] Detected database type: %v\n", dbType)

	// Step 1: Check if uuid column already exists
	exists, err := columnExistsMultiDB(ctx, tx, dbType, "users", "uuid")
	if err != nil {
		return fmt.Errorf("failed to check if uuid column exists: %w", err)
	}

	if !exists {
		log.Println("[add_user_uuid] Step 1: Adding uuid column (nullable)...")
		var addColumnSQL string
		switch dbType {
		case DBTypePostgres:
			addColumnSQL = `ALTER TABLE users ADD COLUMN uuid VARCHAR(36)`
		case DBTypeMySQL:
			addColumnSQL = `ALTER TABLE users ADD COLUMN uuid VARCHAR(36)`
		case DBTypeSQLite:
			addColumnSQL = `ALTER TABLE users ADD COLUMN uuid TEXT`
		default:
			addColumnSQL = `ALTER TABLE users ADD COLUMN uuid VARCHAR(36)`
		}
		_, err = tx.ExecContext(ctx, addColumnSQL)
		if err != nil {
			return fmt.Errorf("failed to add uuid column: %w", err)
		}
	} else {
		log.Println("[add_user_uuid] Step 1: uuid column already exists, skipping...")
	}

	// Step 2: Generate UUIDv7 for all existing users without UUID
	log.Println("[add_user_uuid] Step 2: Generating UUIDv7 for existing users...")
	rows, err := tx.QueryContext(ctx, `SELECT id FROM users WHERE uuid IS NULL OR uuid = ''`)
	if err != nil {
		return fmt.Errorf("failed to query users without UUID: %w", err)
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan user id: %w", err)
		}
		userIDs = append(userIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating user rows: %w", err)
	}

	// Update each user with a new UUIDv7
	for _, userID := range userIDs {
		newUUID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate UUIDv7 for user %d: %w", userID, err)
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET uuid = ? WHERE id = ?`, newUUID.String(), userID)
		if err != nil {
			// Try PostgreSQL placeholder style
			_, err = tx.ExecContext(ctx, `UPDATE users SET uuid = $1 WHERE id = $2`, newUUID.String(), userID)
			if err != nil {
				return fmt.Errorf("failed to update UUID for user %d: %w", userID, err)
			}
		}
	}
	log.Printf("[add_user_uuid] Generated UUIDs for %d existing users\n", len(userIDs))

	// Step 3: Set NOT NULL constraint (skip for SQLite as it doesn't support MODIFY COLUMN)
	if dbType != DBTypeSQLite {
		log.Println("[add_user_uuid] Step 3: Setting NOT NULL constraint...")
		var modifySQL string
		switch dbType {
		case DBTypePostgres:
			modifySQL = `ALTER TABLE users ALTER COLUMN uuid SET NOT NULL`
		case DBTypeMySQL:
			modifySQL = `ALTER TABLE users MODIFY COLUMN uuid VARCHAR(36) NOT NULL`
		}
		if modifySQL != "" {
			_, err = tx.ExecContext(ctx, modifySQL)
			if err != nil {
				return fmt.Errorf("failed to set NOT NULL constraint: %w", err)
			}
		}
	} else {
		log.Println("[add_user_uuid] Step 3: Skipping NOT NULL constraint for SQLite...")
	}

	// Step 4: Create unique index if not exists
	idxExists, err := indexExistsMultiDB(ctx, tx, dbType, "users", "idx_users_uuid")
	if err != nil {
		return fmt.Errorf("failed to check if index exists: %w", err)
	}

	if !idxExists {
		log.Println("[add_user_uuid] Step 4: Creating unique index...")
		_, err = tx.ExecContext(ctx, `CREATE UNIQUE INDEX idx_users_uuid ON users(uuid)`)
		if err != nil {
			return fmt.Errorf("failed to create unique index: %w", err)
		}
	} else {
		log.Println("[add_user_uuid] Step 4: Index already exists, skipping...")
	}

	log.Println("[add_user_uuid] Migration completed successfully")
	return nil
}

func downAddUserUUID(ctx context.Context, tx *sql.Tx) error {
	log.Println("[add_user_uuid] Rolling back UUID migration...")

	// Detect database type
	dbType := detectDBType(ctx, tx)
	log.Printf("[add_user_uuid] Detected database type: %v\n", dbType)

	// Step 1: Drop unique index if exists
	idxExists, err := indexExistsMultiDB(ctx, tx, dbType, "users", "idx_users_uuid")
	if err != nil {
		return fmt.Errorf("failed to check if index exists: %w", err)
	}

	if idxExists {
		log.Println("[add_user_uuid] Step 1: Dropping unique index...")
		var dropIndexSQL string
		switch dbType {
		case DBTypePostgres:
			dropIndexSQL = `DROP INDEX idx_users_uuid`
		case DBTypeMySQL:
			dropIndexSQL = `DROP INDEX idx_users_uuid ON users`
		case DBTypeSQLite:
			dropIndexSQL = `DROP INDEX idx_users_uuid`
		default:
			dropIndexSQL = `DROP INDEX idx_users_uuid ON users`
		}
		_, err = tx.ExecContext(ctx, dropIndexSQL)
		if err != nil {
			return fmt.Errorf("failed to drop unique index: %w", err)
		}
	} else {
		log.Println("[add_user_uuid] Step 1: Index does not exist, skipping...")
	}

	// Step 2: Drop uuid column if exists
	exists, err := columnExistsMultiDB(ctx, tx, dbType, "users", "uuid")
	if err != nil {
		return fmt.Errorf("failed to check if uuid column exists: %w", err)
	}

	if exists {
		log.Println("[add_user_uuid] Step 2: Dropping uuid column...")
		// Note: SQLite doesn't support DROP COLUMN in older versions
		// For SQLite < 3.35.0, this will fail and require table recreation
		_, err = tx.ExecContext(ctx, `ALTER TABLE users DROP COLUMN uuid`)
		if err != nil {
			return fmt.Errorf("failed to drop uuid column: %w", err)
		}
	} else {
		log.Println("[add_user_uuid] Step 2: uuid column does not exist, skipping...")
	}

	log.Println("[add_user_uuid] Rollback completed successfully")
	return nil
}
