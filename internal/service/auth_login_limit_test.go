package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// fakeLimiter 内存版限流仓库：记录计数、窗口 TTL 与冻结（Expire）调用。
type fakeLimiter struct {
	repository.RateLimitRepository
	counts  map[string]int64
	ttls    map[string]time.Duration
	expires map[string]time.Duration
}

func newFakeLimiter() *fakeLimiter {
	return &fakeLimiter{
		counts:  map[string]int64{},
		ttls:    map[string]time.Duration{},
		expires: map[string]time.Duration{},
	}
}

func (l *fakeLimiter) Get(_ context.Context, key string) (int64, error) { return l.counts[key], nil }

func (l *fakeLimiter) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	l.counts[key]++
	if _, ok := l.ttls[key]; !ok {
		l.ttls[key] = ttl
	}
	return l.counts[key], nil
}

func (l *fakeLimiter) Reset(_ context.Context, key string) error {
	delete(l.counts, key)
	return nil
}

func (l *fakeLimiter) Expire(_ context.Context, key string, ttl time.Duration) error {
	l.expires[key] = ttl
	return nil
}

// 用户仓库桩：登录失败限制在"校验密码"之前生效，因此只需返回"用户不存在"。
type stubUserRepo struct{ repository.UserRepository }

func (r *stubUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return nil, errors.New("user not found")
}

func (r *stubUserRepo) GetByEmail(context.Context, int64, string) (*repository.User, error) {
	return nil, errors.New("user not found")
}

// 应用级两项配置的解析：0 = 沿用平台默认，非 0 = 覆盖。
func TestSigninFailurePolicyResolution(t *testing.T) {
	platformMax, platformWait := int64(5), 5*time.Minute
	cases := []struct {
		name     string
		app      *repository.Application
		wantMax  int64
		wantWait time.Duration
	}{
		{"组织未配置应用 → 平台默认", nil, platformMax, platformWait},
		{"应用未设置（0）→ 平台默认", &repository.Application{}, platformMax, platformWait},
		{"应用覆盖两项", &repository.Application{SigninErrorLimit: 3, SigninRetryWaitMinutes: 30}, 3, 30 * time.Minute},
		{"只覆盖次数，等待沿用平台默认", &repository.Application{SigninErrorLimit: 9}, 9, platformWait},
		{"只覆盖等待，次数沿用平台默认", &repository.Application{SigninRetryWaitMinutes: 1}, platformMax, time.Minute},
	}
	for _, tc := range cases {
		gotMax, gotWait := signinFailurePolicy(tc.app, platformMax, platformWait)
		if gotMax != tc.wantMax || gotWait != tc.wantWait {
			t.Fatalf("%s: 期望 (%d, %v)，得到 (%d, %v)", tc.name, tc.wantMax, tc.wantWait, gotMax, gotWait)
		}
	}
}

// 冻结自"达到次数上限"那一刻开始：此前只计数，达到上限时按等待时间重置窗口。
func TestCountFailureFreezesWhenLimitReached(t *testing.T) {
	limiter := newFakeLimiter()
	svc := &AuthService{rateLimiter: limiter}
	key, wait, max := "polaris:login_fail:acme:app-a:admin:10.0.0.1", 15*time.Minute, int64(2)

	svc.countFailure(context.Background(), key, max, wait)
	if got := limiter.counts[key]; got != 1 {
		t.Fatalf("首次失败应计数 1，得到 %d", got)
	}
	if _, ok := limiter.expires[key]; ok {
		t.Fatal("未达上限不应冻结")
	}
	if limiter.ttls[key] != wait {
		t.Fatalf("计数窗口应为等待时间 %v，得到 %v", wait, limiter.ttls[key])
	}

	svc.countFailure(context.Background(), key, max, wait)
	if limiter.counts[key] != max {
		t.Fatalf("第二次失败应计数 %d，得到 %d", max, limiter.counts[key])
	}
	if limiter.expires[key] != wait {
		t.Fatalf("达到上限应按等待时间 %v 冻结，得到 %v", wait, limiter.expires[key])
	}
}

// 未启用限制（次数或等待时间为 0）时不计数，避免"配了但不生效/生效了却不可见"。
func TestCountFailureNoopWithoutLimitOrWait(t *testing.T) {
	limiter := newFakeLimiter()
	svc := &AuthService{rateLimiter: limiter}
	svc.countFailure(context.Background(), "k", 0, 15*time.Minute)
	svc.countFailure(context.Background(), "k", 5, 0)
	if len(limiter.counts) != 0 {
		t.Fatalf("未启用限制时不应写入计数，得到 %v", limiter.counts)
	}
}

// 失败计数按「组织 + 应用 + 账号 + IP」隔离：各应用的阈值互不干扰。
func TestLoginRateKeyIsolatedPerApplication(t *testing.T) {
	a := loginRateKey("acme", "app-a", "admin", "10.0.0.1")
	b := loginRateKey("acme", "app-b", "admin", "10.0.0.1")
	if a == b {
		t.Fatalf("不同应用应使用不同计数键：%s", a)
	}
	if !strings.Contains(a, ":app-a:") {
		t.Fatalf("计数键应包含应用标识：%s", a)
	}
}

// 应用级阈值优先于平台默认：平台默认 5 次，应用配 2 次 → 计数 2 时即 429。
func TestLoginUsesApplicationFailureLimit(t *testing.T) {
	svc, limiter := newLoginLimitService(repository.Application{
		ID: 1, InstitutionID: 1, Code: "app-a", Name: "App A", State: "active",
		SigninErrorLimit: 2, SigninRetryWaitMinutes: 15,
	})
	limiter.counts[loginRateKey("acme", "app-a", "admin", "10.0.0.1")] = 2

	err := loginExpectingError(t, svc, "app-a")
	if err != bizerrors.ErrTooManyRequests {
		t.Fatalf("应用级上限 2 次：期望 429，得到 %v", err)
	}
}

// 应用未设置（0）时沿用平台默认 5 次：计数 2 仍未锁定（继续走凭证校验）。
func TestLoginFallsBackToPlatformLimit(t *testing.T) {
	svc, limiter := newLoginLimitService(repository.Application{
		ID: 1, InstitutionID: 1, Code: "app-a", Name: "App A", State: "active",
	})
	limiter.counts[loginRateKey("acme", "app-a", "admin", "10.0.0.1")] = 2

	err := loginExpectingError(t, svc, "app-a")
	if err == bizerrors.ErrTooManyRequests {
		t.Fatal("未达平台默认上限（5 次）不应被锁定")
	}
	if err != bizerrors.ErrInvalidCredentials {
		t.Fatalf("期望继续凭证校验（401），得到 %v", err)
	}
}

// newLoginLimitService 组装最小可用的 AuthService（平台默认 5 次 / 5 分钟）。
func newLoginLimitService(app repository.Application) (*AuthService, *fakeLimiter) {
	apps := &stubAppRepo{byName: map[string]*repository.Application{"1/" + app.Code: &app}}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme"}}}
	svc := newPolicyService(apps, orgs, "built-in")
	svc.users = &stubUserRepo{}
	limiter := newFakeLimiter()
	svc.rateLimiter = limiter
	svc.loginMax, svc.loginWindow = 5, 5*time.Minute
	return svc, limiter
}

func loginExpectingError(t *testing.T, svc *AuthService, application string) error {
	t.Helper()
	_, _, err := svc.Login(context.Background(), auth.LoginRequest{
		Institution: "acme", Application: application, Username: "admin", Password: "wrong-password",
	}, "acme.example.com", "10.0.0.1", "test-agent")
	return err
}
