package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	bizerrors "polaris/internal/errors"
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
	if err := s.client.Set(ctx, StateKeyPrefix+state, payload, ttl); err != nil {
		return bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return nil
}

// Take 取出并删除 state 载荷（GETDEL 语义，保证一次性）。
func (s *StateStore) Take(ctx context.Context, state string) ([]byte, error) {
	v, err := s.client.Client().GetDel(ctx, StateKeyPrefix+state).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, bizerrors.ErrSSOStateInvalid
		}
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return v, nil
}
