package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"polaris/pkg/redisclient"
)

// CodeStore 验证码存储（Redis 实现，一次性消费）
type CodeStore struct{ client *redisclient.Client }

// NewCodeStore 创建验证码存储
func NewCodeStore(client *redisclient.Client) *CodeStore { return &CodeStore{client: client} }

// Save 写入验证码
func (s *CodeStore) Save(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl)
}

// Take 取出并删除验证码（GETDEL 语义，避免重放）
func (s *CodeStore) Take(ctx context.Context, key string) (string, error) {
	value, err := s.client.Client().GetDel(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}
