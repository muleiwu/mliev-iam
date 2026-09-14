package extension

import (
	"errors"
	"gorm.io/gorm"
	"sync"
)

type LoginIdentifierOption struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}
type LoginIdentifier struct {
	LoginIdentifierOption
	Resolve func(*gorm.DB, string) (uint, error)
}

var identifiers struct {
	sync.RWMutex
	values []LoginIdentifier
}

// ConfigureLoginIdentifiers is called at application composition time only.
func ConfigureLoginIdentifiers(values []LoginIdentifier) {
	identifiers.Lock()
	defer identifiers.Unlock()
	identifiers.values = append([]LoginIdentifier(nil), values...)
}
func LoginIdentifierOptions() []LoginIdentifierOption {
	identifiers.RLock()
	defer identifiers.RUnlock()
	result := []LoginIdentifierOption{}
	for _, value := range identifiers.values {
		result = append(result, value.LoginIdentifierOption)
	}
	return result
}
func ResolveLoginIdentifier(db *gorm.DB, kind, value string) (uint, error) {
	identifiers.RLock()
	list := append([]LoginIdentifier(nil), identifiers.values...)
	identifiers.RUnlock()
	for _, item := range list {
		if item.Type == kind && item.Resolve != nil {
			return item.Resolve(db, value)
		}
	}
	return 0, errors.New("登录标识类型不可用")
}
