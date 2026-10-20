package service

import (
	"context"
	"sort"
	"strings"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// UpdateUserProfileInput 用户资料**部分更新**入参。
//
// 指针字段表示"本次提交了该字段"：nil 一律不覆盖库中现值。
// 事故背景（第五阶段缺陷 D-1）：过去 handler 直接把请求体绑进实体后用反射整行写回，
// 前端只提交 8 个字段，结果头像 / 姓名 / 部门等未提交字段**每次保存都被清空**。
//
// 敏感与状态字段（password_*、mfa_*、is_forbidden、is_deleted、type）刻意不在此结构里：
// 它们各自有专用端点，不能借"改资料"顺带生效（§3.1 最小权限）。
type UpdateUserProfileInput struct {
	DisplayName   *string `json:"display_name"`
	FirstName     *string `json:"first_name"`
	LastName      *string `json:"last_name"`
	Avatar        *string `json:"avatar"`
	Email         *string `json:"email"`
	EmailVerified *bool   `json:"email_verified"`
	Phone         *string `json:"phone"`
	PhoneVerified *bool   `json:"phone_verified"`
	CountryCode   *string `json:"country_code"`
	Location      *string `json:"location"`
	Address       *string `json:"address"`
	Affiliation   *string `json:"affiliation"`
	Title         *string `json:"title"`
	Homepage      *string `json:"homepage"`
	Bio           *string `json:"bio"`
	Language      *string `json:"language"`
	Gender        *string `json:"gender"`
	Birthday      *string `json:"birthday"`
	Education     *string `json:"education"`
	// JobLevelID 职级（Spec Q4.1 = A：仅属性，不参与授权；nil = 本次不修改）
	JobLevelID *int64 `json:"job_level_id"`
}

// EffectivePermission 单条有效权限（含来源标注）。
type EffectivePermission struct {
	Name     string `json:"name,omitempty"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Effect   string `json:"effect"`
	// Sources 来源标识：user（直接绑定）/ role:<角色名> / job_title:<职务名>
	Sources []string `json:"sources"`
}

// UserAuthorizations 用户授权全景（详情页「授权」标签）。
type UserAuthorizations struct {
	DirectRoleIDs  []int64  `json:"direct_role_ids"`
	DirectRoles    []string `json:"direct_roles"`
	InheritedRoles []string `json:"inherited_roles"`
	JobTitleIDs    []int64  `json:"job_title_ids"`
	JobTitles      []string `json:"job_titles"`
	// DirectPermissions 直接绑定到该用户的权限名
	DirectPermissions []string `json:"direct_permissions"`
	// EffectivePermissions 判定后的"资源-动作-效果"明细（与登录会话中的有效权限同源）
	EffectivePermissions []EffectivePermission `json:"effective_permissions"`
}

// UserSecurityStatus 用户安全状态（详情页「安全与认证」标签中的非会话部分）。
type UserSecurityStatus struct {
	HasPassword       bool       `json:"has_password"`
	PasswordUpdatedAt *time.Time `json:"password_updated_at,omitempty"`
	// PasswordExpireAt 依据组织密码有效期策略推算（策略为 0 时不下发）
	PasswordExpireAt *time.Time `json:"password_expire_at,omitempty"`
	MfaEnabled       bool       `json:"mfa_enabled"`
	MfaType          string     `json:"mfa_type,omitempty"`
	// MfaRecoveryCodesLeft 恢复码剩余数量（只回数量，绝不回码本身）
	MfaRecoveryCodesLeft int `json:"mfa_recovery_codes_left"`
}

// trimUserFields 去掉资料字段的首尾空白（控制台表单常见误输入）。
func trimUserFields(u *repository.User) {
	for _, field := range []*string{
		&u.Name, &u.DisplayName, &u.FirstName, &u.LastName, &u.Email, &u.Phone, &u.CountryCode,
		&u.Location, &u.Address, &u.Affiliation, &u.Title, &u.Homepage, &u.Language, &u.Gender,
		&u.Birthday, &u.Education,
	} {
		*field = strings.TrimSpace(*field)
	}
}

// UpdateProfile 更新用户资料（部分更新语义）。
func (s *UserService) UpdateProfile(ctx context.Context, actor string, institutionID int64, name string, in UpdateUserProfileInput) (*repository.User, error) {
	org, u, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	apply := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	apply(&u.DisplayName, in.DisplayName)
	apply(&u.FirstName, in.FirstName)
	apply(&u.LastName, in.LastName)
	apply(&u.Avatar, in.Avatar)
	apply(&u.Email, in.Email)
	apply(&u.Phone, in.Phone)
	apply(&u.CountryCode, in.CountryCode)
	apply(&u.Location, in.Location)
	apply(&u.Address, in.Address)
	apply(&u.Affiliation, in.Affiliation)
	apply(&u.Title, in.Title)
	apply(&u.Homepage, in.Homepage)
	apply(&u.Bio, in.Bio)
	apply(&u.Language, in.Language)
	apply(&u.Gender, in.Gender)
	apply(&u.Birthday, in.Birthday)
	apply(&u.Education, in.Education)
	if in.EmailVerified != nil {
		u.EmailVerified = *in.EmailVerified
	}
	if in.PhoneVerified != nil {
		u.PhoneVerified = *in.PhoneVerified
	}
	if in.JobLevelID != nil {
		if *in.JobLevelID == 0 {
			u.JobLevelID = nil // 0 = 清空职级
		} else {
			level, err := s.levels.GetByID(ctx, *in.JobLevelID)
			if err != nil || level.InstitutionID != institutionID {
				return nil, bizerrors.ErrInvalidParams.WithDetail("job_level_id", *in.JobLevelID)
			}
			u.JobLevelID = in.JobLevelID
		}
	}
	normalizeUserType(u)
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, org.ID, "user.update", u.Name, nil)
	return u, nil
}

// SetDisabled 禁用 / 启用账号。
//
// 禁用立即失效该用户全部会话（§3.1-11），并按账号策略触发下游回收；
// 启用不触发供给（账号重新可用时由登录/授权链路按需建号）。
func (s *UserService) SetDisabled(ctx context.Context, actor string, institutionID int64, name string, disabled bool) error {
	org, u, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	if u.IsForbidden == disabled {
		return nil // 幂等：状态未变化不做写入与审计
	}
	after := *u
	after.IsForbidden = disabled
	if err := s.assertAdminSafety(ctx, org, u, &after); err != nil {
		return err
	}
	u.IsForbidden = disabled
	normalizeUserType(u)
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	action := "user.enable"
	if disabled {
		action = "user.disable"
	}
	if disabled {
		if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{name}); err != nil {
			return err
		}
		if s.provision != nil {
			s.provision.TriggerUserStatusChange(ctx, institutionID, name, false)
		}
	}
	s.audits.Record(ctx, actor, institutionID, action, name, nil)
	return nil
}

// SetAdmin 设置 / 取消管理员。
func (s *UserService) SetAdmin(ctx context.Context, actor string, institutionID int64, name string, isAdmin bool) error {
	org, u, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	if u.IsAdmin == isAdmin {
		return nil
	}
	after := *u
	after.IsAdmin = isAdmin
	if err := s.assertAdminSafety(ctx, org, u, &after); err != nil {
		return err
	}
	u.IsAdmin = isAdmin
	normalizeUserType(u)
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	// 权限变了 → 会话必须失效（§3.1-11）
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{name}); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "user.set_admin", name, map[string]interface{}{"is_admin": isAdmin})
	return nil
}

// SetDepartments 变更用户部门归属（多值；Spec 第六阶段 Q2 = A）。
//
// 多值关系与主部门都在 user_departments（is_primary）中维护；
// primaryDepartmentID 为 0 时取 departmentIDs 的第一个。
func (s *UserService) SetDepartments(
	ctx context.Context, actor string, institutionID int64, name string, departmentIDs []int64, primaryDepartmentID int64,
) error {
	_, user, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(departmentIDs))
	seen := map[int64]bool{}
	for _, id := range departmentIDs {
		if id == 0 || seen[id] {
			continue
		}
		dept, err := s.depts.GetByID(ctx, id)
		if err != nil || dept.InstitutionID != institutionID {
			return bizerrors.ErrInvalidParams.WithDetail("department_id", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if primaryDepartmentID == 0 && len(ids) > 0 {
		primaryDepartmentID = ids[0]
	}
	if primaryDepartmentID != 0 && !seen[primaryDepartmentID] {
		return bizerrors.ErrInvalidParams.WithDetail("primary_department_id", primaryDepartmentID)
	}
	if err := s.userDepts.ReplaceForUser(ctx, institutionID, user.ID, ids, primaryDepartmentID, nil); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "user.set_departments", name, map[string]interface{}{
		"department_ids": ids, "primary_department_id": primaryDepartmentID,
	})
	return nil
}

// DepartmentIDs 返回用户的全部部门归属（主部门优先）
// JobLevelLabels 返回组织内「职级主键 → 名称」（列表/详情展示用，一次查询）
func (s *UserService) JobLevelLabels(ctx context.Context, institutionID int64) (map[int64]string, error) {
	items, err := s.levels.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(items))
	for _, l := range items {
		out[l.ID] = l.Name
	}
	return out, nil
}

func (s *UserService) DepartmentIDs(ctx context.Context, institutionID int64, name string) ([]int64, error) {
	_, user, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	return s.userDepts.DepartmentIDsForUser(ctx, user.ID)
}

// DepartmentIDsForUsers 批量返回用户的部门归属（列表页展示多部门；一次查询）。
func (s *UserService) DepartmentIDsForUsers(ctx context.Context, institutionID int64, userIDs []int64) (map[int64][]int64, error) {
	return s.userDepts.DepartmentIDsForUsers(ctx, institutionID, userIDs)
}

// SetJobLevel 设置用户职级（nil = 清空；Spec Q4.1 = A：仅属性，不参与授权）
func (s *UserService) SetJobLevel(ctx context.Context, actor string, institutionID int64, name string, levelID *int64) error {
	_, user, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	if levelID != nil {
		level, err := s.levels.GetByID(ctx, *levelID)
		if err != nil || level.InstitutionID != institutionID {
			return bizerrors.ErrInvalidParams.WithDetail("job_level_id", *levelID)
		}
	}
	user.JobLevelID = levelID
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "user.set_job_level", name, map[string]interface{}{"job_level_id": levelID})
	return nil
}

// AdminResetMFA 管理员重置用户 MFA（清空绑定，用户下次登录重新绑定）。
//
// 这是唯一能"降级第二因子"的管理动作，因此单独挂 user:reset_mfa 权限：
// 仅能改资料的角色不应顺带获得"绕过 MFA"的能力。
func (s *UserService) AdminResetMFA(ctx context.Context, actor string, institutionID int64, name string) error {
	_, user, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	if s.mfa != nil {
		if err := s.mfa.ResetUser(ctx, institutionID, user.ID); err != nil {
			return err
		}
	}
	if err := s.clearMFA(ctx, institutionID, name, actor, "user.mfa_reset"); err != nil {
		return err
	}
	return nil
}

// SetDirectRoles replaces the user's direct roles using primary keys.
func (s *UserService) SetDirectRoles(ctx context.Context, actor string, institutionID, userID int64, roleIDs []int64) error {
	user, err := s.loadUserByID(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	before, err := s.roles.DirectRoleIDsForUser(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	roleIDs = dedupeInt64(roleIDs)
	if err := s.roles.SetDirectRoleIDs(ctx, institutionID, userID, roleIDs); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{user.Name}); err != nil {
		return err
	}
	// 新授予的角色可能命中"授权即建号"的账号策略
	if len(newlyAddedIDs(before, roleIDs)) > 0 && s.provision != nil {
		s.provision.TriggerRoleGranted(ctx, institutionID, user.Name)
	}
	s.audits.Record(ctx, actor, institutionID, "user.set_roles", user.Name, map[string]interface{}{"role_ids": roleIDs})
	return nil
}

// SetUserJobTitles replaces the user's job-title assignments using primary keys.
func (s *UserService) SetUserJobTitles(ctx context.Context, actor string, institutionID, userID int64, jobTitleIDs []int64) error {
	user, err := s.loadUserByID(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	before, err := s.jobTitles.JobTitleIDsForUser(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	jobTitleIDs = dedupeInt64(jobTitleIDs)
	if err := s.jobTitles.SetJobTitleIDsForUser(ctx, institutionID, userID, jobTitleIDs); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{user.Name}); err != nil {
		return err
	}
	if len(newlyAddedIDs(before, jobTitleIDs)) > 0 && s.provision != nil {
		s.provision.TriggerRoleGranted(ctx, institutionID, user.Name)
	}
	s.audits.Record(ctx, actor, institutionID, "user.set_job_titles", user.Name, map[string]interface{}{"job_title_ids": jobTitleIDs})
	return nil
}

// Authorizations 汇总用户授权全景（直接角色 / 继承角色 / 用户组 / 直接权限 / 有效权限含来源）。
func (s *UserService) Authorizations(ctx context.Context, institutionID int64, name string) (*UserAuthorizations, error) {
	_, user, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	directRoleIDs, err := s.roles.DirectRoleIDsForUser(ctx, institutionID, user.ID)
	if err != nil {
		return nil, err
	}
	direct, err := s.roles.DirectRolesForUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	all, err := s.roles.RolesForUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	jobTitleNames, err := s.jobTitles.JobTitlesForUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	jobTitleIDs, err := s.jobTitles.JobTitleIDsForUser(ctx, institutionID, user.ID)
	if err != nil {
		return nil, err
	}
	bindings, err := s.roles.PermissionBindingsForUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	if s.sessions == nil {
		return nil, bizerrors.New(bizerrors.CodeInternalError, "session manager unavailable")
	}
	rules, err := s.sessions.EffectivePermissions(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}

	directSet := stringSet(direct)
	inherited := []string{}
	for _, role := range all {
		if !directSet[role] {
			inherited = append(inherited, role)
		}
	}
	sort.Strings(inherited)

	directPermissions := []string{}
	sources := map[string][]string{}
	for _, b := range bindings {
		sources[b.Permission] = append(sources[b.Permission], b.Source)
		if b.Source == "user" {
			directPermissions = append(directPermissions, b.Permission)
		}
	}
	sort.Strings(directPermissions)

	// 与登录会话中的有效权限同源：同一份 EffectivePermissions（resource × action × effect）
	effective := make([]EffectivePermission, 0, len(rules))
	for _, rule := range rules {
		own := sources[rule.Name]
		if own == nil {
			own = []string{}
		}
		effective = append(effective, EffectivePermission{
			Name: rule.Name, Resource: rule.Resource, Action: rule.Action, Effect: rule.Effect, Sources: own,
		})
	}
	return &UserAuthorizations{
		DirectRoleIDs:        orEmptyInt64(directRoleIDs),
		DirectRoles:          orEmptyStrings(direct),
		InheritedRoles:       inherited,
		JobTitleIDs:          orEmptyInt64(jobTitleIDs),
		JobTitles:            orEmptyStrings(jobTitleNames),
		DirectPermissions:    directPermissions,
		EffectivePermissions: effective,
	}, nil
}

// DepartmentLabels 返回组织内「部门主键 → 展示路径」映射。
// 列表页与详情页据此批量补全归属文案，避免逐行查询（部门数量少，一次取全）。
func (s *UserService) DepartmentLabels(ctx context.Context, institutionID int64) (map[int64]string, error) {
	items, err := s.depts.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 10000})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(items))
	for _, dept := range items {
		label := dept.NamePath
		if label == "" {
			label = dept.DisplayName
		}
		if label == "" {
			label = dept.Name
		}
		out[dept.ID] = label
	}
	return out, nil
}

// SecurityStatus 返回密码与 MFA 状态（会话与锁定条目由各自的端点提供）。
func (s *UserService) SecurityStatus(ctx context.Context, institutionID int64, name string) (*UserSecurityStatus, error) {
	org, u, err := s.loadOrgAndUser(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	status := &UserSecurityStatus{
		HasPassword:       u.PasswordHash != "",
		PasswordUpdatedAt: u.PasswordUpdatedAt,
		MfaEnabled:        u.MfaEnabled,
		MfaType:           u.MfaType,
	}
	if s.mfa != nil {
		enabled, types, recoveryLeft, summaryErr := s.mfa.CredentialSummary(ctx, institutionID, u.ID)
		if summaryErr != nil {
			return nil, summaryErr
		}
		status.MfaEnabled = enabled
		status.MfaRecoveryCodesLeft = recoveryLeft
		if len(types) > 0 {
			status.MfaType = strings.Join(types, ",")
		}
	}
	if u.PasswordUpdatedAt != nil && org.PasswordExpireDays > 0 {
		expireAt := u.PasswordUpdatedAt.Add(time.Duration(org.PasswordExpireDays) * 24 * time.Hour)
		status.PasswordExpireAt = &expireAt
	}
	return status, nil
}

// clearMFA 清空 MFA 绑定（自助关闭与管理员重置共用）。
func (s *UserService) clearMFA(ctx context.Context, institutionID int64, name, actor, action string) error {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	u.MfaEnabled = false
	u.MfaType = ""
	u.MfaSecret = ""
	u.MfaRecoveryCodes = nil
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, action, name, nil)
	return nil
}

// dedupeStrings 去重并剔除空串（整体替换类入参的前置归一化）。
func dedupeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

// orEmptyStrings nil → 空切片（响应中集合一律归一化，避免前端判空分支）。
func orEmptyStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

func orEmptyInt64(items []int64) []int64 {
	if items == nil {
		return []int64{}
	}
	return items
}

// stringSet 转集合。
func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}
