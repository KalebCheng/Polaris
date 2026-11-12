package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	bizerrors "polaris/internal/errors"
	"polaris/pkg/redisclient"
)

// incrWithExpiryScript makes the counter and its expiry one atomic unit.
// The TTL guard also repairs keys created by older versions without an expiry.
const incrWithExpiryScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 or redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`

var incrWithExpiry = redis.NewScript(incrWithExpiryScript)

// RateLimitRepository 限流仓库（Redis 实现；失败计数 + 窗口 TTL）
type RateLimitRepository struct{ client *redisclient.Client }

// NewRateLimitRepository 创建限流仓库
func NewRateLimitRepository(client *redisclient.Client) *RateLimitRepository {
	return &RateLimitRepository{client: client}
}

// Incr atomically increments the counter and ensures the window TTL is set.
func (r *RateLimitRepository) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	seconds := int64(ttl / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	n, err := incrWithExpiry.Run(ctx, r.client.Client(), []string{key}, seconds).Int64()
	if err != nil {
		return 0, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return n, nil
}

// Get 读取当前计数（不存在返回 0）
func (r *RateLimitRepository) Get(ctx context.Context, key string) (int64, error) {
	v, err := r.client.Get(ctx, key)
	if err != nil {
		// 键不存在视为 0
		return 0, nil
	}
	n, convErr := strconv.ParseInt(v, 10, 64)
	if convErr != nil {
		return 0, nil
	}
	return n, nil
}

// Reset 清零计数
func (r *RateLimitRepository) Reset(ctx context.Context, key string) error {
	return r.client.Del(ctx, key)
}

// TTL 读取键的剩余存活时间（键不存在返回 0）。
func (r *RateLimitRepository) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := r.client.TTL(ctx, key)
	if err != nil {
		return 0, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

// Keys 按 glob 模式枚举键（SCAN 全量游标）。
//
// 用途：解除登录失败锁定需要按「组织标识 + 账号」定位 `polaris:login_fail:*` 键。
// 注意 SCAN 的 MATCH 是 glob 语法（`*`/`?`/`[]`），**不能**直接把用户名拼进模式后据结果删除，
// 调用方必须对返回的键做精确段比对（见 service/unlock.go）。
func (r *RateLimitRepository) Keys(ctx context.Context, pattern string) ([]string, error) {
	keys, err := r.client.Scan(ctx, pattern, 200)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return keys, nil
}

// Expire 重置键的存活时间（键不存在时无副作用）。
func (r *RateLimitRepository) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if _, err := r.client.Expire(ctx, key, ttl); err != nil {
		return bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return nil
}
