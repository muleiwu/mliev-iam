package extension

import (
	"context"
	"gorm.io/gorm"
	"reflect"
)

type mutationOriginKey struct{}

func WithMutationOrigin(db *gorm.DB, origin string) *gorm.DB {
	return db.WithContext(context.WithValue(db.Statement.Context, mutationOriginKey{}, origin))
}
func MutationOrigin(db *gorm.DB) string {
	value, _ := db.Statement.Context.Value(mutationOriginKey{}).(string)
	return value
}

// Mutation describes actual database writes, including batch writes and directory
// synchronization. Policies run on the same connection/transaction as the write.
type Mutation struct {
	Table, Action  string
	Before, Values []map[string]any
}
type MutationPolicy func(*gorm.DB, Mutation) error

// RegisterMutationPolicy installs a database-local policy. No policy is present
// in community/enterprise unless their composition explicitly registers one.
func RegisterMutationPolicy(db *gorm.DB, name string, tables []string, policy MutationPolicy) error {
	callback := func(action string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			if tx.Error != nil {
				return
			}
			table := tx.Statement.Table
			selected := false
			for _, t := range tables {
				if t == table {
					selected = true
					break
				}
			}
			if !selected {
				return
			}
			m := Mutation{Table: table, Action: action, Values: mutationValues(tx, tx.Statement.Dest)}
			if action != "create" {
				q := tx.Session(&gorm.Session{NewDB: true}).Unscoped().Table(table)
				if tx.Statement.Schema != nil {
					q = q.Model(tx.Statement.Model)
				}
				if condition, ok := tx.Statement.Clauses["WHERE"]; ok {
					q = q.Where(condition.Expression)
				}
				modelValues := mutationValues(tx, tx.Statement.Model)
				if len(modelValues) == 1 && tx.Statement.Schema != nil {
					for _, key := range tx.Statement.Schema.PrimaryFields {
						value := modelValues[0][key.DBName]
						if value != nil && !reflect.ValueOf(value).IsZero() {
							q = q.Where(key.DBName+" = ?", value)
						}
					}
				}
				if err := q.Find(&m.Before).Error; err != nil {
					tx.AddError(err)
					return
				}
			}
			if err := policy(tx, m); err != nil {
				tx.AddError(err)
			}
		}
	}
	if err := db.Callback().Create().Before("gorm:before_create").Register(name+":create", callback("create")); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:before_update").Register(name+":update", callback("update")); err != nil {
		return err
	}
	return db.Callback().Delete().Before("gorm:before_delete").Register(name+":delete", callback("delete"))
}

func mutationValues(tx *gorm.DB, value any) []map[string]any {
	if row, ok := value.(map[string]any); ok {
		return []map[string]any{row}
	}
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		var rows []map[string]any
		for i := 0; i < v.Len(); i++ {
			rows = append(rows, mutationValues(tx, v.Index(i).Interface())...)
		}
		return rows
	}
	if v.Kind() != reflect.Struct || tx.Statement.Schema == nil {
		return nil
	}
	row := map[string]any{}
	for _, field := range tx.Statement.Schema.Fields {
		if field.DBName != "" {
			row[field.DBName], _ = field.ValueOf(tx.Statement.Context, v)
		}
	}
	return []map[string]any{row}
}
