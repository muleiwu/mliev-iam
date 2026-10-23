package model

import (
	"encoding/json"
	"errors"
	"time"
)

type MiniProgramConfig struct {
	Enabled       bool   `json:"enabled"`
	ConnectionID  uint   `json:"connectionId"`
	AccountPolicy string `json:"accountPolicy"`
}

func DecodeMiniProgramConfig(config map[string]any) (MiniProgramConfig, error) {
	result := MiniProgramConfig{AccountPolicy: "bind_existing"}
	if raw, ok := config["miniProgram"]; ok && raw != nil {
		data, err := json.Marshal(raw)
		if err != nil {
			return result, errors.New("小程序配置无效")
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return result, errors.New("小程序配置无效")
		}
	}
	if result.AccountPolicy == "" {
		result.AccountPolicy = "bind_existing"
	}
	if result.AccountPolicy != "bind_existing" && result.AccountPolicy != "auto_register" {
		return result, errors.New("小程序账号策略无效")
	}
	if result.Enabled && result.ConnectionID == 0 {
		return result, errors.New("请选择微信小程序身份连接")
	}
	return result, nil
}

// Hash is SHA-256 of [kind, scope, subject]. It gives case-sensitive,
// collation-independent uniqueness on all supported databases.
type WeChatIdentity struct {
	Hash      string `gorm:"primaryKey;size:64" json:"-"`
	Kind      string `gorm:"size:16;not null" json:"-"`
	Scope     string `gorm:"size:128;not null" json:"-"`
	Subject   string `gorm:"size:255;not null" json:"-"`
	UserID    uint   `gorm:"not null;index" json:"-"`
	CreatedAt time.Time
}

// Conflicts are append-only evidence, never an instruction to merge accounts.
type WeChatIdentityConflict struct {
	ID           uint   `gorm:"primaryKey"`
	ConnectionID uint   `gorm:"not null;index"`
	IdentityHash string `gorm:"size:64;not null"`
	CreatedAt    time.Time
}
