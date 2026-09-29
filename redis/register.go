// lockx/redis - 驱动注册
package redis

import (
	"github.com/gospacex/lockx"
)

func init() {
	lockx.RegisterCreator("redis", func(cfg *lockx.Config) (lockx.Locker, error) {
		return New(cfg)
	})
}
