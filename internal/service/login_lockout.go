package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// LoginLockoutService 登录失败锁定（枚举 + 解除）。
//
// 键结构由 auth_service.loginRateKey 定义（登录链路本身不改）：
//
//	polaris:login_fail:<组织标识>:<应用 code>:<账号>:<IP>
//
// 本服务是它唯一的**管理面**读取/清理入口。为什么需要它：锁定是 Redis TTL 语义，
// 此前管理员既看不到"谁被锁了"，也删不掉键，用户只能等到窗口结束（应用可配 30 分钟以上）。
type LoginLockoutService struct {
	limiter     repository.RateLimitRepository
	users       repository.UserRepository
	orgs        repository.InstitutionRepository
	apps        repository.ApplicationRepository
	audits      *AuditRecorder
	loginMax    int64
	loginWindow time.Duration
}

// NewLoginLockoutService 创建登录锁定服务。
// loginMax / loginWindow 为平台默认阈值（应用级配置优先，与登录时的解析规则一致）。
func NewLoginLockoutService(
	limiter repository.RateLimitRepository,
	users repository.UserRepository,
	orgs repository.InstitutionRepository,
	apps repository.ApplicationRepository,
	audits *AuditRecorder,
	loginMax int64,
	loginWindow time.Duration,
) *LoginLockoutService {
	return &LoginLockoutService{
		limiter: limiter, users: users, orgs: orgs, apps: apps, audits: audits,
		loginMax: loginMax, loginWindow: loginWindow,
	}
}

// loginLockoutPrefix 失败计数键前缀（与 auth_service.loginRateKey 同源）。
const loginLockoutPrefix = "polaris:login_fail:"

// LoginLockout 一条登录失败计数记录（管理面展示；不含 Redis 键名）。
type LoginLockout struct {
	// Application 触发计数的应用 code；空串表示该组织未配置应用
	Application string `json:"application"`
	IP          string `json:"ip"`
	// Failures 当前窗口内的失败次数
	Failures int64 `json:"failures"`
	// FrozenSecondsLeft 距离计数窗口结束（键过期）的秒数
	FrozenSecondsLeft int64 `json:"frozen_seconds_left"`
	// Blocked 是否已达到生效阈值（达到即登录返回 429）
	Blocked bool `json:"blocked"`
}

// Entries 列出某用户当前的失败计数记录（按应用 / IP 维度）。
func (s *LoginLockoutService) Entries(ctx context.Context, institutionID int64, user string) ([]*LoginLockout, error) {
	if s.limiter == nil {
		return []*LoginLockout{}, nil
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	keys, err := s.lockoutKeys(ctx, org.Name, strconv.FormatInt(org.ID, 10), user)
	if err != nil {
		return nil, err
	}
	out := make([]*LoginLockout, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, ":", 6)
		application := parts[3]
		ip := parts[5]
		failures, err := s.limiter.Get(ctx, key)
		if err != nil || failures <= 0 {
			continue
		}
		ttl, _ := s.limiter.TTL(ctx, key)
		max := s.maxFailuresFor(ctx, institutionID, application)
		out = append(out, &LoginLockout{
			Application:       application,
			IP:                ip,
			Failures:          failures,
			FrozenSecondsLeft: int64(ttl / time.Second),
			Blocked:           max > 0 && failures >= max,
		})
	}
	return out, nil
}

// Unlock 清除某用户的全部失败计数记录，返回清理条数。
func (s *LoginLockoutService) Unlock(ctx context.Context, actor string, institutionID int64, user string) (int, error) {
	if _, err := s.users.GetByName(ctx, institutionID, user); err != nil {
		return 0, bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("user", user)
	}
	if s.limiter == nil {
		return 0, nil
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return 0, bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	keys, err := s.lockoutKeys(ctx, org.Name, strconv.FormatInt(org.ID, 10), user)
	if err != nil {
		return 0, err
	}
	cleared := 0
	for _, key := range keys {
		if err := s.limiter.Reset(ctx, key); err != nil {
			return cleared, err
		}
		cleared++
	}
	s.audits.Record(ctx, actor, institutionID, "user.unlock", user, map[string]interface{}{
		"count": cleared,
	})
	return cleared, nil
}

// lockoutKeys 定位该用户的全部计数键。
//
// 两道保险：
//  1. 组织标识既可能是组织名（登录时解析后写入的标识），也可能是组织主键（旧客户端直传），
//     因此两种写法都要扫；
//  2. SCAN 的 MATCH 是 glob，用户名里若含 `*`/`?`/`[]` 会**过度匹配**，
//     所以拿到键后必须按分隔符切段做**精确比对**，只保留第 3 段与第 5 段完全相符的键。
func (s *LoginLockoutService) lockoutKeys(ctx context.Context, orgName, orgID, userName string) ([]string, error) {
	if s.limiter == nil || userName == "" {
		return nil, nil
	}
	identifiers := []string{orgName}
	if orgID != "" && orgID != orgName {
		identifiers = append(identifiers, orgID)
	}
	var matched []string
	for _, identifier := range identifiers {
		keys, err := s.limiter.Keys(ctx, loginLockoutPrefix+identifier+":*:"+userName+":*")
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			parts := strings.SplitN(key, ":", 6)
			if len(parts) != 6 || parts[0] != "polaris" || parts[1] != "login_fail" {
				continue
			}
			if parts[2] != identifier || parts[4] != userName {
				continue
			}
			matched = append(matched, key)
		}
	}
	return matched, nil
}

// maxFailuresFor 解析指定应用的生效失败阈值（应用级配置优先；取不到时用平台默认）。
func (s *LoginLockoutService) maxFailuresFor(ctx context.Context, institutionID int64, application string) int64 {
	if application == "" {
		return s.loginMax
	}
	app, err := s.apps.GetByCode(ctx, institutionID, application)
	if err != nil {
		return s.loginMax
	}
	max, _ := signinFailurePolicy(app, s.loginMax, s.loginWindow)
	return max
}
