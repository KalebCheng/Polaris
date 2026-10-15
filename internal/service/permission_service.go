package service

import (
	"context"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// PermissionService 权限服务
type PermissionService struct {
	permissions repository.PermissionRepository
	orgs        repository.InstitutionRepository
	roles       repository.RoleRepository
	sessions    *auth.SessionManager
	audits      *AuditRecorder
}

// NewPermissionService 创建权限服务
func NewPermissionService(
	permissions repository.PermissionRepository,
	orgs repository.InstitutionRepository,
	roles repository.RoleRepository,
	sessions *auth.SessionManager,
	audits *AuditRecorder,
) *PermissionService {
	return &PermissionService{permissions: permissions, orgs: orgs, roles: roles, sessions: sessions, audits: audits}
}

// Create 创建权限
func (s *PermissionService) Create(ctx context.Context, actor string, p *repository.Permission) (*repository.Permission, error) {
	if p.InstitutionID == 0 || p.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, p.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", p.InstitutionID)
	}
	if _, err := s.permissions.GetByName(ctx, p.InstitutionID, p.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "permission already exists")
	}
	if p.Effect == "" {
		p.Effect = "Allow"
	}
	if p.State == "" {
		p.State = "Approved"
	}
	id, err := s.permissions.Create(ctx, p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	s.audits.Record(ctx, actor, p.InstitutionID, "permission.create", p.Name, nil)
	return p, nil
}

// Get 查询权限
func (s *PermissionService) Get(ctx context.Context, institutionID int64, name string) (*repository.Permission, error) {
	p, err := s.permissions.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("permission", name)
	}
	return p, nil
}

// Update 更新权限并使相关用户会话失效。
func (s *PermissionService) Update(ctx context.Context, actor string, p *repository.Permission) error {
	existing, err := s.permissions.GetByName(ctx, p.InstitutionID, p.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("permission", p.Name)
	}
	p.ID = existing.ID
	affected := append([]string{}, existing.Users...)
	affected = append(affected, p.Users...)
	if err := s.permissions.Update(ctx, p); err != nil {
		return err
	}
	s.invalidateUsers(ctx, p.InstitutionID, affected)
	s.audits.Record(ctx, actor, p.InstitutionID, "permission.update", p.Name, nil)
	return nil
}

// SetSubjects 设置权限绑定的主体，并使相关用户会话失效。
func (s *PermissionService) SetSubjects(ctx context.Context, actor string, institutionID int64, name string, users, groups, roles []string) error {
	p, err := s.permissions.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("permission", name)
	}
	affected := append([]string{}, p.Users...)
	affected = append(affected, users...)
	if err := s.permissions.SetSubjects(ctx, p.ID, users, groups, roles); err != nil {
		return err
	}
	s.invalidateUsers(ctx, institutionID, affected)
	s.audits.Record(ctx, actor, institutionID, "permission.set_subjects", name, nil)
	return nil
}

// Delete 删除权限并使相关用户会话失效。
func (s *PermissionService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	p, err := s.permissions.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("permission", name)
	}
	if err := s.permissions.Delete(ctx, p.ID); err != nil {
		return err
	}
	s.invalidateUsers(ctx, institutionID, p.Users)
	s.audits.Record(ctx, actor, institutionID, "permission.delete", name, nil)
	return nil
}

// List 分页列出权限
func (s *PermissionService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Permission, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.permissions.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.permissions.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Enforce 判定某用户是否可对资源执行动作（用于 /enforce 调试端点）。
func (s *PermissionService) Enforce(ctx context.Context, institutionID int64, user, resource, action string) (bool, []string, error) {
	roles, err := s.roles.RolesForUser(ctx, institutionID, user)
	if err != nil {
		return false, nil, err
	}
	perms, err := s.roles.PermissionsForUser(ctx, institutionID, user)
	if err != nil {
		return false, nil, err
	}
	allowed := false
	for _, p := range perms {
		if !matchResource(p.Resources, resource) || !matchResource(p.Actions, action) {
			continue
		}
		if p.Effect == "Deny" {
			return false, roles, nil
		}
		allowed = true
	}
	return allowed, roles, nil
}

func matchResource(items []string, target string) bool {
	for _, it := range items {
		if it == "*" || it == target {
			return true
		}
	}
	return false
}

func (s *PermissionService) invalidateUsers(ctx context.Context, institutionID int64, users []string) {
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

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *PermissionService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.permissions.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
