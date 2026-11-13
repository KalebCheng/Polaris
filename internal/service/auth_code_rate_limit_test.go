package service

import (
	"context"
	"errors"
	"testing"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type clientIDAppRepo struct {
	repository.ApplicationRepository
	byClientID map[string]*repository.Application
}

func (r *clientIDAppRepo) GetByClientID(_ context.Context, clientID string) (*repository.Application, error) {
	if app := r.byClientID[clientID]; app != nil {
		return app, nil
	}
	return nil, errors.New("application not found")
}

type jitUserRepo struct {
	repository.UserRepository
	created     *repository.User
	createCalls int
	listErr     error
}

func (r *jitUserRepo) List(context.Context, repository.ListQuery) ([]*repository.User, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return nil, nil
}

func (r *jitUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return nil, errors.New("user not found")
}

func (r *jitUserRepo) Create(_ context.Context, user *repository.User) (int64, error) {
	r.createCalls++
	r.created = user
	user.ID = int64(r.createCalls)
	return user.ID, nil
}

func TestResolveCodeSendAppUsesClientID(t *testing.T) {
	apps := &clientIDAppRepo{byClientID: map[string]*repository.Application{
		"client-1": {ID: 2, InstitutionID: 1, Code: "target-app", Name: "Target App"},
	}}
	svc := &AuthService{apps: apps}

	app, err := svc.resolveCodeSendApp(context.Background(), CodeSendRequest{
		Institution: "acme",
		ClientID:    "client-1",
	})
	if err != nil {
		t.Fatalf("resolveCodeSendApp failed: %v", err)
	}
	if app == nil || app.Code != "target-app" {
		t.Fatalf("expected client_id application, got %+v", app)
	}
}

func TestEnforceCodeSendLimitsBlocksRotatingTargetsPerIP(t *testing.T) {
	limiter := newFakeLimiter()
	svc := &AuthService{
		rateLimiter: limiter,
		codeLimits: VerificationCodeLimitConfig{
			SendMinGap:       60 * time.Second,
			SendHourlyMax:    10,
			SendIPMinuteMax:  1,
			SendIPHourlyMax:  300,
			SendOrgHourlyMax: 5000,
		},
	}
	ctx := context.Background()

	if err := svc.enforceCodeSendLimits(ctx, CodeKindSMS, "acme", "+8613800000000", "10.0.0.1", 9); err != nil {
		t.Fatalf("first target should pass: %v", err)
	}
	err := svc.enforceCodeSendLimits(ctx, CodeKindSMS, "acme", "+8613800000001", "10.0.0.1", 9)
	if !bizerrors.Is(err, bizerrors.CodeVerifyCodeTooMany) {
		t.Fatalf("second target from same IP should be limited, got %v", err)
	}
	detail, ok := bizerrors.As(err)
	if !ok || detail.Details["scope"] != "ip" {
		t.Fatalf("expected ip scope detail, got %+v", detail)
	}
	if limiter.counts[codeSendLimitKey(CodeKindSMS, "acme", "target", "+8613800000001")] != 0 {
		t.Fatal("blocked IP must not consume the target counter")
	}
}

func TestFindUserByCodeTargetAllowsNilDefaultJobTitle(t *testing.T) {
	users := &jitUserRepo{}
	svc := &AuthService{
		users:       users,
		rateLimiter: newFakeLimiter(),
		codeLimits: VerificationCodeLimitConfig{
			JITSignupIPHourlyMax: 1,
		},
	}
	app := &repository.Application{
		ID:                1,
		InstitutionID:     1,
		Code:              "app",
		SelfSignupEnabled: true,
	}

	user, err := svc.findUserByCodeTarget(context.Background(), app, CodeKindSMS, "+8613800000000", "10.0.0.1")
	if err != nil {
		t.Fatalf("JIT signup failed: %v", err)
	}
	if users.created == nil || user == nil {
		t.Fatal("expected user to be created")
	}
	if user.Phone != "+8613800000000" {
		t.Fatalf("phone = %q, want canonical value", user.Phone)
	}
}

func TestFindUserByCodeTargetRateLimitsJITSignup(t *testing.T) {
	users := &jitUserRepo{}
	svc := &AuthService{
		users:       users,
		rateLimiter: newFakeLimiter(),
		codeLimits: VerificationCodeLimitConfig{
			JITSignupIPHourlyMax: 1,
		},
	}
	app := &repository.Application{
		ID:                1,
		InstitutionID:     1,
		Code:              "app",
		SelfSignupEnabled: true,
	}
	ctx := context.Background()

	if _, err := svc.findUserByCodeTarget(ctx, app, CodeKindSMS, "+8613800000000", "10.0.0.1"); err != nil {
		t.Fatalf("first JIT signup failed: %v", err)
	}
	_, err := svc.findUserByCodeTarget(ctx, app, CodeKindSMS, "+8613800000001", "10.0.0.1")
	if !bizerrors.Is(err, bizerrors.CodeVerifyCodeTooMany) {
		t.Fatalf("second JIT signup should be rate limited, got %v", err)
	}
	if users.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", users.createCalls)
	}
}

func TestFindUserByCodeTargetPropagatesLookupError(t *testing.T) {
	users := &jitUserRepo{listErr: bizerrors.New(bizerrors.CodeDBError, "database error")}
	svc := &AuthService{users: users, rateLimiter: newFakeLimiter()}
	app := &repository.Application{
		ID:                1,
		InstitutionID:     1,
		Code:              "app",
		SelfSignupEnabled: true,
	}

	_, err := svc.findUserByCodeTarget(context.Background(), app, CodeKindSMS, "+8613800000000", "10.0.0.1")
	if !bizerrors.Is(err, bizerrors.CodeDBError) {
		t.Fatalf("lookup error should propagate, got %v", err)
	}
	if users.createCalls != 0 {
		t.Fatal("database errors must not create users")
	}
}
