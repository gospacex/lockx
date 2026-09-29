// lockx/redis - Redis 分布式锁驱动配置（Standalone + Cluster）
package redis

import (
	"time"

	"github.com/gospacex/lockx"
)

// StandaloneConfig Redis 单实例锁配置
type StandaloneConfig struct {
	Driver     string            `yaml:"driver" json:"driver"`
	Name       string            `yaml:"name" json:"name"`
	Addr       string            `yaml:"addr" json:"addr"`
	Password   string            `yaml:"password" json:"password"`
	DB         int               `yaml:"db" json:"db"`
	PoolSize   int               `yaml:"pool_size" json:"pool_size"`
	TTL        time.Duration     `yaml:"ttl" json:"ttl"`
	Namespace  string            `yaml:"namespace" json:"namespace"`
	Retry      lockx.RetryConfig `yaml:"retry" json:"retry"`
	TLSEnabled bool              `yaml:"tls_enabled" json:"tls_enabled"`
	Timeout    time.Duration     `yaml:"timeout" json:"timeout"`
}

// ClusterConfig Redis 集群锁配置（Sentinel/Cluster）
type ClusterConfig struct {
	Driver     string            `yaml:"driver" json:"driver"`
	Name       string            `yaml:"name" json:"name"`
	Addrs      []string          `yaml:"addrs" json:"addrs"`
	Password   string            `yaml:"password" json:"password"`
	PoolSize   int               `yaml:"pool_size" json:"pool_size"`
	TTL        time.Duration     `yaml:"ttl" json:"ttl"`
	Namespace  string            `yaml:"namespace" json:"namespace"`
	Retry      lockx.RetryConfig `yaml:"retry" json:"retry"`
	TLSEnabled bool              `yaml:"tls_enabled" json:"tls_enabled"`
	Timeout    time.Duration     `yaml:"timeout" json:"timeout"`
	// Sentinel
	SentinelMaster string   `yaml:"sentinel_master" json:"sentinel_master"`
	SentinelAddrs  []string `yaml:"sentinel_addrs" json:"sentinel_addrs"`
	// Cluster
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace"`
}

// ToStandaloneConfig 转换为 lockx.Config
func (s *StandaloneConfig) ToStandaloneConfig(name string) *lockx.Config {
	n := name
	if s.Name != "" {
		n = s.Name
	}
	return &lockx.Config{
		Driver:     "redis",
		Name:       n,
		Addr:       s.Addr,
		Password:   s.Password,
		Namespace:  s.Namespace,
		TTL:        s.TTL,
		Retry:      s.Retry,
		TLSEnabled: s.TLSEnabled,
	}
}

// ToClusterConfig 转换为 lockx.Config
func (c *ClusterConfig) ToClusterConfig(name string) *lockx.Config {
	n := name
	if c.Name != "" {
		n = c.Name
	}
	addr := ""
	if len(c.Addrs) > 0 {
		addr = c.Addrs[0]
	}
	return &lockx.Config{
		Driver:     "redis",
		Name:       n,
		Addr:       addr,
		Password:   c.Password,
		Namespace:  c.Namespace,
		TTL:        c.TTL,
		Retry:      c.Retry,
		TLSEnabled: c.TLSEnabled,
	}
}
