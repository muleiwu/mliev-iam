package model

import (
	"time"

	"gorm.io/gorm"
)

// ApplicationOIDCKey OIDC应用密钥模型 - 用于存储每个应用的签名密钥
type ApplicationOIDCKey struct {
	// 基础信息
	ID            uint   `gorm:"primaryKey" json:"id"`
	ApplicationID uint   `gorm:"not null;index:idx_app_keys" json:"applicationId"`
	KeyID         string `gorm:"size:100;uniqueIndex;not null" json:"keyId"` // kid，用于JWT header

	// 密钥信息
	Algorithm  string `gorm:"size:20;not null" json:"algorithm"`                 // RS256, RS512, HS256, HS512, ES256等
	KeyType    string `gorm:"size:20;not null" json:"keyType"`                   // RSA, HMAC, ECDSA等
	PrivateKey string `gorm:"type:text;not null" json:"-"`                       // 私钥数据（PEM或base64，加密存储）
	PublicKey  string `gorm:"type:text" json:"publicKey,omitempty"`              // 公钥数据（非对称加密使用）
	KeySize    int    `gorm:"default:0" json:"keySize"`                          // 密钥长度（2048, 4096, 256等）
	IsPrimary  bool   `gorm:"default:false;index:idx_app_keys" json:"isPrimary"` // 是否为主密钥

	// 过期和时间信息
	ExpiresAt *time.Time     `gorm:"index" json:"expiresAt,omitempty"` // 密钥过期时间（可选，为轮转预留）
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// 关联
	Application *Application `gorm:"foreignKey:ApplicationID" json:"-"`
}

// TableName returns the table name for ApplicationOIDCKey model
func (*ApplicationOIDCKey) TableName() string {
	return "application_oidc_keys"
}

// IsExpired 检查密钥是否已过期
func (k *ApplicationOIDCKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsValid 检查密钥是否有效（未过期且未被软删除）
func (k *ApplicationOIDCKey) IsValid() bool {
	return !k.IsExpired() && k.DeletedAt.Time.IsZero()
}
