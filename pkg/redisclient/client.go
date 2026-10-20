// Package redisclient 封装 Redis 客户端，用于会话、临时态与限流计数。
package redisclient

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// takeScriptSource 原子读取并删除 key，兼容不支持 GETDEL 的 Redis（Redis < 6.2 / 部分云代理）。
const takeScriptSource = `
local value = redis.call('GET', KEYS[1])
if value then
  redis.call('DEL', KEYS[1])
end
return value
`

var takeScript = redis.NewScript(takeScriptSource)

// Client Redis 客户端
type Client struct {
	client redis.Client
	cfg    Config
}

// Config Redis 配置
type Config struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{PoolSize: 10, MinIdleConns: 2}
}

// New 创建 Redis 客户端
func New(cfg Config) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}

	return &Client{client: *client, cfg: cfg}, nil
}

// Client 获取底层 redis 客户端
func (c *Client) Client() *redis.Client { return &c.client }

// Addr 返回当前连接的 Redis 地址（诊断日志用，不含密码）。
func (c *Client) Addr() string { return c.cfg.Addr }

// DB 返回当前连接的 Redis DB（诊断日志用）。
func (c *Client) DB() int { return c.cfg.DB }

// Take 原子读取并删除 key；key 不存在时返回 redis.Nil。
//
// 不使用 GETDEL：阿里云 Redis 等部分实例/代理不支持该命令（ERR unknown command 'getdel'）。
// 这里用 EVAL 执行 GET + DEL，Redis 2.6+ 均支持，同时保持一次性消费的原子性。
func (c *Client) Take(ctx context.Context, key string) ([]byte, error) {
	value, err := takeScript.Run(ctx, &c.client, []string{key}).Result()
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, redis.Nil
	}
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return nil, fmt.Errorf("redis take: unexpected value type %T", value)
	}
}

// Ping 健康检查
func (c *Client) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

// Close 关闭连接
func (c *Client) Close() error { return c.client.Close() }

// Health 健康状态
func (c *Client) Health(ctx context.Context) (bool, error) {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return false, err
	}
	return true, nil
}

// Get 获取键值
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set 设置键值
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Del 删除键
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// Exists 检查键是否存在
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Exists(ctx, keys...).Result()
}

// Expire 设置过期时间
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.client.Expire(ctx, key, ttl).Result()
}

// TTL 获取剩余过期时间
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// Incr 自增
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

// Scan 按前缀扫描键（用于会话索引反查）
func (c *Client) Scan(ctx context.Context, match string, count int64) ([]string, error) {
	var cursor uint64
	var keys []string
	for {
		part, next, err := c.client.Scan(ctx, cursor, match, count).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, part...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// SAdd 集合添加成员
func (c *Client) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

// SRem 集合移除成员
func (c *Client) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

// SMembers 集合成员
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// SetNX 仅在键不存在时设置
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, key, value, ttl).Result()
}
