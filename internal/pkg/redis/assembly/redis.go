package assembly

import (
	"sync"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/redis/config"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/redis/impl"
)

type Redis struct {
	Helper interfaces.HelperInterface
}

var (
	redisOnce sync.Once
)

func (receiver *Redis) Assembly() error {
	redisConfig := config.NewRedis(receiver.Helper.GetConfig())

	redis, err := impl.NewRedis(receiver.Helper, redisConfig.Host, redisConfig.Port, redisConfig.DB, redisConfig.Password)
	receiver.Helper.SetRedis(redis)

	return err
}
