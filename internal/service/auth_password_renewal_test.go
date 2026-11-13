package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type renewalSessionRepo struct {
	repository.SessionRepository
	data  map[string][]byte
	index map[string]map[string]bool
}

func newRenewalSessionRepo() *renewalSessionRepo {
	return &renewalSessionRepo{data: map[string][]byte{}, index: map[string]map[string]bool{}}
}

func (r *renewalSessionRepo) Create(_ context.Context, id string, data []byte, _ time.Duration) error {
	r.data[id] = append([]byte(nil), data...)
	return nil
}

func (r *renewalSessionRepo) Get(_ context.Context, id string) ([]byte, error) {
	data, ok := r.data[id]
	if !ok {
		return nil, bizerrors.ErrSessionNotFound
	}
	return data, nil
}

func (r *renewalSessionRepo) Delete(_ context.Context, id string) error {
	delete(r.data, id)
	return nil
}

func (r *renewalSessionRepo) AddUserSession(_ context.Context, institutionID int64, user, id string) error {
	key := sessionIndexKey(institutionID, user)
	if r.index[key] == nil {
		r.index[key] = map[string]bool{}
	}
	r.index[key][id] = true
	return nil
}

func (r *renewalSessionRepo) RemoveUserSession(_ context.Context, institutionID int64, user, id string) error {
	delete(r.index[sessionIndexKey(institutionID, user)], id)
	return nil
}

func (r *renewalSessionRepo) UserSessions(_ context.Context, institutionID int64, user string) ([]string, error) {
	out := make([]string, 0)
	for id := range r.index[sessionIndexKey(institutionID, user)] {
		out = append(out, id)
	}
	return out, nil
}

func (r *renewalSessionRepo) DeleteUserSessions(ctx context.Context, institutionID int64, user string) ([]string, error) {
	ids, err := r.UserSessions(ctx, institutionID, user)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		delete(r.data, id)
	}
	delete(r.index, sessionIndexKey(institutionID, user))
	return ids, nil
}

func sessionIndexKey(institutionID int64, user string) string {
	return fmt.Sprintf("%d:%s", institutionID, user)
}

type renewalRoleRepo struct{ repository.RoleRepository }

func (renewalRoleRepo) RolesForUser(context.Context, int64, string) ([]string, error) {
	return nil, nil
}

func (renewalRoleRepo) PermissionsForUser(context.Context, int64, string) ([]*repository.Permission, error) {
	return nil, nil
}

type renewalUserRepo struct{ *passwordUserRepo }

func (r *renewalUserRepo) UpdateLastSignin(context.Context, int64, time.Time, string) error {
	return nil
}

func TestChangePasswordAndRenewSessionReplacesRestrictedSession(t *testing.T) {
	hash, err := auth.HashPassword("OldPass123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	passwordRepo := &passwordUserRepo{user: &repository.User{
		ID: 11, InstitutionID: 7, Name: "local-user",
		PasswordHash: hash, MustChangePassword: true,
	}}
	sessionRepo := newRenewalSessionRepo()
	sessions := auth.NewSessionManager(sessionRepo, &renewalUserRepo{passwordRepo}, renewalRoleRepo{}, nil, time.Hour)
	userSvc := NewUserService(
		passwordRepo,
		&passwordOrgRepo{org: &repository.Institution{ID: 7, Name: "acme"}},
		nil, nil, nil, nil, nil, nil, sessions, nil, nil, nil, nil,
	)
	svc := &AuthService{userService: userSvc, users: &renewalUserRepo{passwordRepo}, sessions: sessions}

	restricted, err := sessions.CreateWithPasswordProof(context.Background(), passwordRepo.user, "", "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create restricted session: %v", err)
	}
	if !restricted.PasswordChangeRequired {
		t.Fatal("restricted session should require password change")
	}

	renewedID, err := svc.ChangePasswordAndRenewSession(
		context.Background(), restricted.ID, "", "NewPass123")
	if err != nil {
		t.Fatalf("change password and renew: %v", err)
	}
	if renewedID == restricted.ID {
		t.Fatal("renewed session must be a new session")
	}
	if passwordRepo.user.MustChangePassword {
		t.Fatal("must_change_password should be cleared")
	}
	if passwordRepo.updated == nil || !auth.VerifyPasswordWith("NewPass123", passwordRepo.updated.PasswordHash, passwordRepo.updated.PasswordType, passwordRepo.updated.PasswordSalt) {
		t.Fatal("new password was not persisted")
	}
	data, err := sessionRepo.Get(context.Background(), renewedID)
	if err != nil {
		t.Fatalf("load renewed session: %v", err)
	}
	var renewed auth.Session
	if err := json.Unmarshal(data, &renewed); err != nil {
		t.Fatalf("decode renewed session: %v", err)
	}
	if renewed.PasswordChangeRequired {
		t.Fatal("renewed session must not require another password change")
	}
	if _, err := sessionRepo.Get(context.Background(), restricted.ID); err == nil {
		t.Fatal("restricted session should be invalidated")
	}
}

func TestForcedPasswordChangeFromNonPasswordLoginRequiresOldPassword(t *testing.T) {
	hash, err := auth.HashPassword("OldPass123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	passwordRepo := &passwordUserRepo{user: &repository.User{
		ID: 12, InstitutionID: 7, Name: "sso-user",
		PasswordHash: hash, MustChangePassword: true,
	}}
	sessionRepo := newRenewalSessionRepo()
	sessions := auth.NewSessionManager(sessionRepo, &renewalUserRepo{passwordRepo}, renewalRoleRepo{}, nil, time.Hour)
	userSvc := NewUserService(
		passwordRepo,
		&passwordOrgRepo{org: &repository.Institution{ID: 7, Name: "acme"}},
		nil, nil, nil, nil, nil, nil, sessions, nil, nil, nil, nil,
	)
	svc := &AuthService{userService: userSvc, users: &renewalUserRepo{passwordRepo}, sessions: sessions}

	restricted, err := sessions.Create(context.Background(), passwordRepo.user, "", "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create restricted session: %v", err)
	}
	if _, err := svc.ChangePasswordAndRenewSession(
		context.Background(), restricted.ID, "", "NewPass123"); !bizerrors.Is(err, bizerrors.CodeInvalidCredentials) {
		t.Fatalf("non-password session must require old password, got %v", err)
	}
	if _, err := svc.ChangePasswordAndRenewSession(
		context.Background(), restricted.ID, "OldPass123", "NewPass123"); err != nil {
		t.Fatalf("old password should allow change: %v", err)
	}
}
