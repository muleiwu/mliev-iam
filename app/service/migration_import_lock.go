package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// 导入任务的分布式协调（基于 Redis），用于多实例部署：
//   - 源级互斥锁：同一迁移源同一时刻只允许一个导入任务（跨实例）。
//   - 取消标志：取消请求落到 Redis，任意实例上运行的任务通过 watcher 感知并停止。
const (
	importLockTTL      = 60 * time.Second
	importLockRenew    = 20 * time.Second
	importCancelTTL    = 2 * time.Hour
	importCancelPoll   = 2 * time.Second
	importRedisOpTimeo = 3 * time.Second
)

func importLockKey(sourceID uint) string {
	return fmt.Sprintf("migration:import:lock:%d", sourceID)
}

func importCancelKey(taskID uint) string {
	return fmt.Sprintf("migration:import:cancel:%d", taskID)
}

// 仅当持有者匹配时才续期 / 释放，避免误删他人锁。
var (
	importRenewScript   = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("EXPIRE", KEYS[1], ARGV[2]) else return 0 end`)
	importReleaseScript = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)
)

// acquireImportLock 尝试抢占源级锁。redis 为 nil 时返回 true（退化为无分布式锁）。
func acquireImportLock(rdb *redis.Client, sourceID uint, token string) (bool, error) {
	if rdb == nil {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	return rdb.SetNX(ctx, importLockKey(sourceID), token, importLockTTL).Result()
}

func renewImportLock(rdb *redis.Client, sourceID uint, token string) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	_ = importRenewScript.Run(ctx, rdb,
		[]string{importLockKey(sourceID)}, token, int(importLockTTL.Seconds())).Err()
}

func releaseImportLock(rdb *redis.Client, sourceID uint, token string) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	_ = importReleaseScript.Run(ctx, rdb, []string{importLockKey(sourceID)}, token).Err()
}

// setImportCancelFlag 写入取消标志（跨实例可见）。
func setImportCancelFlag(rdb *redis.Client, taskID uint) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	_ = rdb.Set(ctx, importCancelKey(taskID), "1", importCancelTTL).Err()
}

// isImportCancelRequested 查询取消标志。
func isImportCancelRequested(rdb *redis.Client, taskID uint) bool {
	if rdb == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	n, err := rdb.Exists(ctx, importCancelKey(taskID)).Result()
	return err == nil && n > 0
}

func clearImportCancelFlag(rdb *redis.Client, taskID uint) {
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), importRedisOpTimeo)
	defer cancel()
	_ = rdb.Del(ctx, importCancelKey(taskID)).Err()
}
