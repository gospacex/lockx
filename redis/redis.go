// lockx/redis: Redis 分布式锁驱动（基于 redsync）
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	goredis "github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/gospacex/lockx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LockerImpl Redis 分布式锁实现
type LockerImpl struct {
	rs     *redsync.Redsync
	client redis.UniversalClient
	name   string
	cfg    *lockx.Config
	tracer *traceflag.Tracer
	logger logx.Logger
}

// LockImpl 已获取的锁
type LockImpl struct {
	mtx    *redsync.Mutex
	key    string
	ttl    time.Duration
	name   string
	tracer *traceflag.Tracer
	logger logx.Logger
}

// New 创建 Redis 锁实例
func New(cfg *lockx.Config) (*LockerImpl, error) {
	if cfg.Driver != "redis" {
		return nil, fmt.Errorf("redis: expected driver 'redis', got %q", cfg.Driver)
	}
	addr := cfg.Addr
	if addr == "" {
		addr = "localhost:6379"
	}
	opts := &redis.Options{
		Addr:     addr,
		Password: cfg.Password,
		DB:       0,
	}
	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	rdb := redis.NewClient(opts)
	if err := withRetry(func() error { return rdb.Ping(context.Background()).Err() }, 3); err != nil {
		return nil, fmt.Errorf("lockx/redis: ping failed: %w", err)
	}
	pool := goredis.NewPool(rdb)
	ms := redsync.New(pool)
	ctxLog := context.Background()
	logx.GetGlobal().InfoContext(ctxLog, "lockx/redis initialized",
		logx.String("name", cfg.Name), logx.String("addr", addr))
	return &LockerImpl{
		rs:     ms,
		client: rdb,
		name:   cfg.Name,
		cfg:    cfg,
		tracer: traceflag.NewTracer(cfg.EnableTrace, "github.com/gospacex/lockx/redis", "lockx"),
		logger: logx.GetGlobal(),
	}, nil
}

func (l *LockerImpl) Lock(ctx context.Context, key string, opts ...lockx.LockOption) (lockx.Lock, error) {
	var span trace.Span
	if l.cfg != nil && l.cfg.EnableTrace {
		ctx, span = l.tracer.Start(ctx, "lock.redis.acquire")
		defer span.End()
	}
	if span != nil {
		span.SetAttributes(
			attribute.String("lock.system", "redis"),
			attribute.String("lock.key", key),
		)
	}

	o := lockx.LockOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	ttl := o.TTL
	if ttl <= 0 {
		ttl = lockx.DefaultTTL
	}

	ml := l.rs.NewMutex(key, redsync.WithExpiry(ttl))
	if err := ml.LockContext(ctx); err != nil {
		if span != nil {
			span.RecordError(err)
		}
		l.logger.ErrorContext(ctx, "lockx/redis lock failed", logx.Err(err), logx.String("key", key))
		return nil, fmt.Errorf("lockx/redis: lock %q: %w", key, err)
	}

	if span != nil {
		span.SetAttributes(attribute.Bool("lock.acquired", true))
	}
	l.logger.InfoContext(ctx, "lockx/redis lock acquired", logx.String("key", key))
	return &LockImpl{
		mtx:    ml,
		key:    key,
		ttl:    ttl,
		name:   l.name,
		tracer: l.tracer,
		logger: l.logger,
	}, nil
}

func (l *LockerImpl) TryLock(ctx context.Context, key string, opts ...lockx.LockOption) (lockx.Lock, error) {
	o := lockx.LockOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	ttl := o.TTL
	if ttl <= 0 {
		ttl = lockx.DefaultTTL
	}

	ml := l.rs.NewMutex(key, redsync.WithExpiry(ttl))
	if err := ml.TryLockContext(ctx); err != nil {
		l.logger.ErrorContext(ctx, "lockx/redis try lock failed", logx.Err(err), logx.String("key", key))
		return nil, fmt.Errorf("lockx/redis: try lock %q: %w", key, err)
	}
	return &LockImpl{
		mtx:    ml,
		key:    key,
		ttl:    ttl,
		name:   l.name,
		logger: l.logger,
	}, nil
}

func (l *LockerImpl) Name() string { return "redis" }
func (l *LockerImpl) Close() error { return l.client.Close() }

// ===== Lock 实现 =====

func (lk *LockImpl) Unlock(ctx context.Context) error {
	_, span := lk.tracer.Start(ctx, "lock.redis.release")
	defer span.End()
	if _, err := lk.mtx.UnlockContext(ctx); err != nil {
		span.RecordError(err)
		lk.logger.ErrorContext(ctx, "lockx/redis unlock failed", logx.Err(err), logx.String("key", lk.key))
		return fmt.Errorf("lockx/redis: unlock %q: %w", lk.key, err)
	}
	lk.logger.InfoContext(ctx, "lockx/redis lock released", logx.String("key", lk.key))
	return nil
}

func (lk *LockImpl) Renew(ctx context.Context) error {
	_, span := lk.tracer.Start(ctx, "lock.redis.renew")
	defer span.End()
	if _, err := lk.mtx.ExtendContext(ctx); err != nil {
		span.RecordError(err)
		lk.logger.ErrorContext(ctx, "lockx/redis renew failed", logx.Err(err), logx.String("key", lk.key))
		return fmt.Errorf("lockx/redis: renew %q: %w", lk.key, err)
	}
	lk.logger.InfoContext(ctx, "lockx/redis lock renewed", logx.String("key", lk.key))
	return nil
}

func (lk *LockImpl) Key() string   { return lk.key }
func (lk *LockImpl) Expired() bool { return false }

// ===== 驱动注册 =====

func init() {
	lockx.RegisterCreator("redis", func(cfg *lockx.Config) (lockx.Locker, error) {
		return New(cfg)
	})
}

func withRetry(fn func() error, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(100<<uint(i-1)) * time.Millisecond)
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, lastErr)
}
