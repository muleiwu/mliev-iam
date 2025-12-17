package helper

import (
	"context"
	"sync"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Helper struct {
	env      gsr.Enver
	config   gsr.Provider
	logger   gsr.Logger
	redis    *redis.Client
	database *gorm.DB
	cache    gsr.Cacher
}

var helperOnce sync.Once
var helperData interfaces.HelperInterface

func GetHelper() interfaces.HelperInterface {
	helperOnce.Do(func() {
		helperData = &Helper{}
	})
	return helperData
}

func (receiver *Helper) GetEnv() gsr.Enver {
	return receiver.env
}

func (receiver *Helper) GetConfig() gsr.Provider {
	return receiver.config
}

func (receiver *Helper) GetLogger() gsr.Logger {
	return receiver.logger
}

func (receiver *Helper) GetRedis() *redis.Client {
	return receiver.redis
}

func (receiver *Helper) GetDatabase() *gorm.DB {
	return receiver.database
}

func (receiver *Helper) SetEnv(env gsr.Enver) {
	receiver.env = env
}

func (receiver *Helper) SetConfig(config gsr.Provider) {
	receiver.config = config
}

func (receiver *Helper) SetLogger(logger gsr.Logger) {
	receiver.logger = logger
}

func (receiver *Helper) SetRedis(redis *redis.Client) {
	receiver.redis = redis
}

func (receiver *Helper) SetDatabase(database *gorm.DB) {
	receiver.database = database
}

func (receiver *Helper) GetCache() gsr.Cacher {
	return receiver.cache
}

func (receiver *Helper) SetCache(cache gsr.Cacher) {
	receiver.cache = cache
}

// GetSetting retrieves a setting value by key, using cache when available
func (receiver *Helper) GetSetting(key string) string {
	return receiver.GetSettingWithDefault(key, "")
}

// GetSettingWithDefault retrieves a setting value by key with a default value, using cache when available
func (receiver *Helper) GetSettingWithDefault(key string, defaultValue string) string {
	cacheKey := "setting:" + key
	ctx := context.Background()

	// Try to get from cache first
	if receiver.cache != nil {
		var cachedValue string
		err := receiver.cache.Get(ctx, cacheKey, &cachedValue)
		if err == nil && cachedValue != "" {
			return cachedValue
		}
	}

	// Cache miss or no cache, query from database
	if receiver.database == nil {
		return defaultValue
	}

	settingDao := dao.NewSettingDao(receiver.database)
	setting, err := settingDao.GetByKey(key)
	if err != nil {
		return defaultValue
	}

	// Store in cache with 1 hour TTL
	if receiver.cache != nil {
		_ = receiver.cache.Set(ctx, cacheKey, setting.Value, time.Hour)
	}

	return setting.Value
}

// SetSetting updates a setting value and invalidates cache
func (receiver *Helper) SetSetting(key, value string) error {
	if receiver.database == nil {
		return gorm.ErrInvalidDB
	}

	settingDao := dao.NewSettingDao(receiver.database)
	err := settingDao.UpdateByKey(key, value)
	if err != nil {
		return err
	}

	// Invalidate cache
	if receiver.cache != nil {
		cacheKey := "setting:" + key
		ctx := context.Background()
		_ = receiver.cache.Del(ctx, cacheKey)
	}

	return nil
}
