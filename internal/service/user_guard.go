package service

import (
	"context"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// normalizeUserType 收敛账号类型的唯一真相源：type 由 is_admin 派生。
//
// 动机（第五阶段缺陷 D-10）：type 与 is_admin 是两个字段表达同一件事，前端各自传值，
// 会出现 type=admin 而 is_admin=false 的漂移；而 type 会作为 claim 下发给下游应用，
// 漂移会直接传导到下游鉴权。因此写入点统一在这里，请求体里的 type 一律忽略。
func normalizeUserType(u *repository.User) {
	if u.IsAdmin {
		u.Type = "admin"
		return
	}
	u.Type = "normal"
}

// assertAdminSafety 管理员账号防呆（禁用 / 删除 / 取消管理员三条路径共用）。
//
// 规则：
//  1. 内置 admin 属于系统自身，不可被禁用、删除或取消管理员（否则无人可管理系统）；
//  2. 组织内最后一个"仍可登录"的管理员不可被取消管理员或禁用——只允许通过"先把别人设为管理员"过渡。
//
// before 为库中现状，after 为期望写入后的状态（删除场景传 after.IsDeleted = true）。
func (s *UserService) assertAdminSafety(ctx context.Context, org *repository.Institution, before, after *repository.User) error {
	if org != nil && repository.IsBuiltInAdmin(org.Name, before.Name) {
		return bizerrors.ErrProtectedObject.WithDetail("user", before.Name)
	}
	// 从「可用管理员」变成「不可用管理员」才需要校验数量
	wasUsable := before.IsAdmin && !before.IsForbidden && !before.IsDeleted
	stillUsable := after.IsAdmin && !after.IsForbidden && !after.IsDeleted
	if !wasUsable || stillUsable {
		return nil
	}
	count, err := s.users.CountAdmins(ctx, before.InstitutionID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return bizerrors.New(bizerrors.CodeConflict,
			"institution must keep at least one enabled administrator")
	}
	return nil
}

// loadOrgAndUser 读取组织与用户（两者都必须属于同一组织），并统一"未找到"语义。
func (s *UserService) loadOrgAndUser(ctx context.Context, institutionID int64, name string) (*repository.Institution, *repository.User, error) {
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return nil, nil, bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("user", name)
	}
	return org, u, nil
}

// loadUserByID scopes the lookup to the active institution.
func (s *UserService) loadUserByID(ctx context.Context, institutionID, userID int64) (*repository.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil || user == nil || user.InstitutionID != institutionID {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("id", userID)
	}
	return user, nil
}
