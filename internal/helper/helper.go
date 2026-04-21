// Package helper 提供 HelperInterface 单例，是迁移 mach/go-web 期间的过渡桥接。
//
// 过渡策略：
//   - 目标框架（go-web）将 env/config/logger/db/redis/cache 放在 cnb.cool/mliev/open/go-web/pkg/helper
//     的包级全局函数里，controller 与 middleware 直接 helper.GetDatabase() 即可。
//   - 本包保留 HelperInterface + GetHelper() 单例 API，把每次访问委托给 go-web 的包级函数。
//   - 这样未迁移完成的 controller（仍持有 helper 参数并调用 helper.GetDatabase() 等）继续工作，
//     而底层数据源已由 go-web 的装配链托管。
//   - Controller 全部切到 go-web signature 后，本包会被删除；新增代码请直接用 go-web helper。
package helper

import (
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	goweb "cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Helper 是 HelperInterface 的过渡实现；字段都委托给 go-web 包级函数，
// 无状态，可在 container 装配完成后任意时刻调用。
type Helper struct{}

var helperData interfaces.HelperInterface = &Helper{}

// GetHelper 返回进程级单例；在 go-web 装配完成前调用时，Getter 也会返回 nil（由 go-web 决定）。
func GetHelper() interfaces.HelperInterface {
	return helperData
}

func (*Helper) GetEnv() gsr.Enver       { return goweb.GetEnv() }
func (*Helper) GetConfig() gsr.Provider { return goweb.GetConfig() }
func (*Helper) GetLogger() gsr.Logger   { return goweb.GetLogger() }
func (*Helper) GetRedis() *redis.Client { return goweb.GetRedis() }
func (*Helper) GetDatabase() *gorm.DB   { return goweb.GetDatabase() }
func (*Helper) GetCache() gsr.Cacher    { return goweb.GetCache() }

// Setter 在新架构下不再需要（由 container 托管），仅保留空实现以满足接口。
func (*Helper) SetEnv(gsr.Enver)       {}
func (*Helper) SetConfig(gsr.Provider) {}
func (*Helper) SetLogger(gsr.Logger)   {}
func (*Helper) SetRedis(*redis.Client) {}
func (*Helper) SetDatabase(*gorm.DB)   {}
func (*Helper) SetCache(gsr.Cacher)    {}

// GetSetting / GetSettingWithDefault / SetSetting 委托给 app/service.SettingService。
// 新代码请直接使用 service.NewSettingServiceFromHelper()。
func (*Helper) GetSetting(key string) string {
	return service.NewSettingServiceFromHelper().Get(key)
}

func (*Helper) GetSettingWithDefault(key, defaultValue string) string {
	return service.NewSettingServiceFromHelper().GetWithDefault(key, defaultValue)
}

func (*Helper) SetSetting(key, value string) error {
	return service.NewSettingServiceFromHelper().SetSimple(key, value)
}
