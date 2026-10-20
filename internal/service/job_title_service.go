package service

import (
	"context"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// JobTitleService 职务服务
type JobTitleService struct {
	jobTitles repository.JobTitleRepository
	orgs      repository.InstitutionRepository
	users     repository.UserRepository
	roles     repository.RoleRepository
	sessions  sessionController
	audits    *AuditRecorder
	// provision 供给事件挂钩（可选；职务成员授予时触发"授权即建号"）。
	provision *ProvisioningService
}

// NewJobTitleService 创建职务服务。
// users / roles 用于职务详情（成员明细 + 该职务关联的角色与汇总权限）。
func NewJobTitleService(
	jobTitles repository.JobTitleRepository,
	orgs repository.InstitutionRepository,
	users repository.UserRepository,
	roles repository.RoleRepository,
	sessions sessionController,
	audits *AuditRecorder,
	provision *ProvisioningService,
) *JobTitleService {
	return &JobTitleService{
		jobTitles: jobTitles, orgs: orgs, users: users, roles: roles,
		sessions: sessions, audits: audits, provision: provision,
	}
}

// JobTitleDetail 职务详情：成员明细 + 关联角色 + 这些角色带来的汇总权限。
//
// 动机（第三/四阶段遗留）：职务是"批量授权的基本单元"，但界面上只能看到一个逗号分隔的成员串，
// 既看不到成员明细，也不知道这个职务到底把哪些权限授予了职务内的人。
type JobTitleDetail struct {
	JobTitle    *repository.JobTitle     `json:"jobTitle"`
	Members     []*repository.User       `json:"members"`
	Roles       []string                 `json:"roles"`
	Permissions []*repository.Permission `json:"permissions"`
}

// GetDetail 返回职务详情。
func (s *JobTitleService) GetDetail(ctx context.Context, institutionID int64, name string) (*JobTitleDetail, error) {
	g, err := s.jobTitles.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("job_title", name)
	}
	members, err := s.users.ListByIDs(ctx, institutionID, g.UserIDs)
	if err != nil {
		return nil, err
	}
	roles, err := s.roles.RolesForJobTitle(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	permissions, err := s.roles.PermissionsForJobTitle(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	return &JobTitleDetail{
		JobTitle:    g,
		Members:     members,
		Roles:       roles,
		Permissions: permissions,
	}, nil
}

// MemberCounts 返回组织内各职务的成员数（列表页展示）。
func (s *JobTitleService) MemberCounts(ctx context.Context, institutionID int64) (map[string]int, error) {
	return s.jobTitles.MemberCounts(ctx, institutionID)
}

// Create 创建职务
func (s *JobTitleService) Create(ctx context.Context, actor string, g *repository.JobTitle) (*repository.JobTitle, error) {
	if g.InstitutionID == 0 || g.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, g.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", g.InstitutionID)
	}
	if _, err := s.jobTitles.GetByName(ctx, g.InstitutionID, g.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "job title already exists")
	}
	id, err := s.jobTitles.Create(ctx, g)
	if err != nil {
		return nil, err
	}
	g.ID = id
	s.audits.Record(ctx, actor, g.InstitutionID, "job_title.create", g.Name, nil)
	return g, nil
}

// Get 查询职务
func (s *JobTitleService) Get(ctx context.Context, institutionID int64, name string) (*repository.JobTitle, error) {
	g, err := s.jobTitles.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("job_title", name)
	}
	return g, nil
}

// Update 更新职务
func (s *JobTitleService) Update(ctx context.Context, actor string, g *repository.JobTitle) error {
	existing, err := s.jobTitles.GetByName(ctx, g.InstitutionID, g.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("job_title", g.Name)
	}
	g.ID = existing.ID
	if err := s.jobTitles.Update(ctx, g); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, g.InstitutionID, "job_title.update", g.Name, nil)
	return nil
}

// SetMembers replaces job-title members by primary key and invalidates affected sessions.
func (s *JobTitleService) SetMembers(ctx context.Context, actor string, institutionID, jobTitleID int64, userIDs []int64) error {
	g, err := s.jobTitles.GetByID(ctx, jobTitleID)
	if err != nil || g.InstitutionID != institutionID {
		return bizerrors.ErrNotFound.WithDetail("job_title", jobTitleID)
	}
	userIDs = dedupeInt64(userIDs)
	addedIDs := newlyAddedIDs(g.UserIDs, userIDs)
	addedUsers, err := s.users.ListByIDs(ctx, institutionID, addedIDs)
	if err != nil {
		return err
	}
	affected := append([]string{}, g.Users...)
	for _, user := range addedUsers {
		affected = append(affected, user.Name)
	}
	if err := s.jobTitles.SetMemberIDs(ctx, g.ID, userIDs); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, affected); err != nil {
		return err
	}
	// 触发"授权即建号"的账号策略（仅对本次新增的职务成员）
	if s.provision != nil {
		for _, user := range addedUsers {
			s.provision.TriggerRoleGranted(ctx, institutionID, user.Name)
		}
	}
	s.audits.Record(ctx, actor, institutionID, "job_title.set_members", g.Name, nil)
	return nil
}

// Delete 删除职务
func (s *JobTitleService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	g, err := s.jobTitles.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("job_title", name)
	}
	if err := s.jobTitles.Delete(ctx, g.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "job_title.delete", name, nil)
	return nil
}

// List 分页列出职务
func (s *JobTitleService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.JobTitle, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.jobTitles.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.jobTitles.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// JobTitlesForUser 返回用户所属职务。
func (s *JobTitleService) JobTitlesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return s.jobTitles.JobTitlesForUser(ctx, institutionID, user)
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *JobTitleService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.jobTitles.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
