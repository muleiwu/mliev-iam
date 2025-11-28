package protocol

import (
	"fmt"

	"gorm.io/gorm"
)

// ProtocolHandlerFactory 协议处理器工厂
type ProtocolHandlerFactory struct {
	db *gorm.DB
}

// NewProtocolHandlerFactory 创建协议处理器工厂
func NewProtocolHandlerFactory(db *gorm.DB) *ProtocolHandlerFactory {
	return &ProtocolHandlerFactory{db: db}
}

// GetHandler 根据协议类型获取对应的处理器
func (f *ProtocolHandlerFactory) GetHandler(protocolType string) (ProtocolHandler, error) {
	return f.GetHandlerWithDB(protocolType, f.db)
}

// GetHandlerWithDB 根据协议类型和数据库连接获取对应的处理器（支持事务）
func (f *ProtocolHandlerFactory) GetHandlerWithDB(protocolType string, db *gorm.DB) (ProtocolHandler, error) {
	switch protocolType {
	case "oidc":
		return NewOIDCProtocolHandler(db), nil
	case "saml":
		return NewSAMLProtocolHandler(db), nil
	case "cas":
		return NewCASProtocolHandler(db), nil
	case "oauth2":
		return NewOAuth2ProtocolHandler(db), nil
	default:
		return nil, fmt.Errorf("不支持的协议类型: %s", protocolType)
	}
}

// GetSupportedProtocols 获取支持的协议列表
func (f *ProtocolHandlerFactory) GetSupportedProtocols() []string {
	return []string{"oidc", "saml", "cas", "oauth2"}
}

// IsProtocolSupported 检查协议是否支持
func (f *ProtocolHandlerFactory) IsProtocolSupported(protocolType string) bool {
	supportedProtocols := f.GetSupportedProtocols()
	for _, protocol := range supportedProtocols {
		if protocol == protocolType {
			return true
		}
	}
	return false
}
