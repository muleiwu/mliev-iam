package dao

import (
	"errors"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

var ErrLoginSessionInvalid = errors.New("登录会话已失效，请重新登录")

type LoginSessionDAO struct{ db *gorm.DB }

func NewLoginSessionDAO(db *gorm.DB) *LoginSessionDAO { return &LoginSessionDAO{db: db} }

func (d *LoginSessionDAO) Valid(id string, userID uint) (*model.LoginSession, error) {
	if id == "" || userID == 0 {
		return nil, ErrLoginSessionInvalid
	}
	var session model.LoginSession
	err := d.db.Where("id = ? AND user_id = ? AND ended_at IS NULL AND expires_at > ?", id, userID, time.Now()).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrLoginSessionInvalid
	}
	if err == nil {
		err = ValidateSessionAssurance(d.db, &session)
	}
	return &session, err
}

// Lock serializes grant creation/rotation and revocation on all supported SQL
// dialects, including SQLite where SELECT FOR UPDATE is unavailable.
func (d *LoginSessionDAO) Lock(id string, userID uint) error {
	if id == "" || userID == 0 {
		return ErrLoginSessionInvalid
	}
	r := d.db.Model(&model.LoginSession{}).Where("id = ? AND user_id = ? AND ended_at IS NULL AND expires_at > ?", id, userID, time.Now()).UpdateColumn("id", gorm.Expr("id"))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		_, err := d.Valid(id, userID)
		return err
	}
	_, err := d.Valid(id, userID)
	return err
}

func (d *LoginSessionDAO) Extend(id string, until time.Time) error {
	return d.db.Model(&model.LoginSession{}).Where("id = ? AND ended_at IS NULL AND expires_at < ?", id, until).UpdateColumn("expires_at", until).Error
}

func (d *LoginSessionDAO) Touch(id, ip string) error {
	updates := map[string]any{"last_active_at": time.Now()}
	if ip != "" {
		updates["last_ip"] = ip
	}
	return d.db.Model(&model.LoginSession{}).Where("id = ? AND ended_at IS NULL AND last_active_at < ?", id, time.Now().Add(-time.Minute)).Updates(updates).Error
}

// LockLoginUser must precede session locks whenever both are needed. Account
// changes, new logins and bulk revocations share this write lock.
func LockLoginUser(db *gorm.DB, userID uint) error {
	r := db.Unscoped().Model(&model.User{}).Where("id = ?", userID).UpdateColumn("id", gorm.Expr("id"))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		var user model.User
		return db.Unscoped().First(&user, userID).Error
	}
	return nil
}
