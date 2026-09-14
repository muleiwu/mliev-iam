package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"errors"
	"gorm.io/gorm"
)

func initializeInstallationOnce(db *gorm.DB, admin dto.AdminAccountInfo, id string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var marker model.Setting
		err := tx.Where("setting_key = ?", "system.installation_id").First(&marker).Error
		if err == nil {
			if marker.Value != id {
				return errors.New("数据库已由其他安装初始化，请使用原配置启动")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Unscoped().Model(&model.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("数据库已有用户，请使用原配置升级；安装向导不会覆盖已有账户")
		}
		if err := NewInstallService(tx).CreateInitialData(admin); err != nil {
			return err
		}
		return tx.Create(&model.Setting{Key: "system.installation_id", Value: id, Category: "system"}).Error
	})
}
