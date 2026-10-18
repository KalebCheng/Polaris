package service

import (
	"context"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// InstitutionService 组织服务。
// 除组织 CRUD 外，还承担"以组织为中心的聚合视图"（Stats/StatsAll）。
type InstitutionService struct {
	orgs        repository.InstitutionRepository
	users       repository.UserRepository
	jobTitles   repository.JobTitleRepository
	jobLevels   repository.JobLevelRepository
	depts       repository.DepartmentRepository
	roles       repository.RoleRepository
	permissions repository.PermissionRepository
	apps        repository.ApplicationRepository
	providers   repository.ProviderRepository
	syncers     repository.SyncerRepository
	tokens      repository.TokenRepository
	records     repository.RecordRepository
	audits      *AuditRecorder
}

// InstitutionDeps 组织服务依赖（构造参数较多，用结构体承载以免签名漂移）
type InstitutionDeps struct {
	Orgs        repository.InstitutionRepository
	Users       repository.UserRepository
	JobTitles   repository.JobTitleRepository
	Depts       repository.DepartmentRepository
	JobLevels   repository.JobLevelRepository
	Roles       repository.RoleRepository
	Permissions repository.PermissionRepository
	Apps        repository.ApplicationRepository
	Providers   repository.ProviderRepository
	Syncers     repository.SyncerRepository
	Tokens      repository.TokenRepository
	Records     repository.RecordRepository
	Audits      *AuditRecorder
}

// NewInstitutionService 创建组织服务
func NewInstitutionService(deps InstitutionDeps) *InstitutionService {
	return &InstitutionService{
		orgs: deps.Orgs, users: deps.Users, jobTitles: deps.JobTitles, depts: deps.Depts, jobLevels: deps.JobLevels, roles: deps.Roles,
		permissions: deps.Permissions, apps: deps.Apps, providers: deps.Providers,
		syncers: deps.Syncers, tokens: deps.Tokens, records: deps.Records, audits: deps.Audits,
	}
}

// Create 创建组织
func (s *InstitutionService) Create(ctx context.Context, actor string, org *repository.Institution) (*repository.Institution, error) {
	if org.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institutionID name is required")
	}
	if _, err := s.orgs.GetByName(ctx, org.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "institutionID already exists")
	}
	if org.PasswordType == "" {
		org.PasswordType = auth.SupportedPasswordTypes()[0]
	}
	if err := validatePasswordConfig(org); err != nil {
		return nil, err
	}
	id, err := s.orgs.Create(ctx, org)
	if err != nil {
		return nil, err
	}
	org.ID = id
	// 组织根部门（Spec Q4.5：组织恒定作为部门树的根；新建组织必须有且只有一行根节点）
	root := &repository.Department{
		InstitutionID: org.ID, Name: org.Name, DisplayName: firstNonEmptyStr(org.DisplayName, org.Name),
		CodePath: org.Name, NamePath: firstNonEmptyStr(org.DisplayName, org.Name), Level: 0, IsRoot: true,
	}
	if _, err := s.depts.Create(ctx, root); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, org.ID, "institutionID.create", org.Name, nil)
	return org, nil
}

// Get 查询组织
func (s *InstitutionService) Get(ctx context.Context, id int64) (*repository.Institution, error) {
	org, err := s.orgs.GetByID(ctx, id)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeInstitutionNotFound, "institution not found").WithDetail("institution_id", id)
	}
	return org, nil
}

// Update 更新组织
func (s *InstitutionService) Update(ctx context.Context, actor string, org *repository.Institution) error {
	existing, err := s.orgs.GetByName(ctx, org.Name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeInstitutionNotFound, "institution not found").WithDetail("institution_id", org.Name)
	}
	org.ID = existing.ID
	if org.PasswordType == "" {
		org.PasswordType = existing.PasswordType
	}
	if err := validatePasswordConfig(org); err != nil {
		return err
	}
	// 盐为 write-only：请求未携带（空）时保留既有值，避免"编辑其它字段即清空盐"。
	if org.PasswordSalt == "" {
		org.PasswordSalt = existing.PasswordSalt
	}
	if err := s.orgs.Update(ctx, org); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, org.ID, "institutionID.update", org.Name, nil)
	return nil
}

// Delete 删除组织（存在用户时禁止删除）
func (s *InstitutionService) Delete(ctx context.Context, actor string, id int64) error {
	org, err := s.orgs.GetByID(ctx, id)
	if err != nil {
		return bizerrors.New(bizerrors.CodeInstitutionNotFound, "institution not found").WithDetail("institution_id", id)
	}
	// 内置组织属于系统自身，任何情况下都不允许删除
	if repository.IsBuiltInInstitution(org.Name) {
		return bizerrors.ErrProtectedObject.WithDetail("institution_id", org.Name)
	}
	count, err := s.users.Count(ctx, repository.ListQuery{InstitutionID: id})
	if err != nil {
		return err
	}
	if count > 0 {
		return bizerrors.New(bizerrors.CodeConflict, "institutionID still has users, remove them first")
	}
	if err := s.orgs.Delete(ctx, org.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, org.ID, "institutionID.delete", org.Name, nil)
	return nil
}

// List 分页列出组织
func (s *InstitutionService) List(ctx context.Context, keyword string, offset, limit int) ([]*repository.Institution, int64, error) {
	q := institutionQuery(0, keyword, offset, limit)
	items, err := s.orgs.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.orgs.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// validatePasswordConfig 校验组织密码配置：密码类型与强度策略必须是受支持的取值。
func validatePasswordConfig(org *repository.Institution) error {
	if org.PasswordType != "" && !auth.IsSupportedPasswordType(org.PasswordType) {
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported password type").
			WithDetail("allowed", auth.SupportedPasswordTypes())
	}
	if org.MfaPolicy != "" && !repository.IsMfaPolicy(org.MfaPolicy) {
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported mfa policy").
			WithDetail("allowed", []string{repository.MfaPolicyOptional, repository.MfaPolicyRequired})
	}
	if org.MfaRememberHours < 0 {
		return bizerrors.New(bizerrors.CodeInvalidParams, "mfa remember hours must not be negative")
	}
	for _, option := range org.PasswordOptions {
		if option == "" {
			continue
		}
		if !auth.IsSupportedPasswordOption(option) {
			return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported password option").
				WithDetail("allowed", auth.SupportedPasswordOptions())
		}
	}
	return nil
}

// NameByID 按主键解析组织名（对外 HTTP 路径统一用 id）。
func (s *InstitutionService) NameByID(ctx context.Context, id int64) (string, error) {
	item, err := s.orgs.GetByID(ctx, id)
	if err != nil || item == nil {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
