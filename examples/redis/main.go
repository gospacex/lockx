// lockx/examples/redis/main.go: Redis 分布式锁示例
// 功能：获取锁、尝试获取锁、锁续期、Key/Expired 状态查询、解锁、调用链追踪
package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/lockx"
	_ "github.com/gospacex/lockx/redis"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func initTracer() (*sdktrace.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://localhost:14268/api/traces")))
	if err != nil {
		return nil, fmt.Errorf("jaeger exporter init failed: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.Default()),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func main() {
	ctx := context.Background()

	// 1. 初始化 logx
	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()

	_, err := initTracer()
	if err != nil {
		logger.InfoContext(ctx, "tracer init skipped", logx.Err(err))
	}
	logger.InfoContext(ctx, "lockx redis example started", logx.String("driver", "redis"))

	// 2. 读取配置并创建客户端
	locker, err := lockx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create lockx client failed", logx.Err(err))
	}
	defer locker.Close()

	// 3. 测试获取锁
	lockName := "test-resource-1"
	logger.InfoContext(ctx, "acquiring lock", logx.String("lock", lockName))
	lock, err := locker.Lock(ctx, lockName, lockx.WithTTL(10*time.Second))
	if err != nil {
		logger.Fatal("lock failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock acquired",
		logx.String("lock", lockName),
		logx.String("driver", locker.Name()),
		logx.String("key", lock.Key()),
		logx.Any("expired", lock.Expired()))

	// 4. 锁续期（看门狗模拟）
	logger.InfoContext(ctx, "renewing lock")
	if err := lock.Renew(ctx); err != nil {
		logger.Fatal("renew failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock renewed", logx.Any("expired", lock.Expired()))

	// 5. 测试解锁
	if err := lock.Unlock(ctx); err != nil {
		logger.Fatal("unlock failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lock released", logx.String("lock", lockName))

	// 6. 测试 TryLock
	logger.InfoContext(ctx, "trying to acquire lock", logx.String("lock", lockName))
	lock2, err := locker.TryLock(ctx, lockName, lockx.WithTTL(5*time.Second))
	if err != nil {
		logger.Fatal("tryLock failed", logx.Err(err))
	}
	if lock2 != nil {
		logger.InfoContext(ctx, "lock acquired via TryLock",
			logx.String("key", lock2.Key()),
			logx.Any("expired", lock2.Expired()))
		if err := lock2.Unlock(ctx); err != nil {
			logger.Fatal("unlock failed", logx.Err(err))
		}
		logger.InfoContext(ctx, "lock released", logx.String("lock", lockName))
	} else {
		logger.InfoContext(ctx, "lock is busy", logx.String("lock", lockName))
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
