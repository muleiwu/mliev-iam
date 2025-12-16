package assembly

import (
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	gocache "github.com/muleiwu/go-cache"
	"github.com/muleiwu/gsr"
)

type Cache struct {
	Helper interfaces.HelperInterface
}

func (receiver *Cache) Assembly() error {
	driver := receiver.Helper.GetConfig().GetString("cache.driver", "memory")
	receiver.Helper.GetLogger().Debug("加载缓存驱动: " + driver)

	if driver == "redis" && receiver.Helper.GetRedis() == nil {
		return errors.New("缓存服务驱动配置为：redis，但Redis服务不可用，拒绝启动")
	}

	cacheDriver, err := receiver.GetDriver(driver)
	if err != nil {
		return fmt.Errorf("[cache] 加载缓存驱动失败: %w", err)
	}
	receiver.Helper.SetCache(cacheDriver)

	return nil
}

func (receiver *Cache) GetDriver(driver string) (gsr.Cacher, error) {
	switch driver {
	case "redis":
		return gocache.NewRedis(receiver.Helper.GetRedis()), nil
	case "memory", "local":
		// 设置超时时间和清理时间
		return gocache.NewMemory(5*time.Minute, 10*time.Minute), nil
	case "none":
		return gocache.NewNone(), nil
	default:
		return nil, fmt.Errorf("不支持的缓存驱动: %s", driver)
	}
}
