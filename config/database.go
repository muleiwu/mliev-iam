package config

import (
	"cnb.cool/mliev/open/go-web/pkg/helper"
	databaseAssembly "cnb.cool/mliev/open/go-web/pkg/server/database/assembly"
	"gorm.io/gorm"
)

type bootstrapDatabase struct{ databaseAssembly.Database }

func (d *bootstrapDatabase) Assembly() (any, error) {
	value, err := d.Database.Assembly()
	if err != nil {
		return nil, err
	}
	if helper.GetConfig().GetString("database.driver", "") == "sqlite" {
		db, err := value.(*gorm.DB).DB()
		if err != nil {
			return nil, err
		}
		// Authentication, audit and background jobs share one SQLite writer.
		// Queue in-process work instead of losing writes to SQLITE_BUSY.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	return value, nil
}
