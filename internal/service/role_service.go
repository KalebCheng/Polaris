package service

import (
	"context"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// RoleService 角色服务
type RoleService struct {
	roles    repository.RoleRepository
	groups   repository.GroupRepository
	orgs     repository.InstitutionRepository
	sessions *auth.SessionManager
	audits   *AuditRecorder
	// provision 供给事件挂钩（可选；角色授予用户时触发"授权即建号"）。
	provision *ProvisioningService
}

// NewRoleService 创建角色服务
func NewRoleService(
	roles repository.RoleRepository,
	groups repository.GroupRepository,
	orgs repository.InstitutionRepository,
	sessions *auth.SessionManager,
	audits *AuditRecorder,
) *RoleService {
	return &RoleService{roles: roles, groups: groups, orgs: orgs, sessions: sessions, audits: audits}
}

// SetProvisioning 注入供给服务（装配阶段调用）。
func (s *RoleService) SetProvisioning(p *ProvisioningService) { s.provision = p }

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
func (s *RoleService) Get(ctx context.Context, institutionID int64, name string) (*repository.Role, error) {
	r, err := s.roles.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrRoleNotFound.WithDetail("role", name)
	}
	return r, nil
}

// Update 更新角色
func (s *RoleService) Update(ctx context.Context, actor string, r *repository.Role) error {
	existing, err := s.roles.GetByName(ctx, r.InstitutionID, r.Name)
	if err != nil {
		return bizerrors.ErrRoleNotFound.WithDetail("role", r.Name)
	}
	r.ID = existing.ID
	r.IsSystem = existing.IsSystem
	if err := s.roles.Update(ctx, r); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, r.InstitutionID, "role.update", r.Name, nil)
	return nil
}

// SetRelations 设置角色的成员/子角色/权限，并使相关用户会话失效。
func (s *RoleService) SetRelations(ctx context.Context, actor string, institutionID int64, name string, rel *repository.RoleRelations) error {
	r, err := s.roles.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrRoleNotFound.WithDetail("role", name)
	}
	affected := append([]string{}, r.Users...)
	affected = append(affected, rel.Users...)
	for _, g := range append(r.Groups, rel.Groups...) {
		if grp, err := s.groups.GetByName(ctx, institutionID, g); err == nil {
			affected = append(affected, grp.Users...)
		}
	}
	if err := s.roles.SetMembers(ctx, r.ID, rel); err != nil {
		return err
	}
	s.invalidate(ctx, institutionID, affected)
	// 触发"授权即建号"的账号策略（仅对本次新授予的用户）
	if s.provision != nil {
		for _, u := range newlyAdded(r.Users, rel.Users) {
			s.provision.TriggerRoleGranted(ctx, institutionID, u)
		}
	}
	s.audits.Record(ctx, actor, institutionID, "role.set_relations", name, nil)
	return nil
}

// Delete 删除角色
func (s *RoleService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	r, err := s.roles.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrRoleNotFound.WithDetail("role", name)
	}
	if r.IsSystem {
		return bizerrors.ErrProtectedObject.WithDetail("role", name)
	}
	if err := s.roles.Delete(ctx, r.ID); err != nil {
		return err
	}
	s.invalidate(ctx, institutionID, r.Users)
	s.audits.Record(ctx, actor, institutionID, "role.delete", name, nil)
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

// invalidate 使指定用户的会话失效（去重）。
func (s *RoleService) invalidate(ctx context.Context, institutionID int64, users []string) {
	if s.sessions == nil {
		return
	}
	seen := make(map[string]bool, len(users))
	for _, u := range users {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		_, _ = s.sessions.InvalidateUser(ctx, institutionID, u)
	}
}

// newlyAdded 返回 next 中相对 prev 新增的元素（去重、去空）。
func newlyAdded(prev, next []string) []string {
	existing := make(map[string]bool, len(prev))
	for _, v := range prev {
		existing[v] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range next {
		if v == "" || existing[v] || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *RoleService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.roles.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
