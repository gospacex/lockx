package lockx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== 选项函数：纯逻辑，钉住语义不被后续重构改坏 =====

func TestWithTTL(t *testing.T) {
	o := &LockOptions{}
	WithTTL(5 * time.Second)(o)
	if o.TTL != 5*time.Second {
		t.Errorf("TTL = %v, want 5s", o.TTL)
	}
}

func TestWithWatchdog(t *testing.T) {
	o := &LockOptions{}
	WithWatchdog(2 * time.Second)(o)
	if !o.Watchdog {
		t.Error("Watchdog 应为 true")
	}
	if o.Interval != 2*time.Second {
		t.Errorf("Interval = %v, want 2s", o.Interval)
	}
}

// TestWithWatchdogNonPositiveInterval 钉住边界：interval <= 0 时回落 3s，
// 而不是写入一个会让看门狗空转/立即触发的非法值。
func TestWithWatchdogNonPositiveInterval(t *testing.T) {
	for _, in := range []time.Duration{0, -1 * time.Second} {
		o := &LockOptions{}
		WithWatchdog(in)(o)
		if o.Interval != 3*time.Second {
			t.Errorf("WithWatchdog(%v): Interval = %v, want 3s（非正值应回落默认）", in, o.Interval)
		}
	}
}

func TestWithNamespace(t *testing.T) {
	o := &LockOptions{}
	WithNamespace("tenant-a")(o)
	if o.Namespace != "tenant-a" {
		t.Errorf("Namespace = %q, want %q", o.Namespace, "tenant-a")
	}
}

// ===== Config.FillDefaults =====

func TestConfigFillDefaults(t *testing.T) {
	c := &Config{}
	c.FillDefaults()

	if c.TTL != DefaultTTL {
		t.Errorf("TTL = %v, want %v", c.TTL, DefaultTTL)
	}
	if c.Retry.MaxAttempts != 3 {
		t.Errorf("Retry.MaxAttempts = %d, want 3", c.Retry.MaxAttempts)
	}
	if c.Retry.BackoffMs != 100 {
		t.Errorf("Retry.BackoffMs = %d, want 100", c.Retry.BackoffMs)
	}
	if c.Name != "default" {
		t.Errorf("Name = %q, want %q", c.Name, "default")
	}
}

// TestConfigFillDefaultsKeepsExplicitValues 钉住「显式配置不被默认值覆盖」——
// 否则用户配了 ttl=1s 会静默变成 30s。
func TestConfigFillDefaultsKeepsExplicitValues(t *testing.T) {
	c := &Config{
		TTL:   1 * time.Second,
		Name:  "svc-a",
		Retry: RetryConfig{MaxAttempts: 7, BackoffMs: 250},
	}
	c.FillDefaults()

	if c.TTL != time.Second {
		t.Errorf("TTL = %v, want 1s（显式值不应被覆盖）", c.TTL)
	}
	if c.Name != "svc-a" {
		t.Errorf("Name = %q, want svc-a", c.Name)
	}
	if c.Retry.MaxAttempts != 7 || c.Retry.BackoffMs != 250 {
		t.Errorf("Retry = %+v, want {MaxAttempts:7 BackoffMs:250}", c.Retry)
	}
}

// ===== 工厂：未注册 driver 的错误必须可定位 =====

func TestNewUnregisteredDriver(t *testing.T) {
	_, err := New(&Config{Driver: "definitely-not-registered"})
	if err == nil {
		t.Fatal("未注册 driver 应返回 error，实际 nil")
	}
	// 错误信息要能指引调用方去 import 驱动包
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("错误信息应含 %q，实际: %v", "not registered", err)
	}
}

// fakeLocker 仅用于验证工厂接线，不依赖任何外部中间件。
type fakeLocker struct{}

func (f *fakeLocker) Lock(ctx context.Context, key string, opts ...LockOption) (Lock, error) {
	return nil, nil
}
func (f *fakeLocker) TryLock(ctx context.Context, key string, opts ...LockOption) (Lock, error) {
	return nil, nil
}
func (f *fakeLocker) Name() string { return "fake" }
func (f *fakeLocker) Close() error { return nil }

func TestFactoryCreateAfterRegister(t *testing.T) {
	const drv = "test-factory-lockx"
	RegisterCreator(drv, func(c *Config) (Locker, error) { return &fakeLocker{}, nil })

	got, err := New(&Config{Driver: drv})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got == nil {
		t.Fatal("New 返回的 Locker 为 nil")
	}
}

// TestFactoryConcurrentAccess 钉住 creators map 的并发安全性。
//
// 与 coordx 同源缺陷：REVIEW-REPORT.md 第二优先级第 6 条点名 lockx 要给
// factory map 加锁，但第八节已修复清单未覆盖。未加锁时并发注册 + 并发查找
// 会触发 "fatal error: concurrent map read and map write"（不可 recover）。
//
// 读写两端都必须走受保护的 API（RegisterCreator / New）。
// ⚠️ 不要在测试里直接读 creators 裸 map：那样会绕过 creatorsMu，
// 测到的就不是修复后的路径，用例会永远红且无法反映真实修复效果。
// New() 内部的 traceflag.Apply 是并发安全的（globalEnabled atomic.Bool +
// compMu sync.RWMutex），不会引入无关竞态。
func TestFactoryConcurrentAccess(t *testing.T) {
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		drv := "concurrent-lockx-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		go func(d string) {
			defer wg.Done()
			RegisterCreator(d, func(c *Config) (Locker, error) { return &fakeLocker{}, nil })
		}(drv)

		// 命中与否都允许（注册与查找并发，顺序不定），只要不 panic / 不 race
		go func(d string) {
			defer wg.Done()
			_, _ = New(&Config{Driver: d})
		}(drv)
	}
	wg.Wait()
}
