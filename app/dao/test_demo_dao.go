package dao

import (
	"cnb.cool/mliev/examples/go-web/app/model"
	"gorm.io/gorm"
)

// TestDemoDao 注入
type TestDemoDao struct {
	database *gorm.DB
}

func (receiver *TestDemoDao) GetUserByUsername(username string) (*model.TestDemo, error) {
	var user model.TestDemo
	if err := receiver.database.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
