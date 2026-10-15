package service

import (
	"context"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// GroupService 用户组服务
type GroupService struct {
	groups   repository.GroupRepository
	orgs     repository.InstitutionRepository
	users    repository.UserRepository
	roles    repository.RoleRepository
	sessions *auth.SessionManager
	audits   *AuditRecorder
	// provision 供给事件挂钩（可选；组成员授予时触发"授权即建号"）。
	provision *ProvisioningService
}

// NewGroupService 创建用户组服务。
// users / roles 用于组详情（成员明细 + 该组关联的角色与汇总权限）。
func NewGroupService(
	groups repository.GroupRepository,
	orgs repository.InstitutionRepository,
	users repository.UserRepository,
	roles repository.RoleRepository,
	sessions *auth.SessionManager,
	audits *AuditRecorder,
) *GroupService {
	return &GroupService{groups: groups, orgs: orgs, users: users, roles: roles, sessions: sessions, audits: audits}
}

// GroupDetail 用户组详情：成员明细 + 关联角色 + 这些角色带来的汇总权限。
//
// 动机（第三/四阶段遗留）：组是"批量授权的基本单元"，但界面上只能看到一个逗号分隔的成员串，
// 既看不到成员明细，也不知道这个组到底把哪些权限授予了组内的人。
type GroupDetail struct {
	Group       *repository.Group        `json:"group"`
	Members     []*repository.User       `json:"members"`
	Roles       []string                 `json:"roles"`
	Permissions []*repository.Permission `json:"permissions"`
}

// GetDetail 返回用户组详情。
func (s *GroupService) GetDetail(ctx context.Context, institutionID int64, name string) (*GroupDetail, error) {
	g, err := s.groups.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("group", name)
	}
	members, err := s.users.ListByNames(ctx, institutionID, g.Users)
	if err != nil {
		return nil, err
	}
	roles, err := s.roles.RolesForGroup(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	permissions, err := s.roles.PermissionsForGroup(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	return &GroupDetail{
		Group:       g,
		Members:     members,
		Roles:       roles,
		Permissions: permissions,
	}, nil
}

// MemberCounts 返回组织内各用户组的成员数（列表页展示）。
func (s *GroupService) MemberCounts(ctx context.Context, institutionID int64) (map[string]int, error) {
	return s.groups.MemberCounts(ctx, institutionID)
}

// SetProvisioning 注入供给服务（装配阶段调用）。
func (s *GroupService) SetProvisioning(p *ProvisioningService) { s.provision = p }

// Create 创建用户组
func (s *GroupService) Create(ctx context.Context, actor string, g *repository.Group) (*repository.Group, error) {
	if g.InstitutionID == 0 || g.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, g.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", g.InstitutionID)
	}
	if _, err := s.groups.GetByName(ctx, g.InstitutionID, g.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "group already exists")
	}
	id, err := s.groups.Create(ctx, g)
	if err != nil {
		return nil, err
	}
	g.ID = id
	s.audits.Record(ctx, actor, g.InstitutionID, "group.create", g.Name, nil)
	return g, nil
}

// Get 查询用户组
func (s *GroupService) Get(ctx context.Context, institutionID int64, name string) (*repository.Group, error) {
	g, err := s.groups.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("group", name)
	}
	return g, nil
}

// Update 更新用户组
func (s *GroupService) Update(ctx context.Context, actor string, g *repository.Group) error {
	existing, err := s.groups.GetByName(ctx, g.InstitutionID, g.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("group", g.Name)
	}
	g.ID = existing.ID
	if err := s.groups.Update(ctx, g); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, g.InstitutionID, "group.update", g.Name, nil)
	return nil
}

// SetMembers 设置用户组成员并使相关用户会话失效。
func (s *GroupService) SetMembers(ctx context.Context, actor string, institutionID int64, name string, users []string) error {
	g, err := s.groups.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("group", name)
	}
	affected := append([]string{}, g.Users...)
	affected = append(affected, users...)
	if err := s.groups.SetMembers(ctx, g.ID, users); err != nil {
		return err
	}
	if s.sessions != nil {
		seen := make(map[string]bool, len(affected))
		for _, u := range affected {
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			_, _ = s.sessions.InvalidateUser(ctx, institutionID, u)
		}
	}
	// 触发"授权即建号"的账号策略（仅对本次新增的组成员）
	if s.provision != nil {
		for _, u := range newlyAdded(g.Users, users) {
			s.provision.TriggerRoleGranted(ctx, institutionID, u)
		}
	}
	s.audits.Record(ctx, actor, institutionID, "group.set_members", name, nil)
	return nil
}

// Delete 删除用户组
func (s *GroupService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	g, err := s.groups.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("group", name)
	}
	if err := s.groups.Delete(ctx, g.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "group.delete", name, nil)
	return nil
}

// List 分页列出用户组
func (s *GroupService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Group, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.groups.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.groups.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GroupsForUser 返回用户所属用户组。
func (s *GroupService) GroupsForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return s.groups.GroupsForUser(ctx, institutionID, user)
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *GroupService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.groups.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
