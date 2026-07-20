package event

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func NewAdapter(connection *model.IdentityConnection, credentials map[string]any) (Adapter, error) {
	if connection == nil {
		return nil, fmt.Errorf("身份连接不能为空")
	}
	if connection.Category != model.ConnectionCategoryEnterprise {
		return nil, fmt.Errorf("只有企业身份连接可以接收目录事件")
	}
	switch connection.ProviderType {
	case "feishu", "lark":
		return NewFeishuAdapter(credentials)
	case "wecom", "wechat_work":
		return NewWeComAdapter(credentials)
	case "dingtalk":
		return NewDingTalkAdapter(credentials)
	default:
		return nil, fmt.Errorf("提供商 %q 暂不支持目录事件", connection.ProviderType)
	}
}
