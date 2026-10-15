package service

import (
	"context"
	"testing"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type protectedOrgRepo struct {
	repository.InstitutionRepository
	deleted bool
}

func (r *protectedOrgRepo) GetByName(_ context.Context, name string) (*repository.Institution, error) {
	return &repository.Institution{ID: 2, Name: name}, nil
}

func (r *protectedOrgRepo) GetByID(_ context.Context, id int64) (*repository.Institution, error) {
	if id == 1 {
		return &repository.Institution{ID: 1, Name: repository.BuiltInInstitution}, nil
	}
	return &repository.Institution{ID: id, Name: "acme"}, nil
}
func (r *protectedOrgRepo) Delete(context.Context, int64) error { r.deleted = true; return nil }

type protectedUserRepo struct {
	repository.UserRepository
	softDeleted bool
}

func (r *protectedUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return &repository.User{ID: 9, InstitutionID: 1, Name: "bob"}, nil
}
func (r *protectedUserRepo) Count(context.Context, repository.ListQuery) (int64, error) {
	return 0, nil
}
func (r *protectedUserRepo) SoftDelete(context.Context, int64) error {
	r.softDeleted = true
	return nil
}

// 回归：内置组织不可删除。
func TestBuiltInInstitutionCannotBeDeleted(t *testing.T) {
	orgs := &protectedOrgRepo{}
	svc := &InstitutionService{orgs: orgs, users: &protectedUserRepo{}}
	err := svc.Delete(context.Background(), "admin", 1)
	if err == nil {
		t.Fatal("内置组织应拒绝删除")
	}
	if !bizerrors.Is(err, bizerrors.CodeProtectedObject) {
		t.Fatalf("应返回 PROTECTED_OBJECT，实际 %v", err)
	}
	if orgs.deleted {
		t.Fatal("内置组织不应被真正删除")
	}
}

// 普通组织仍可删除（未过度限制）。
func TestNormalInstitutionCanBeDeleted(t *testing.T) {
	orgs := &protectedOrgRepo{}
	svc := &InstitutionService{orgs: orgs, users: &protectedUserRepo{}}
	if err := svc.Delete(context.Background(), "admin", 2); err != nil {
		t.Fatalf("普通组织应可删除，实际 %v", err)
	}
	if !orgs.deleted {
		t.Fatal("普通组织应已删除")
	}
}

// 回归：内置系统管理员账号不可删除。
func TestBuiltInAdminCannotBeDeleted(t *testing.T) {
	users := &protectedUserRepo{}
	svc := &UserService{users: users, orgs: &protectedOrgRepo{}}
	err := svc.Delete(context.Background(), "admin", 1, repository.BuiltInAdminUser)
	if err == nil {
		t.Fatal("内置管理员应拒绝删除")
	}
	if !bizerrors.Is(err, bizerrors.CodeProtectedObject) {
		t.Fatalf("应返回 PROTECTED_OBJECT，实际 %v", err)
	}
	if users.softDeleted {
		t.Fatal("内置管理员不应被真正删除")
	}
}

// 普通用户仍可删除。
func TestNormalUserCanBeDeleted(t *testing.T) {
	users := &protectedUserRepo{}
	svc := &UserService{users: users, orgs: &protectedOrgRepo{}}
	if err := svc.Delete(context.Background(), "admin", 2, "bob"); err != nil {
		t.Fatalf("普通用户应可删除，实际 %v", err)
	}
	if !users.softDeleted {
		t.Fatal("普通用户应已删除")
	}
}

// 同名的 admin 若不在内置组织，则不受保护（保护只针对内置组织下的 admin）。
func TestAdminOutsideBuiltInIsNotProtected(t *testing.T) {
	users := &protectedUserRepo{}
	svc := &UserService{users: users, orgs: &protectedOrgRepo{}}
	if err := svc.Delete(context.Background(), "ops", 2, repository.BuiltInAdminUser); err != nil {
		t.Fatalf("非内置组织的 admin 应可删除，实际 %v", err)
	}
}
