package service

import (
	"context"
	"errors"
	"testing"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

type failingSessionController struct {
	err     error
	invalid []string
}

func (f *failingSessionController) InvalidateUser(_ context.Context, _ int64, user string) ([]string, error) {
	f.invalid = append(f.invalid, user)
	return nil, f.err
}

func (f *failingSessionController) EffectivePermissions(context.Context, int64, string) ([]auth.PermissionRule, error) {
	return nil, nil
}

type relationRoleRepo struct {
	repository.RoleRepository
	role     *repository.Role
	affected []string
}

func (r *relationRoleRepo) GetByName(context.Context, int64, string) (*repository.Role, error) {
	return r.role, nil
}

func (r *relationRoleRepo) GetByID(context.Context, int64) (*repository.Role, error) {
	return r.role, nil
}

func (r *relationRoleRepo) SetMembers(context.Context, int64, *repository.RoleRelations) error {
	return nil
}

func (r *relationRoleRepo) AffectedUsersForRoles(context.Context, int64, []string) ([]string, error) {
	return append([]string(nil), r.affected...), nil
}

func TestRoleRelationChangeReturnsSessionInvalidationError(t *testing.T) {
	sessionErr := errors.New("redis unavailable")
	sessions := &failingSessionController{err: sessionErr}
	roles := &relationRoleRepo{
		role:     &repository.Role{ID: 7, InstitutionID: 1, Name: "staff"},
		affected: []string{"alice", "parent-role-user"},
	}
	svc := &RoleService{roles: roles, sessions: sessions, audits: &AuditRecorder{}}

	err := svc.SetRelations(context.Background(), "admin", 1, 7, &repository.RoleRelations{})
	if !errors.Is(err, sessionErr) {
		t.Fatalf("SetRelations error = %v, want session invalidation error", err)
	}
	if len(sessions.invalid) != 2 {
		t.Fatalf("invalidated users = %v, want both direct and inherited users", sessions.invalid)
	}
}
