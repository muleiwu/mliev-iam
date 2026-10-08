package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type TwoFactorDAO struct{ db *gorm.DB }

func NewTwoFactorDAO(db *gorm.DB) *TwoFactorDAO { return &TwoFactorDAO{db: db} }

func (d *TwoFactorDAO) Methods(userID uint) ([]model.TwoFactorMethod, error) {
	var methods []model.TwoFactorMethod
	err := d.db.Where("user_id = ?", userID).Order("method").Find(&methods).Error
	return methods, err
}

func (d *TwoFactorDAO) Method(userID uint, method string) (*model.TwoFactorMethod, error) {
	var factor model.TwoFactorMethod
	err := d.db.Where("user_id = ? AND method = ?", userID, method).First(&factor).Error
	return &factor, err
}

// ValidateSessionAssurance is also used by locked grant creation. It deliberately
// reads the user's current configuration, not a cached JWT claim.
func ValidateSessionAssurance(db *gorm.DB, session *model.LoginSession) error {
	var user model.User
	if err := db.First(&user, session.UserID).Error; err != nil {
		return err
	}
	if !user.CanLogin() || user.TwoFactorVersion != session.TwoFactorVersion {
		return ErrLoginSessionInvalid
	}
	if user.TwoFactorEnabled && (session.TwoFactorAt == nil || session.TwoFactorMethod == "") {
		return ErrLoginSessionInvalid
	}
	return nil
}
