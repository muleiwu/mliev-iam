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
	switch protocolType {
	case "oidc":
		return NewOIDCProtocolHandler(f.db), nil
	case "saml":
		// TODO: 实现SAML协议处理器
		return nil, fmt.Errorf("SAML协议处理器尚未实现")
	case "cas":
		// TODO: 实现CAS协议处理器
		return nil, fmt.Errorf("CAS协议处理器尚未实现")
	case "oauth2":
		// TODO: 实现OAuth2协议处理器
		return nil, fmt.Errorf("OAuth2协议处理器尚未实现")
	default:
		return nil, fmt.Errorf("不支持的协议类型: %s", protocolType)
	}
}

// GetSupportedProtocols 获取支持的协议列表
func (f *ProtocolHandlerFactory) GetSupportedProtocols() []string {
	return []string{"oidc"} // 目前只支持OIDC
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
