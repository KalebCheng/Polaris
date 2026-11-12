package service

import (
	"context"
	"testing"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

func TestEnforceRateLimitRulesUsesAtomicCounter(t *testing.T) {
	limiter := newFakeLimiter()
	rule := rateLimitRule{
		key:    "polaris:test:limit",
		window: time.Minute,
		max:    1,
		scope:  "target",
		details: map[string]interface{}{
			"retry_after_seconds": 60,
		},
	}
	if err := enforceRateLimitRules(context.Background(), limiter, rule); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	err := enforceRateLimitRules(context.Background(), limiter, rule)
	if !bizerrors.Is(err, bizerrors.CodeVerifyCodeTooMany) {
		t.Fatalf("second request should be rate limited, got %v", err)
	}
	if limiter.counts[rule.key] != 2 {
		t.Fatalf("counter = %d, want 2", limiter.counts[rule.key])
	}
	if limiter.ttls[rule.key] != time.Minute {
		t.Fatalf("ttl = %v, want %v", limiter.ttls[rule.key], time.Minute)
	}
}

func TestVerificationCodeLimitConfigDefaults(t *testing.T) {
	got := (VerificationCodeLimitConfig{}).withDefaults()
	if got.SendMinGap != 60*time.Second ||
		got.SendHourlyMax != 10 ||
		got.SendDailyMax != 20 ||
		got.SendIPMinuteMax != 20 ||
		got.SendIPHourlyMax != 300 ||
		got.SendOrgHourlyMax != 5000 ||
		got.SendProviderHourlyMax != 10000 ||
		got.VerifyIPHourlyMax != 300 ||
		got.JITSignupIPHourlyMax != 20 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestMFASMSCodeSendLimitAppliesToInitialEnrollment(t *testing.T) {
	svc := &MFAService{
		rateLimiter: newFakeLimiter(),
		codeLimits: VerificationCodeLimitConfig{
			SendMinGap:    60 * time.Second,
			SendHourlyMax: 10,
		},
	}
	session := &auth.Session{InstitutionID: 1, UserID: 2}
	user := &repository.User{ID: 2}
	provider := &repository.Provider{ID: 3}
	ctx := context.Background()

	if err := svc.enforceSMSCodeSendLimit(ctx, session, user, provider); err != nil {
		t.Fatalf("initial enrollment send should pass: %v", err)
	}
	err := svc.enforceSMSCodeSendLimit(ctx, session, user, provider)
	if !bizerrors.Is(err, bizerrors.CodeVerifyCodeTooMany) {
		t.Fatalf("second send should be rate limited, got %v", err)
	}
}
