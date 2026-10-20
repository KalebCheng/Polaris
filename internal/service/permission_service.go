package service

import (
	"context"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// PermissionService 权限服务
type PermissionService struct {
	permissions repository.PermissionRepository
	orgs        repository.InstitutionRepository
	roles       repository.RoleRepository
	sessions    sessionController
	audits      *AuditRecorder
}

// NewPermissionService 创建权限服务
func NewPermissionService(
	permissions repository.PermissionRepository,
	orgs repository.InstitutionRepository,
	roles repository.RoleRepository,
	sessions sessionController,
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
func (s *PermissionService) Get(ctx context.Context, institutionID, id int64) (*repository.Permission, error) {
	p, err := s.permissions.GetByID(ctx, id)
	if err != nil || (institutionID != 0 && p.InstitutionID != institutionID) {
		return nil, bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return p, nil
}

// Update 更新权限并使相关用户会话失效。
func (s *PermissionService) Update(ctx context.Context, actor string, id int64, p *repository.Permission) error {
	existing, err := s.permissions.GetByID(ctx, id)
	if err != nil || existing.InstitutionID != p.InstitutionID {
		return bizerrors.ErrNotFound.WithDetail("id", id)
	}
	p.ID = id
	affected, err := s.roles.AffectedUsersForPermission(ctx, p.InstitutionID, p.Name)
	if err != nil {
		return err
	}
	if err := s.permissions.Update(ctx, p); err != nil {
		return err
	}
	nextAffected, err := s.roles.AffectedUsersForPermission(ctx, p.InstitutionID, p.Name)
	if err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, p.InstitutionID, append(affected, nextAffected...)); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, p.InstitutionID, "permission.update", p.Name, nil)
	return nil
}

// SetSubjects 设置权限绑定的主体，并使相关用户会话失效。
func (s *PermissionService) SetSubjects(ctx context.Context, actor string, institutionID, id int64, userIDs, jobTitleIDs, roleIDs []int64) error {
	p, err := s.Get(ctx, institutionID, id)
	if err != nil {
		return err
	}
	affected, err := s.roles.AffectedUsersForPermission(ctx, institutionID, p.Name)
	if err != nil {
		return err
	}
	if err := s.permissions.SetSubjects(ctx, p.ID, userIDs, jobTitleIDs, roleIDs); err != nil {
		return err
	}
	nextAffected, err := s.roles.AffectedUsersForPermission(ctx, institutionID, p.Name)
	if err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, append(affected, nextAffected...)); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "permission.set_subjects", p.Name, nil)
	return nil
}

// Delete 删除权限并使相关用户会话失效。
func (s *PermissionService) Delete(ctx context.Context, actor string, institutionID, id int64) error {
	p, err := s.Get(ctx, institutionID, id)
	if err != nil {
		return err
	}
	affected, err := s.roles.AffectedUsersForPermission(ctx, institutionID, p.Name)
	if err != nil {
		return err
	}
	if err := s.permissions.Delete(ctx, p.ID); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, affected); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "permission.delete", p.Name, nil)
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
