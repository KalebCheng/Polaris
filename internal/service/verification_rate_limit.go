package service

import (
	"context"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

const (
	defaultCodeSendMinGap            = 60 * time.Second
	defaultCodeSendHourlyMax         = int64(10)
	defaultCodeSendDailyMax          = int64(20)
	defaultCodeSendIPMinuteMax       = int64(20)
	defaultCodeSendIPHourlyMax       = int64(300)
	defaultCodeSendOrgHourlyMax      = int64(5000)
	defaultCodeSendProviderHourlyMax = int64(10000)
	defaultCodeVerifyIPHourlyMax     = int64(300)
	defaultCodeJITSignupIPHourlyMax  = int64(20)
	codeVerifyRateScope              = "polaris:verify_check:"
	codeJITSignupRateScope           = "polaris:code_signup:"
)

// VerificationCodeLimitConfig is the runtime view of auth.rate_limit.verification_code.
// A zero value is normalized to safe defaults so manually constructed services never
// accidentally disable anti-abuse checks.
type VerificationCodeLimitConfig struct {
	SendMinGap            time.Duration
	SendHourlyMax         int64
	SendDailyMax          int64
	SendIPMinuteMax       int64
	SendIPHourlyMax       int64
	SendOrgHourlyMax      int64
	SendProviderHourlyMax int64
	VerifyIPHourlyMax     int64
	JITSignupIPHourlyMax  int64
}

func (c VerificationCodeLimitConfig) withDefaults() VerificationCodeLimitConfig {
	if c.SendMinGap <= 0 {
		c.SendMinGap = defaultCodeSendMinGap
	}
	if c.SendHourlyMax <= 0 {
		c.SendHourlyMax = defaultCodeSendHourlyMax
	}
	if c.SendDailyMax <= 0 {
		c.SendDailyMax = defaultCodeSendDailyMax
	}
	if c.SendIPMinuteMax <= 0 {
		c.SendIPMinuteMax = defaultCodeSendIPMinuteMax
	}
	if c.SendIPHourlyMax <= 0 {
		c.SendIPHourlyMax = defaultCodeSendIPHourlyMax
	}
	if c.SendOrgHourlyMax <= 0 {
		c.SendOrgHourlyMax = defaultCodeSendOrgHourlyMax
	}
	if c.SendProviderHourlyMax <= 0 {
		c.SendProviderHourlyMax = defaultCodeSendProviderHourlyMax
	}
	if c.VerifyIPHourlyMax <= 0 {
		c.VerifyIPHourlyMax = defaultCodeVerifyIPHourlyMax
	}
	if c.JITSignupIPHourlyMax <= 0 {
		c.JITSignupIPHourlyMax = defaultCodeJITSignupIPHourlyMax
	}
	return c
}

type rateLimitRule struct {
	key     string
	window  time.Duration
	max     int64
	scope   string
	details map[string]interface{}
}

// enforceRateLimitRules increments each counter and rejects the request when any
// window is exhausted. Counters are incremented even on rejection; this keeps the
// check atomic under concurrency and makes repeated abuse hit the same window.
func enforceRateLimitRules(ctx context.Context, limiter repository.RateLimitRepository, rules ...rateLimitRule) error {
	if limiter == nil {
		return nil
	}
	for _, rule := range rules {
		if rule.key == "" || rule.window <= 0 || rule.max <= 0 {
			continue
		}
		count, err := limiter.Incr(ctx, rule.key, rule.window)
		if err != nil {
			return err
		}
		if count <= rule.max {
			continue
		}
		exceeded := bizerrors.ErrVerifyCodeTooMany.WithDetail("scope", rule.scope)
		for key, value := range rule.details {
			exceeded = exceeded.WithDetail(key, value)
		}
		return exceeded
	}
	return nil
}
