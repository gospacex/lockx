// lockx: 分布式锁统一接口，支持 redis / etcd / consul 驱动
package lockx

import (
	"context"
	"fmt"
	"time"

	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
)

// Locker 分布式锁接口
type Locker interface {
	// Lock 获取锁（阻塞，带超时）
	Lock(ctx context.Context, key string, opts ...LockOption) (Lock, error)
	// TryLock 尝试获取锁（非阻塞）
	TryLock(ctx context.Context, key string, opts ...LockOption) (Lock, error)
	// Name 返回驱动名称
	Name() string
	// Close 关闭连接
	Close() error
}

// Lock 已获取的锁
type Lock interface {
	// Unlock 释放锁
	Unlock(ctx context.Context) error
	// Renew 续期锁（看门狗）
	Renew(ctx context.Context) error
	// Key 返回锁 key
	Key() string
	// Expired 返回是否已过期
	Expired() bool
}

// LockOption 锁选项
type LockOption func(*LockOptions)

// LockOptions 锁选项配置
type LockOptions struct {
	TTL       time.Duration
	Watchdog  bool
	Interval  time.Duration
	Namespace string
}

// WithTTL 设置锁超时时间（默认 30s）
func WithTTL(ttl time.Duration) LockOption {
	return func(o *LockOptions) { o.TTL = ttl }
}

// WithWatchdog 启用看门狗自动续期
func WithWatchdog(interval time.Duration) LockOption {
	return func(o *LockOptions) {
		o.Watchdog = true
		if interval <= 0 {
			interval = 3 * time.Second
		}
		o.Interval = interval
	}
}

// WithNamespace 设置命名空间
func WithNamespace(ns string) LockOption {
	return func(o *LockOptions) { o.Namespace = ns }
}

// DefaultTTL 默认锁超时
const DefaultTTL = 30 * time.Second

// Config 锁配置
type Config struct {
	Driver      string
	Name        string
	Addr        string
	Password    string
	Namespace   string
	TTL         time.Duration
	Retry       RetryConfig
	TLSEnabled  bool
	EnableTrace bool
}

// RetryConfig 重试配置
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int
	BackoffMs   int
}

// FillDefaults 填充默认值
func (c *Config) FillDefaults() {
	if c.TTL <= 0 {
		c.TTL = DefaultTTL
	}
	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BackoffMs <= 0 {
		c.Retry.BackoffMs = 100
	}
	if c.Name == "" {
		c.Name = "default"
	}
}

// ===== Factory =====

var creators = map[string]func(*Config) (Locker, error){}

// RegisterCreator 注册锁驱动创建函数
func RegisterCreator(name string, creator func(*Config) (Locker, error)) {
	creators[name] = creator
}

// New 创建锁实例
func New(cfg *Config) (Locker, error) {
	cfg.FillDefaults()
	// 接入 hubx 统一调用链开关：组件自身配置为主开关，
	// 全局总闸 traceflag.SetEnabled(false) 可一键熔断 lockx。
	cfg.EnableTrace = traceflag.Apply("lockx", cfg.EnableTrace)

	creator, ok := creators[cfg.Driver]
	if !ok {
		return nil, fmt.Errorf("lockx: driver %q not registered; make sure to import the driver package", cfg.Driver)
	}

	locker, err := creator(cfg)
	if err != nil {
		return nil, fmt.Errorf("lockx: create locker %q: %w", cfg.Driver, err)
	}
	return locker, nil
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
// 详见 lockx/config.go 中的 ReadConfig 与 ReadAndCreate
