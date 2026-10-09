// lockx: 分布式锁统一接口，支持 redis / etcd / consul 驱动
package lockx

import (
	"context"
	"fmt"
	"sync"
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
//
// 为什么逐字段都带三套 tag：本结构体是 configx 三个通道（yaml 本地文件 /
// json 远程配置中心 / mapstructure viper）的共同落点。缺 mapstructure tag 时
// viper 只能按**字段名**匹配，`enable_trace` / `max_attempts` 这类蛇形键解码
// 不进来 —— 不报错，只是永远取零值，表现为「明明写了开关却没生效」。
// 与 config.go 里的 StandaloneConfig / ClusterConfig 保持同一套键名。
type Config struct {
	Driver      string        `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name        string        `yaml:"name" json:"name" mapstructure:"name"`
	Addr        string        `yaml:"addr" json:"addr" mapstructure:"addr"`
	Password    string        `yaml:"password" json:"password" mapstructure:"password"`
	Namespace   string        `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	TTL         time.Duration `yaml:"ttl" json:"ttl" mapstructure:"ttl"`
	Retry       RetryConfig   `yaml:"retry" json:"retry" mapstructure:"retry"`
	TLSEnabled  bool          `yaml:"tls_enabled" json:"tls_enabled" mapstructure:"tls_enabled"`
	EnableTrace bool          `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	MaxAttempts int  `yaml:"max_attempts" json:"max_attempts" mapstructure:"max_attempts"`
	BackoffMs   int  `yaml:"backoff_ms" json:"backoff_ms" mapstructure:"backoff_ms"`
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

// creatorsMu 保护下面的 creators map。
//
// 背景：REVIEW-REPORT.md 第二优先级第 6 条点名 lockx 要给 factory map 加锁，
// 但第八节已修复清单未覆盖。未加锁时并发注册 + 并发查找会触发 Go 运行时的
// "fatal error: concurrent map read and map write" —— 不可 recover 的崩溃级
// 故障（已由 interface_test.go 的 TestFactoryConcurrentAccess 在 -race 下
// 复现，与 coordx 属同源缺陷）。
var creatorsMu sync.RWMutex

var creators = map[string]func(*Config) (Locker, error){}

// RegisterCreator 注册锁驱动创建函数
func RegisterCreator(name string, creator func(*Config) (Locker, error)) {
	creatorsMu.Lock()
	defer creatorsMu.Unlock()
	creators[name] = creator
}

// New 创建锁实例
func New(cfg *Config) (Locker, error) {
	cfg.FillDefaults()
	// 接入 hubx 统一调用链开关：组件自身配置为主开关，
	// 全局总闸 traceflag.SetEnabled(false) 可一键熔断 lockx。
	cfg.EnableTrace = traceflag.Apply("lockx", cfg.EnableTrace)

	// 只锁 map 查找，creator 放到锁外调用 —— 避免 creator 内部再注册
	// 其它驱动时自死锁，也缩短持锁时间。
	creatorsMu.RLock()
	creator, ok := creators[cfg.Driver]
	creatorsMu.RUnlock()
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
