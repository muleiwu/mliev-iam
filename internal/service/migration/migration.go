package migration

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
)

type Migration struct {
	Helper    interfaces.HelperInterface
	Migration []any
}

func (receiver *Migration) Run() error {

	if !receiver.Helper.GetConfig().GetBool("app.installed", false) {
		receiver.Helper.GetLogger().Warn("No Install, stop migrated !")
		return nil
	}

	if len(receiver.Migration) > 0 {
		// Disable foreign key checks during migration
		db := receiver.Helper.GetDatabase()
		db.Exec("SET FOREIGN_KEY_CHECKS = 0;")

		err := db.AutoMigrate(receiver.Migration...)
		if err != nil {
			panic(err)
		}

		// Re-enable foreign key checks after migration
		db.Exec("SET FOREIGN_KEY_CHECKS = 1;")

		receiver.Helper.GetLogger().Info(fmt.Sprintf("[db migration success: %d models migrated]", len(receiver.Migration)))
	}
	return nil
}

// Stop implements ServerInterface
func (receiver *Migration) Stop() error {
	// Migration service doesn't need to stop anything
	return nil
}
