// lockx/config.go — 统一配置集成（configx + ReadAndCreate）
package lockx

import (
	"fmt"
	"os"
	"time"

	"github.com/gospacex/configx"
)

// StandaloneConfig 单实例锁配置（本地/viper，适用于单机部署）
type StandaloneConfig struct {
	Driver    string        `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name      string        `yaml:"name" json:"name" mapstructure:"name"`
	Addr      string        `yaml:"addr" json:"addr" mapstructure:"addr"`
	Password  string        `yaml:"password" json:"password" mapstructure:"password"`
	Namespace string        `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	TTL       time.Duration `yaml:"ttl" json:"ttl" mapstructure:"ttl"`
	Retry     RetryConfig   `yaml:"retry" json:"retry" mapstructure:"retry"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// ClusterConfig 集群锁配置（Nacos/Apollo 远程配置中心，适用于多节点部署）
type ClusterConfig struct {
	Driver    string        `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name      string        `yaml:"name" json:"name" mapstructure:"name"`
	Addr      string        `yaml:"addr" json:"addr" mapstructure:"addr"`
	Password  string        `yaml:"password" json:"password" mapstructure:"password"`
	Namespace string        `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	TTL       time.Duration `yaml:"ttl" json:"ttl" mapstructure:"ttl"`
	Retry     RetryConfig   `yaml:"retry" json:"retry" mapstructure:"retry"`
	// WatchEnabled 是否监听配置变更
	WatchEnabled bool `yaml:"watch_enabled" json:"watch_enabled" mapstructure:"watch_enabled"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// LockSection 对应 configx.yaml 中的 lockx 配置节
type LockSection struct {
	Standalone []*StandaloneConfig `yaml:"standalone"`
	Cluster    []*ClusterConfig    `yaml:"cluster"`
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
func ReadConfig(opts ...configx.ConfigOptions) error {
	_, err := configx.ReadConfig(opts...)
	if err != nil {
		return err
	}
	return nil
}

// ReadAndCreate 从 configx 读取 lockx 配置并创建锁实例
func ReadAndCreate(opts ...configx.ConfigOptions) (Locker, error) {
	c, err := configx.ReadConfig(opts...)
	if err != nil {
		return nil, fmt.Errorf("lockx: failed to read config: %w", err)
	}

	section := &LockSection{}
	if err := c.Unmarshal("lockx", section); err != nil {
		return nil, fmt.Errorf("lockx: failed to unmarshal lockx section: %w", err)
	}

	var cfg *Config
	for _, cc := range section.Cluster {
		cfg = clusterToConfig(cc)
		break
	}
	if cfg == nil {
		for _, sc := range section.Standalone {
			cfg = standaloneToConfig(sc)
			break
		}
	}

	if cfg == nil {
		return nil, fmt.Errorf("lockx: no lockx instance configured")
	}

	return New(cfg)
}

// MustReadAndCreate 类似 ReadAndCreate，失败时 panic
func MustReadAndCreate(opts ...configx.ConfigOptions) Locker {
	locker, err := ReadAndCreate(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lockx: failed to initialize locker: %v\n", err)
		os.Exit(1)
	}
	return locker
}

func standaloneToConfig(s *StandaloneConfig) *Config {
	return &Config{
		Driver:      s.Driver,
		Name:        s.Name,
		Addr:        s.Addr,
		Password:    s.Password,
		Namespace:   s.Namespace,
		TTL:         s.TTL,
		Retry:       s.Retry,
		EnableTrace: s.EnableTrace,
	}
}

func clusterToConfig(c *ClusterConfig) *Config {
	return &Config{
		Driver:      c.Driver,
		Name:        c.Name,
		Addr:        c.Addr,
		Password:    c.Password,
		Namespace:   c.Namespace,
		TTL:         c.TTL,
		Retry:       c.Retry,
		EnableTrace: c.EnableTrace,
	}
}
