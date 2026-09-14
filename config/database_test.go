package config

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cnb.cool/mliev/open/go-web/pkg/container"
	configImpl "cnb.cool/mliev/open/go-web/pkg/server/config/impl"
	"github.com/muleiwu/gsr"
	"gorm.io/gorm"
)

type databaseTestProvider struct{ config gsr.Provider }

func (p databaseTestProvider) Type() reflect.Type { return reflect.TypeFor[gsr.Provider]() }
func (p databaseTestProvider) Build() any         { return p.config }
func (databaseTestProvider) Priority() int        { return 0 }

func TestSQLiteRuntimeQueuesCompetingWrites(t *testing.T) {
	container.ResetAll()
	t.Cleanup(container.ResetAll)
	cfg := configImpl.NewConfig()
	cfg.Set("database.driver", "sqlite")
	cfg.Set("database.host", filepath.Join(t.TempDir(), "runtime.db"))
	container.Register(databaseTestProvider{cfg})
	var db *gorm.DB
	for _, assembly := range (App{}).Assemblies() {
		if assembly.Type() != reflect.TypeFor[*gorm.DB]() {
			continue
		}
		value, err := assembly.Assembly()
		if err != nil {
			t.Fatal(err)
		}
		db = value.(*gorm.DB)
	}
	if db == nil {
		t.Fatal("missing runtime database assembly")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Exec("CREATE TABLE queued_writes (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := held.Exec("INSERT INTO queued_writes VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- db.WithContext(ctx).Exec("INSERT INTO queued_writes VALUES (2)").Error }()
	select {
	case err := <-done:
		t.Fatalf("competing writer must wait for the transaction instead of losing work: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := held.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("queued writer failed: %v", err)
	}
	var count int64
	if err := db.Table("queued_writes").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("lost write: %d, %v", count, err)
	}
}
