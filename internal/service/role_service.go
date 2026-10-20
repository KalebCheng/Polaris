package service

import (
	"context"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// RoleService 角色服务
type RoleService struct {
	roles    repository.RoleRepository
	orgs     repository.InstitutionRepository
	sessions sessionController
	audits   *AuditRecorder
	// provision 供给事件挂钩（可选；角色授予用户时触发"授权即建号"）。
	provision *ProvisioningService
}

// NewRoleService 创建角色服务
func NewRoleService(
	roles repository.RoleRepository,
	orgs repository.InstitutionRepository,
	sessions sessionController,
	audits *AuditRecorder,
	provision *ProvisioningService,
) *RoleService {
	return &RoleService{roles: roles, orgs: orgs, sessions: sessions, audits: audits, provision: provision}
}

// Create 创建角色
func (s *RoleService) Create(ctx context.Context, actor string, r *repository.Role) (*repository.Role, error) {
	if r.InstitutionID == 0 || r.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, r.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", r.InstitutionID)
	}
	if _, err := s.roles.GetByName(ctx, r.InstitutionID, r.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "role already exists")
	}
	id, err := s.roles.Create(ctx, r)
	if err != nil {
		return nil, err
	}
	r.ID = id
	s.audits.Record(ctx, actor, r.InstitutionID, "role.create", r.Name, nil)
	return r, nil
}

// Get 查询角色
func (s *RoleService) Get(ctx context.Context, institutionID, id int64) (*repository.Role, error) {
	r, err := s.roles.GetByID(ctx, id)
	if err != nil || (institutionID != 0 && r.InstitutionID != institutionID) {
		return nil, bizerrors.ErrRoleNotFound.WithDetail("id", id)
	}
	return r, nil
}

// Update 更新角色
func (s *RoleService) Update(ctx context.Context, actor string, id int64, r *repository.Role) error {
	existing, err := s.roles.GetByID(ctx, id)
	if err != nil || existing.InstitutionID != r.InstitutionID {
		return bizerrors.ErrRoleNotFound.WithDetail("id", id)
	}
	r.ID = id
	r.Name = existing.Name
	r.IsSystem = existing.IsSystem
	if err := s.roles.Update(ctx, r); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, r.InstitutionID, "role.update", r.Name, nil)
	return nil
}

// SetRelations 设置角色的成员/子角色/权限，并使相关用户会话失效。
func (s *RoleService) SetRelations(ctx context.Context, actor string, institutionID, id int64, rel *repository.RoleRelations) error {
	r, err := s.Get(ctx, institutionID, id)
	if err != nil {
		return err
	}
	oldUserIDs := relationIDs(r.Users)
	affected, err := s.roles.AffectedUsersForRoles(ctx, institutionID, []string{r.Name})
	if err != nil {
		return err
	}
	if err := s.roles.SetMembers(ctx, r.ID, rel); err != nil {
		return err
	}
	nextAffected, err := s.roles.AffectedUsersForRoles(ctx, institutionID, []string{r.Name})
	if err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, append(affected, nextAffected...)); err != nil {
		return err
	}
	// 触发"授权即建号"的账号策略（仅对本次新授予的用户）
	if s.provision != nil {
		updated, loadErr := s.roles.GetByID(ctx, id)
		if loadErr == nil {
			names := make(map[int64]string, len(updated.Users))
			for _, user := range updated.Users {
				names[user.ID] = user.Name
			}
			for _, userID := range newlyAddedIDs(oldUserIDs, rel.UserIDs) {
				if name := names[userID]; name != "" {
					s.provision.TriggerRoleGranted(ctx, institutionID, name)
				}
			}
		}
	}
	s.audits.Record(ctx, actor, institutionID, "role.set_relations", r.Name, nil)
	return nil
}

// Delete 删除角色
func (s *RoleService) Delete(ctx context.Context, actor string, institutionID, id int64) error {
	r, err := s.Get(ctx, institutionID, id)
	if err != nil {
		return err
	}
	if r.IsSystem {
		return bizerrors.ErrProtectedObject.WithDetail("role", r.Name)
	}
	affected, err := s.roles.AffectedUsersForRoles(ctx, institutionID, []string{r.Name})
	if err != nil {
		return err
	}
	if err := s.roles.Delete(ctx, r.ID); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, affected); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "role.delete", r.Name, nil)
	return nil
}

// List 分页列出角色
func (s *RoleService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Role, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.roles.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.roles.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// RolesForUser 返回用户的有效角色名。
func (s *RoleService) RolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return s.roles.RolesForUser(ctx, institutionID, user)
}

// newlyAdded 返回 next 中相对 prev 新增的元素（去重、去空）。
func newlyAddedIDs(prev, next []int64) []int64 {
	existing := make(map[int64]bool, len(prev))
	for _, v := range prev {
		existing[v] = true
	}
	seen := map[int64]bool{}
	var out []int64
	for _, v := range next {
		if v == 0 || existing[v] || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func newlyAdded(prev, next []string) []string {
	existing := make(map[string]bool, len(prev))
	for _, value := range prev {
		existing[value] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(next))
	for _, value := range next {
		if value == "" || existing[value] || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func relationIDs(items []repository.RelationRef) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}
