package redisclient

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 回归：生产 Redis 不支持 GETDEL（ERR unknown command 'getdel'）。
// 一次性消费必须走 EVAL 的 GET+DEL，不能再退回 GETDEL。
func TestTakeScriptDoesNotUseGetDel(t *testing.T) {
	script := strings.ToLower(takeScriptSource)
	if strings.Contains(script, "getdel") {
		t.Fatal("take script 不能使用 GETDEL：阿里云 Redis 等实例会报 unknown command")
	}
	if !strings.Contains(script, "get") || !strings.Contains(script, "del") {
		t.Fatalf("take script 应执行 GET + DEL，实际 %q", script)
	}
}

// 本地集成测试：需要显式提供 Redis 地址，CI 无 Redis 时跳过。
func TestTakeAtomicDelete(t *testing.T) {
	addr := os.Getenv("POLARIS_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POLARIS_TEST_REDIS_ADDR to run the Redis integration test")
	}
	c, err := New(Config{Addr: addr, Password: os.Getenv("POLARIS_TEST_REDIS_PASSWORD")})
	if err != nil {
		t.Fatalf("连接 Redis: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx := context.Background()
	key := "polaris:test:take:" + time.Now().Format("20060102150405.000000000")
	if err := c.Set(ctx, key, "payload", time.Minute); err != nil {
		t.Fatalf("写入: %v", err)
	}
	value, err := c.Take(ctx, key)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if string(value) != "payload" {
		t.Fatalf("Take = %q, want payload", value)
	}
	if _, err := c.Take(ctx, key); err != redis.Nil {
		t.Fatalf("第二次 Take 应返回 redis.Nil，实际 %v", err)
	}
}
