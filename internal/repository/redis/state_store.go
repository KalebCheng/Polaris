package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/pkg/redisclient"
)

// StateKeyPrefix OAuth/SSO state 键前缀
const StateKeyPrefix = "polaris:oauth:state:"

// StateStore OAuth state 暂存（Redis 实现；一次性消费）
type StateStore struct{ client *redisclient.Client }

// NewStateStore 创建 state 存储
func NewStateStore(client *redisclient.Client) *StateStore { return &StateStore{client: client} }

// Save 保存 state 载荷（带 TTL）
func (s *StateStore) Save(ctx context.Context, state string, payload []byte, ttl time.Duration) error {
	key := StateKeyPrefix + state
	if err := s.client.Set(ctx, key, payload, ttl); err != nil {
		logger.Error("oauth state save failed",
			"key", key, "ttl", ttl.String(), "redis_addr", s.client.Addr(), "redis_db", s.client.DB(), "error", err)
		return bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	logger.Info("oauth state saved",
		"key", key, "ttl", ttl.String(), "bytes", len(payload), "redis_addr", s.client.Addr(), "redis_db", s.client.DB())
	return nil
}

// Take 取出并删除 state 载荷（GETDEL 语义，保证一次性）。
func (s *StateStore) Take(ctx context.Context, state string) ([]byte, error) {
	key := StateKeyPrefix + state
	// 先读 TTL，再 GETDEL：回调失败时能区分“key 从未写入/已过期”和“key 在但读不到”。
	ttl, _ := s.client.Client().TTL(ctx, key).Result()
	v, err := s.client.Take(ctx, key)
	if err != nil {
		if err == redis.Nil {
			logger.Warn("oauth state not found",
				"key", key, "ttl_before", ttl.String(), "redis_addr", s.client.Addr(), "redis_db", s.client.DB())
			return nil, bizerrors.ErrSSOStateInvalid
		}
		logger.Error("oauth state take failed",
			"key", key, "ttl_before", ttl.String(), "redis_addr", s.client.Addr(), "redis_db", s.client.DB(), "error", err)
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	logger.Info("oauth state consumed",
		"key", key, "ttl_before", ttl.String(), "bytes", len(v), "redis_addr", s.client.Addr(), "redis_db", s.client.DB())
	return v, nil
}
