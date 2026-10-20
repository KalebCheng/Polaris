package service

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// fakeLockLimiter 内存版限流仓库：支持 Keys(pattern) 的 glob 语义（与 Redis SCAN MATCH 一致）。
type fakeLockLimiter struct {
	repository.RateLimitRepository
	failures map[string]int64
	ttls     map[string]time.Duration
}

func newFakeLockLimiter() *fakeLockLimiter {
	return &fakeLockLimiter{failures: map[string]int64{}, ttls: map[string]time.Duration{}}
}

func (l *fakeLockLimiter) Get(_ context.Context, key string) (int64, error) {
	return l.failures[key], nil
}

func (l *fakeLockLimiter) Reset(_ context.Context, key string) error {
	delete(l.failures, key)
	return nil
}

func (l *fakeLockLimiter) TTL(_ context.Context, key string) (time.Duration, error) {
	return l.ttls[key], nil
}

func (l *fakeLockLimiter) Keys(_ context.Context, pattern string) ([]string, error) {
	matcher := globToRegexp(pattern)
	out := []string{}
	for key := range l.failures {
		if matcher.MatchString(key) {
			out = append(out, key)
		}
	}
	return out, nil
}

// globToRegexp 把 Redis glob（* / ?）转成等价正则，用于验证"SCAN 会过度匹配"的前提。
func globToRegexp(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		default:
			sb.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}

// lockoutUserRepo 用户仓库桩：GetByName 命中（用来通过"用户存在"校验）。
type lockoutUserRepo struct {
	repository.UserRepository
	found bool
}

func (r *lockoutUserRepo) GetByName(_ context.Context, institutionID int64, name string) (*repository.User, error) {
	if !r.found {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	return &repository.User{ID: 7, InstitutionID: institutionID, Name: name}, nil
}

func newLockoutService(limiter *fakeLockLimiter, apps *stubAppRepo, userFound bool) *LoginLockoutService {
	svc := NewLoginLockoutService(
		limiter,
		&lockoutUserRepo{found: userFound},
		&stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme"}}},
		apps,
		NewAuditRecorder(nil),
		5, 5*time.Minute,
	)
	return svc
}

// 回归：SCAN 的 MATCH 是 glob，用户名含 `*` 会导致过度匹配——必须按段精确比对。
func TestUnlockOnlyRemovesExactUserKeys(t *testing.T) {
	limiter := newFakeLockLimiter()
	exact := "polaris:login_fail:acme:app-a:a*:1.1.1.1"
	overmatched := "polaris:login_fail:acme:app-a:abc:1.1.1.1"
	limiter.failures[exact] = 5
	limiter.failures[overmatched] = 3

	svc := newLockoutService(limiter, &stubAppRepo{}, true)
	cleared, err := svc.Unlock(context.Background(), "admin", 1, "a*")
	if err != nil {
		t.Fatalf("解除失败: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("应只清除 1 条精确匹配的键，实际 %d", cleared)
	}
	if _, ok := limiter.failures[exact]; ok {
		t.Fatal("精确匹配的键应被清除")
	}
	if _, ok := limiter.failures[overmatched]; !ok {
		t.Fatal("glob 过度匹配的键绝不能被清除（会误删他人计数）")
	}
}

// 组织标识既可能是组织名（登录解析后写入），也可能是组织主键（旧客户端直传），两种都要能清。
func TestUnlockCoversNameAndIDKeysAndIsolatesOtherInstitution(t *testing.T) {
	limiter := newFakeLockLimiter()
	byName := "polaris:login_fail:acme:app-a:admin:10.0.0.1"
	byID := "polaris:login_fail:1:app-a:admin:10.0.0.2"
	otherOrg := "polaris:login_fail:beta:app-a:admin:10.0.0.3"
	for _, key := range []string{byName, byID, otherOrg} {
		limiter.failures[key] = 5
	}

	svc := newLockoutService(limiter, &stubAppRepo{}, true)
	cleared, err := svc.Unlock(context.Background(), "admin", 1, "admin")
	if err != nil {
		t.Fatalf("解除失败: %v", err)
	}
	if cleared != 2 {
		t.Fatalf("应清除 2 条（组织名 + 组织主键），实际 %d", cleared)
	}
	if _, ok := limiter.failures[otherOrg]; !ok {
		t.Fatal("其他组织的同名用户计数不应被清除")
	}
}

// 达到应用级阈值即标记为「已限制」，未达阈值只展示计数（与登录时的判定一致）。
func TestLockoutEntriesFlagsBlockedByApplicationThreshold(t *testing.T) {
	limiter := newFakeLockLimiter()
	limiter.failures["polaris:login_fail:acme:app-a:admin:10.0.0.1"] = 2
	limiter.ttls["polaris:login_fail:acme:app-a:admin:10.0.0.1"] = 90 * time.Second
	limiter.failures["polaris:login_fail:acme:app-a:admin:10.0.0.2"] = 1

	apps := &stubAppRepo{byName: map[string]*repository.Application{
		"1/app-a": {ID: 1, InstitutionID: 1, Code: "app-a", SigninErrorLimit: 2, SigninRetryWaitMinutes: 15},
	}}
	svc := newLockoutService(limiter, apps, true)
	entries, err := svc.Entries(context.Background(), 1, "admin")
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("应有 2 条记录，实际 %d", len(entries))
	}
	blocked := map[string]*LoginLockout{}
	for _, entry := range entries {
		blocked[entry.IP] = entry
	}
	if !blocked["10.0.0.1"].Blocked {
		t.Fatal("失败 2 次达到应用阈值 2，应标记为已限制")
	}
	if blocked["10.0.0.1"].FrozenSecondsLeft != 90 {
		t.Fatalf("剩余冻结时长应为 90 秒，实际 %d", blocked["10.0.0.1"].FrozenSecondsLeft)
	}
	if blocked["10.0.0.2"].Blocked {
		t.Fatal("失败 1 次未达阈值，不应标记为已限制")
	}
}

// 用户不存在时解除锁定必须报"未找到"，不能静默成功（否则管理员会以为已处理）。
func TestUnlockRejectsUnknownUser(t *testing.T) {
	svc := newLockoutService(newFakeLockLimiter(), &stubAppRepo{}, false)
	if _, err := svc.Unlock(context.Background(), "admin", 1, "ghost"); err == nil {
		t.Fatal("不存在的用户应返回错误")
	}
}
