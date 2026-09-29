# Redis 驱动功能矩阵（lockx）

> 代码依据：`redis.go`、`register.go`（lockx/redis）
> 更新日期：2026-09-09 | 驱动注册名：`redis` | 客户端库：`github.com/go-redsync/redsync/v4` + `github.com/go-redis/redis/v8`

## 1. 基础命令矩阵（Locker/Lock 接口）

### Locker 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Lock(ctx, key, opts...)` | `redsync.NewMutex(key).LockWithOptions()` | ✅ 支持 | 阻塞获取锁，带 TTL 和看门狗续期选项 |
| `TryLock(ctx, key, opts...)` | `redsync.NewMutex(key).TryLockWithOptions()` | ✅ 支持 | 非阻塞获取锁，失败立即返回 |
| `Name()` | 返回驱动名 | ✅ 支持 | 始终返回 `"redis"` |
| `Close()` | 关闭 redsync pool + redis client | ✅ 支持 | — |

### Lock 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Unlock(ctx)` | `mutex.Unlock()` | ✅ 支持 | 释放锁（验证 nonce 防止误释放） |
| `Renew(ctx)` | 看门狗 goroutine 自动续期 | ✅ 支持 | `WithWatchdog` 开启时自动续期；需手动调用时通过 redsync 续期 |
| `Key()` | 返回锁 key | ✅ 支持 | — |
| `Expired()` | 检查 mutex 是否已过期 | ✅ 支持 | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `lock.redis.{OPERATION}` |
| `db.system` | `redis` |
| `lock.key` | 锁 key |
| `lock.ttl_ms` | TTL 毫秒数 |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| 单机 Redis | ✅ 支持 | `cfg.Addr` 直连 |
| Sentinel 哨兵 | ✅ 支持 | 使用 `redis.FailoverClient`，multi-redsync 跨主从 |
| Cluster 集群 | ✅ 支持 | `redis.NewClusterClient` + multi-redsync |
| TLS | ✅ 支持 | `cfg.TLSEnabled` → `MinVersion: TLS1.2` |
| 连接重试 | ✅ 支持 | `withRetry` 3 次退避重试 |
| 分布式互斥（multi-redsync） | ✅ 支持 | redsync 默认使用多个 redis 实例进行 Quorum 互斥 |

## 3. 配置字段生效状态（lockx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | 单机地址 |
| `Password` | ✅ 生效 | — |
| `Namespace` | ✅ 生效 | 锁 key 前缀，避免不同业务线冲突 |
| `TTL` | ✅ 生效 | 锁超时时间，默认 30s |
| `Watchdog` | ✅ 生效 | `WithWatchdog` 开关，开启后自动续期 |
| `Interval` | ✅ 生效 | 看门狗续期间隔，默认 3s |
| `Retry.MaxAttempts` | ✅ 生效 | 获取锁失败重试次数 |
| `Retry.BackoffMs` | ✅ 生效 | 重试退避间隔 |
| `TLSEnabled` | ✅ 生效 | — |
| `EnableTrace` | ⚠️ 部分 | span 始终上报 |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| 锁超时自定义 | ✅ 支持 | `WithTTL` 选项 |
| 多实例 Quorum | ✅ 支持 | redsync 原生 multi-redsync |
| 锁可重入 | ❌ 不支持 | redsync 不支持可重入锁 |
| 条件锁（CAS） | ❌ 不支持 | — |
| 锁状态查询 | ⚠️ 部分 | `Expired()` 可查询，无 `IsLocked()` 直接接口 |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 分布式互斥锁 | ✅ 完整支持 | redsync 工业级实现，Quorum 保证 |
| 任务调度锁（防重复执行） | ✅ 完整支持 | TryLock + TTL |
| 分布式计数互斥 | ✅ 完整支持 | 持锁期间执行 Incr |
| Leader Election | ✅ 完整支持 | 单一持有者语义 |
| 长耗时操作锁 | ✅ 完整支持 | Watchdog 自动续期，避免业务逻辑慢导致锁提前释放 |
| 秒杀库存扣减 | ⚠️ 部分 | 持锁扣减可行，但高并发下锁竞争是瓶颈（非最优方案） |
| 分布式锁重试 | ✅ 支持 | 内置 Retry 退避 |
| 多锁排序（避免死锁） | ❌ 不支持 | 需业务侧实现锁顺序 |
| 健康探针 | ✅ 支持 | PING |
| 链路追踪 | ✅ 支持 | `lock.redis.*` span |

## 6. 已知限制与风险

1. **不可重入**：redsync 不支持同一 goroutine 重复获取同一把锁，嵌套锁会死锁。
2. **Watchdog 依赖 goroutine**：看门狗续期在后台 goroutine 运行，进程异常退出时 goroutine 被杀，锁按 TTL 自然过期——不会死锁，但可能有短暂窗口期其他进程无法获取。
3. **Redis 宕机导致锁丢失**：若 Redis 集群脑裂（少数派写成功但多数派未收到），可能产生多个持有者（redsync Quorum 机制缓解但无法完全消除）。
4. **锁 Key 命名冲突**：`Namespace` 前缀必须业务侧约定，不同业务线使用相同 namespace 会冲突。
5. **TTL 语义陷阱**：`TTL` 是锁最大持有时间，业务逻辑必须在 TTL 内完成；超时则锁自动释放，可能出现竞态。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/lockx"
    _ "github.com/gospacex/lockx/redis" // 注册驱动
)

locker, _ := lockx.New(&lockx.Config{
    Name: "default", Driver: "redis",
    Addr: "127.0.0.1:6379", Password: "redis123456",
    Namespace: "myapp", TTL: 30 * time.Second,
})

ctx := context.Background()
mu, _ := locker.Lock(ctx, "my_resource_key",
    lockx.WithTTL(30*time.Second),
    lockx.WithWatchdog(3*time.Second),
)
defer mu.Unlock(ctx)
// 执行业务逻辑...
```
