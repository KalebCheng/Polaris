package service

import (
	"context"
	"testing"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type passwordUserRepo struct {
	repository.UserRepository
	user         *repository.User
	updated      *repository.User
	updateCalls  int
	initialCalls int
	initialErr   error
	history      []repository.PasswordHistoryEntry
}

func (r *passwordUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return r.user, nil
}

func (r *passwordUserRepo) UpdatePassword(_ context.Context, u *repository.User, _ int) error {
	r.updateCalls++
	copied := *u
	r.updated = &copied
	return nil
}

func (r *passwordUserRepo) UpdateInitialPassword(_ context.Context, u *repository.User, _ int) error {
	r.initialCalls++
	if r.initialErr != nil {
		return r.initialErr
	}
	copied := *u
	r.updated = &copied
	return nil
}

func (r *passwordUserRepo) PasswordHistory(context.Context, int64, string, int) ([]repository.PasswordHistoryEntry, error) {
	return r.history, nil
}

type passwordOrgRepo struct {
	repository.InstitutionRepository
	org *repository.Institution
}

func (r *passwordOrgRepo) GetByID(context.Context, int64) (*repository.Institution, error) {
	return r.org, nil
}

func newPasswordService(user *repository.User) (*UserService, *passwordUserRepo) {
	return newPasswordServiceWithOrg(user, &repository.Institution{ID: 7, Name: "acme"})
}

func newPasswordServiceWithOrg(user *repository.User, org *repository.Institution) (*UserService, *passwordUserRepo) {
	repo := &passwordUserRepo{user: user}
	svc := NewUserService(
		repo,
		&passwordOrgRepo{org: org},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return svc, repo
}

// 回归：第三方登录自动建号没有密码，门户首次设密不应再要求输入旧密码。
func TestChangePasswordInitializesMissingPasswordWithoutOldPassword(t *testing.T) {
	svc, repo := newPasswordService(&repository.User{
		ID: 11, InstitutionID: 7, Name: "sso-user",
	})

	if err := svc.ChangePassword(context.Background(), 7, "sso-user", "", "InitPass123"); err != nil {
		t.Fatalf("首次设密应成功: %v", err)
	}
	if repo.initialCalls != 1 || repo.updateCalls != 0 {
		t.Fatalf("应走初始设密写入，实际 initial=%d update=%d", repo.initialCalls, repo.updateCalls)
	}
	if repo.updated == nil || !auth.VerifyPasswordWith("InitPass123", repo.updated.PasswordHash, repo.updated.PasswordType, repo.updated.PasswordSalt) {
		t.Fatal("初始密码未按组织策略写入")
	}
}

// 已有密码的账号仍必须校验旧密码；空旧密码不能绕过。
func TestChangePasswordRequiresOldPasswordWhenPasswordExists(t *testing.T) {
	hash, err := auth.HashPassword("OldPass123")
	if err != nil {
		t.Fatalf("生成测试密码失败: %v", err)
	}
	svc, repo := newPasswordService(&repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user", PasswordHash: hash,
	})

	if err := svc.ChangePassword(context.Background(), 7, "local-user", "", "NewPass123"); !bizerrors.Is(err, bizerrors.CodeInvalidCredentials) {
		t.Fatalf("空旧密码应被拒绝，实际: %v", err)
	}
	if repo.updateCalls != 0 || repo.initialCalls != 0 {
		t.Fatal("校验失败时不得写入密码")
	}
}

func TestChangePasswordUpdatesExistingPasswordWithCorrectOldPassword(t *testing.T) {
	hash, err := auth.HashPassword("OldPass123")
	if err != nil {
		t.Fatalf("生成测试密码失败: %v", err)
	}
	svc, repo := newPasswordService(&repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user", PasswordHash: hash,
	})

	if err := svc.ChangePassword(context.Background(), 7, "local-user", "OldPass123", "NewPass123"); err != nil {
		t.Fatalf("旧密码正确时应允许改密: %v", err)
	}
	if repo.updateCalls != 1 || repo.initialCalls != 0 {
		t.Fatalf("应走常规改密写入，实际 initial=%d update=%d", repo.initialCalls, repo.updateCalls)
	}
	if repo.updated == nil || !auth.VerifyPasswordWith("NewPass123", repo.updated.PasswordHash, repo.updated.PasswordType, repo.updated.PasswordSalt) {
		t.Fatal("新密码未写入")
	}
}

