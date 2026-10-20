package redis

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"

	"polaris/pkg/redisclient"
)

// CaptchaStore 图形验证码答案存储（Redis 实现）。
// 使用 Redis 而非库内置内存存储，保证多实例部署下校验一致。
type CaptchaStore struct {
	client *redisclient.Client
	ttl    time.Duration
}

// NewCaptchaStore 创建验证码存储
func NewCaptchaStore(client *redisclient.Client, ttl time.Duration) *CaptchaStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CaptchaStore{client: client, ttl: ttl}
}

func captchaKey(id string) string { return "polaris:captcha:" + id }

// Set 写入答案
func (s *CaptchaStore) Set(id string, value string) error {
	return s.client.Set(context.Background(), captchaKey(id), value, s.ttl)
}

// Get 读取答案（clear=true 时一次性消费）
func (s *CaptchaStore) Get(id string, clear bool) string {
	ctx := context.Background()
	if clear {
		value, err := s.client.Take(ctx, captchaKey(id))
		if err != nil {
			return ""
		}
		return string(value)
	}
	value, err := s.client.Get(ctx, captchaKey(id))
	if err != nil {
		return ""
	}
	return value
}

// Verify 校验答案
func (s *CaptchaStore) Verify(id, answer string, clear bool) bool {
	stored := s.Get(id, clear)
	return stored != "" && stored == answer
}

var _ base64Captcha.Store = (*CaptchaStore)(nil)
