package dao

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type PasskeyDAO struct{ db *gorm.DB }

func NewPasskeyDAO(db *gorm.DB) *PasskeyDAO { return &PasskeyDAO{db: db} }

func PasskeyCredentialHash(id []byte) string {
	hash := sha256.Sum256(id)
	return hex.EncodeToString(hash[:])
}

func (d *PasskeyDAO) List(userID uint) ([]model.Passkey, error) {
	items := []model.Passkey{}
	err := d.db.Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&items).Error
	return items, err
}

func (d *PasskeyDAO) Credential(rpID string, id []byte) (*model.Passkey, error) {
	var item model.Passkey
	err := d.db.Where("rp_id = ? AND credential_id_hash = ?", rpID, PasskeyCredentialHash(id)).First(&item).Error
	if err == nil && item.CredentialID != base64.RawURLEncoding.EncodeToString(id) {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, err
}