// 条件更新未命中说明密码在读取后被并发设置，必须按凭证冲突拒绝，不能覆盖。
func TestChangePasswordRejectsConcurrentInitialization(t *testing.T) {
	svc, repo := newPasswordService(&repository.User{
		ID: 11, InstitutionID: 7, Name: "sso-user",
	})
	repo.initialErr = bizerrors.New(bizerrors.CodeConflict, "password already set")

	if err := svc.ChangePassword(context.Background(), 7, "sso-user", "", "InitPass123"); !bizerrors.Is(err, bizerrors.CodeInvalidCredentials) {
		t.Fatalf("并发设密应按凭证错误拒绝，实际: %v", err)
	}
	if repo.updated != nil {
		t.Fatal("条件更新失败时不得留下已写入的实体")
	}
}

// 回归：历史密码必须按写入时的算法/盐校验。组织后来切换算法或盐时，
// 不能拿当前配置去验旧 hash，否则复用校验会静默失效。
func TestValidatePasswordUsesHistoricalAlgorithmAndSalt(t *testing.T) {
	historyHash, err := auth.HashPasswordWith("ReusedPass123", "salt", "old-pepper")
	if err != nil {
		t.Fatalf("生成历史密码失败: %v", err)
	}
	currentHash, err := auth.HashPassword("CurrentPass123")
	if err != nil {
		t.Fatalf("生成当前密码失败: %v", err)
	}
	svc, repo := newPasswordServiceWithOrg(&repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user", PasswordHash: currentHash,
	}, &repository.Institution{
		ID: 7, Name: "acme", PasswordType: "bcrypt", PasswordSalt: "current-pepper",
		PasswordHistoryCount: 3,
	})
	repo.history = []repository.PasswordHistoryEntry{{
		PasswordHash: historyHash, PasswordType: "salt", PasswordSalt: "old-pepper",
	}}

	err = svc.ChangePassword(context.Background(), 7, "local-user", "CurrentPass123", "ReusedPass123")
	if !bizerrors.Is(err, bizerrors.CodeInvalidParams) {
		t.Fatalf("应拒绝复用历史密码，实际: %v", err)
	}
	if repo.updateCalls != 0 || repo.initialCalls != 0 {
		t.Fatal("复用校验失败时不得写入密码")
	}
}

func TestChangePasswordRejectsCurrentPasswordWithoutHistory(t *testing.T) {
	currentHash, err := auth.HashPassword("CurrentPass123")
	if err != nil {
		t.Fatalf("生成当前密码失败: %v", err)
	}
	svc, repo := newPasswordServiceWithOrg(&repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user", PasswordHash: currentHash,
	}, &repository.Institution{ID: 7, Name: "acme", PasswordHistoryCount: 0})

	err = svc.ChangePassword(context.Background(), 7, "local-user", "CurrentPass123", "CurrentPass123")
	if !bizerrors.Is(err, bizerrors.CodeInvalidParams) {
		t.Fatalf("新密码等于当前密码时应拒绝，实际: %v", err)
	}
	if repo.updateCalls != 0 || repo.initialCalls != 0 {
		t.Fatal("校验失败时不得写入密码")
	}
}

func TestAdminResetPasswordRequiresChangeOnNextLogin(t *testing.T) {
	currentHash, err := auth.HashPassword("CurrentPass123")
	if err != nil {
		t.Fatalf("生成当前密码失败: %v", err)
	}
	svc, repo := newPasswordServiceWithOrg(&repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user", PasswordHash: currentHash,
	}, &repository.Institution{ID: 7, Name: "acme"})

	if err := svc.SetPassword(context.Background(), "admin", 7, "local-user", "ResetPass123"); err != nil {
		t.Fatalf("管理员重置密码失败: %v", err)
	}
	if repo.updated == nil || !repo.updated.MustChangePassword {
		t.Fatal("管理员重置后必须要求下次登录改密")
	}
}
