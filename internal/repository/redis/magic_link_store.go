package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"polaris/pkg/redisclient"
)

// MagicLinkKeyPrefix 魔法链接一次性令牌键前缀
const MagicLinkKeyPrefix = "polaris:magic:"

// MagicLinkStore 魔法链接令牌存储（Redis 实现，一次性消费）。
// 与验证码一致：读取即删除（GETDEL），保证同一链接只能成功登录一次。
type MagicLinkStore struct{ client *redisclient.Client }

// NewMagicLinkStore 创建魔法链接存储
func NewMagicLinkStore(client *redisclient.Client) *MagicLinkStore {
	return &MagicLinkStore{client: client}
}

// Save 写入令牌载荷（带 TTL）
func (s *MagicLinkStore) Save(ctx context.Context, token string, payload []byte, ttl time.Duration) error {
	return s.client.Set(ctx, MagicLinkKeyPrefix+token, payload, ttl)
}

// Take 取出并删除令牌载荷（GETDEL 语义，避免重放）；不存在时返回 (nil, nil)。
func (s *MagicLinkStore) Take(ctx context.Context, token string) ([]byte, error) {
	value, err := s.client.Client().GetDel(ctx, MagicLinkKeyPrefix+token).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}
