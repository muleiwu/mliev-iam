package protocol

import "cnb.cool/mliev/iam/mliev-iam/app/model"

// ProtocolHandler 协议处理器接口
// 定义了所有协议处理器必须实现的方法
type ProtocolHandler interface {
	// ValidateConfig 验证协议特定的配置
	ValidateConfig(config map[string]interface{}) error

	// PrepareConfig 准备配置（例如生成ClientID/Secret）
	PrepareConfig(config map[string]interface{}) (map[string]interface{}, error)

	// EncryptSensitiveFields 加密/哈希配置中的敏感字段（用于存储）
	EncryptSensitiveFields(config map[string]interface{}) error

	// CreateApplication 创建应用时的协议特定处理
	CreateApplication(app *model.Application) error

	// UpdateApplication 更新应用时的协议特定处理
	UpdateApplication(app *model.Application, updates map[string]interface{}) error

	// DeleteApplication 删除应用时的协议特定处理
	DeleteApplication(id uint) error

	// GetProtocolDetails 获取协议特定的详细信息
	GetProtocolDetails(app *model.Application) (interface{}, error)
}
