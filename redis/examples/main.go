// lockx/redis/examples: Redis 分布式锁示例
// 功能：Lock / TryLock / Unlock / Renew / WithTTL / WithWatchdog
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/lockx"
	_ "github.com/gospacex/lockx/redis"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "lockx redis example started", logx.String("driver", "redis"))

	locker, err := lockx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create lockx client failed", logx.Err(err))
	}
	defer locker.Close()
	logger.InfoContext(ctx, "locker driver", logx.String("name", locker.Name()))

	// Lock
	lock, err := locker.Lock(ctx, "resource:order:1001", lockx.WithTTL(30*time.Second))
	if err != nil {
		logger.Fatal("lock failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock acquired", logx.String("key", lock.Key()))

	// TryLock（同一 key 应失败）
	_, err = locker.TryLock(ctx, "resource:order:1001", lockx.WithTTL(30*time.Second))
	if err != nil {
		logger.InfoContext(ctx, "try lock blocked as expected", logx.Err(err))
	}

	// Renew
	if err := lock.Renew(ctx); err != nil {
		logger.Fatal("renew failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock renewed")

	// Unlock
	if err := lock.Unlock(ctx); err != nil {
		logger.Fatal("unlock failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock released")

	// WithWatchdog
	lock2, err := locker.Lock(ctx, "resource:order:2002",
		lockx.WithTTL(60*time.Second),
		lockx.WithWatchdog(10*time.Second),
	)
	if err != nil {
		logger.Fatal("lock with watchdog failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock with watchdog acquired", logx.String("key", lock2.Key()))
	if lock2.Expired() {
		logger.InfoContext(ctx, "lock is expired")
	} else {
		logger.InfoContext(ctx, "lock is still valid")
	}
	if err := lock2.Unlock(ctx); err != nil {
		logger.Fatal("unlock failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "watchdog lock released")

	fmt.Println("\n✅ All operations completed successfully.")
}
