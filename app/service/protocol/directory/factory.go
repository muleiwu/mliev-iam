package directory

import (
	"fmt"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func NewAdapter(connection *model.IdentityConnection, credentials map[string]any, client *http.Client) (Adapter, error) {
	if connection == nil {
		return nil, fmt.Errorf("身份连接不能为空")
	}
	if connection.Category != model.ConnectionCategoryEnterprise {
		return nil, fmt.Errorf("只有企业身份连接可以启用目录同步")
	}
	switch connection.ProviderType {
	case "feishu", "lark":
		return NewFeishuAdapter(credentials, client)
	case "wecom", "wechat_work":
		return NewWeComAdapter(credentials, client)
	case "dingtalk":
		return NewDingTalkAdapter(credentials, client)
	default:
		return nil, fmt.Errorf("提供商 %q 暂不支持目录同步", connection.ProviderType)
	}
}
