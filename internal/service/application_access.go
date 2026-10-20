package service

import (
	"context"
	"sort"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"

	"polaris/internal/auth"
)

// 本文件是应用准入（"谁能使用该应用"）的**唯一判定入口**与名单读写实现。
//
// 判定作用域（Spec Q6）：只在请求携带应用上下文（client_id / application）时调用；
// "登录平台"（不带应用上下文）不判定——因此任何误配都不会把人挡在平台之外，
// 管理员随时能进控制台补授权，也就不需要（并且刻意不提供）管理员直通（Spec Q5）。
//
// 三种状态的语义（Spec §5.5-③）：
//
//	all                  → 全员可访问（默认；零额外查询）
//	restricted + 非空名单 → 仅名单内主体（含其子部门 / 子用户组）可访问
//	restricted + 空名单   → 无人可访问（合法状态，允许保存，事后补授权）
//
// 空名单**必须**判定为拒绝，不得回退为放行——否则就是"配了但不生效"的高危方向。

const (
	// subjectOptionDefaultLimit 主体候选的默认返回条数
	subjectOptionDefaultLimit = 50
	// subjectOptionMaxLimit 主体候选的条数上限（防止把整张目录表拉到前端）
	subjectOptionMaxLimit = 200
)

// SubjectView 授权主体（读取用，含展示信息）。
type SubjectView struct {
	// ID is the directory entity primary key.
	ID int64 `json:"id"`
	// Name is display-only; authorization and storage use ID.
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	// Path 层级路径：部门用 name_path（如「技术部 / 前端组」），用户组用父组链；用户为空
	Path string `json:"path,omitempty"`
	// Email 仅用户主体
	Email string `json:"email,omitempty"`
	// Missing 主体已被删除（判定不命中；界面提示"已失效"并支持清理）
	Missing bool `json:"missing"`
}

// ApplicationAccess 一个应用的访问授权全景（读接口的响应体）。
type ApplicationAccess struct {
	AccessMode  string        `json:"access_mode"`
	Users       []SubjectView `json:"users"`
	Groups      []SubjectView `json:"groups"`
	Departments []SubjectView `json:"departments"`
}

// ApplicationAccessInput 整体替换入参（PUT /applications/:id/access）。
type ApplicationAccessInput struct {
	AccessMode    string  `json:"access_mode"`
	UserIDs       []int64 `json:"user_ids"`
	JobTitleIDs   []int64 `json:"job_title_ids"`
	DepartmentIDs []int64 `json:"department_ids"`
}

// SubjectOption 候选主体（向导第 6 步的选择器数据源）。
//
// 单独提供该端点而不是复用 /users、/groups、/departments 的原因：
// 应用管理员通常没有 user:read / group:read / department:read，
// 直接复用会让选择器空转（"配了但选不出人"的隐性死配置）。
type SubjectOption struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Path        string `json:"path,omitempty"`
	ID          int64  `json:"id"`
	// HasChild is set for departments so the picker can render tree expansion.
	HasChild bool `json:"has_child,omitempty"`
}

// SubjectOptionPage 候选主体分页结果（total 供分页器/「加载更多」判断是否还有更多）
type SubjectOptionPage struct {
	Items []SubjectOption `json:"items"`
	Total int64           `json:"total"`
}

// ApplicationAccessService 应用访问授权服务
type ApplicationAccessService struct {
	subjects  repository.ApplicationSubjectRepository
	users     repository.UserRepository
	jobTitles repository.JobTitleRepository
	depts     repository.DepartmentRepository
	userDepts repository.UserDepartmentRepository
	sessions  *auth.SessionManager
	audits    *AuditRecorder
}

// NewApplicationAccessService 创建应用访问授权服务
func NewApplicationAccessService(
	subjects repository.ApplicationSubjectRepository,
	users repository.UserRepository,
	jobTitles repository.JobTitleRepository,
	depts repository.DepartmentRepository,
	userDepts repository.UserDepartmentRepository,
	sessions *auth.SessionManager,
	audits *AuditRecorder,
) *ApplicationAccessService {
	return &ApplicationAccessService{
		subjects: subjects, users: users, jobTitles: jobTitles, depts: depts, userDepts: userDepts,
		sessions: sessions, audits: audits,
	}
}

// ── 判定 ──────────────────────────────────────────────────────

// Allowed 判定用户当前是否被允许**以该应用为上下文**完成认证。
//
// nil 应用（组织未配置应用）与非受限应用一律放行；受限应用才做名单判定。
func (s *ApplicationAccessService) Allowed(ctx context.Context, app *repository.Application, user *repository.User) (bool, error) {
	if app == nil || user == nil {
		return true, nil
	}
	if app.AccessMode != repository.AccessModeRestricted {
		return true, nil
	}
	subjects, err := s.subjects.List(ctx, app.ID)
	if err != nil {
		return false, err
	}
	if len(subjects) == 0 {
		// 受限 + 空名单 = 无人可访问（禁止回退为放行）
		return false, nil
	}

	userIDs := map[int64]bool{}
	jobTitleIDs := map[int64]bool{}
	departmentIDs := map[int64]bool{}
	for _, sub := range subjects {
		switch sub.SubjectType {
		case repository.SubjectTypeUser:
			if sub.UserID != nil {
				userIDs[*sub.UserID] = true
			}
		case repository.SubjectTypeJobTitle:
			if sub.JobTitleID != nil {
				jobTitleIDs[*sub.JobTitleID] = true
			}
		case repository.SubjectTypeDepartment:
			if sub.DepartmentID != nil {
				departmentIDs[*sub.DepartmentID] = true
			}
		}
	}
	if userIDs[user.ID] {
		return true, nil
	}
	if len(jobTitleIDs) > 0 {
		ids, err := s.jobTitles.JobTitleIDsForUser(ctx, app.InstitutionID, user.ID)
		if err != nil {
			return false, err
		}
		for _, id := range ids {
			if jobTitleIDs[id] {
				return true, nil
			}
		}
	}
	if len(departmentIDs) > 0 {
		// 多部门归属的唯一真相是 user_departments：用户可能在多个部门挂职，
		// 只看主部门会把"通过第二部门授权"的人误判为无权访问。
		userDeptIDs := []int64{}
		if s.userDepts != nil {
			ids, err := s.userDepts.DepartmentIDsForUser(ctx, user.ID)
			if err != nil {
				return false, err
			}
			userDeptIDs = ids
		}
		if len(userDeptIDs) == 0 {
			return false, nil
		}
		// 授权父部门覆盖整棵子树：用户所在部门的 code_path 以被授权部门的 code_path 为前缀（含自身）
		all, err := s.depts.List(ctx, repository.ListQuery{InstitutionID: app.InstitutionID})
		if err != nil {
			return false, err
		}
		byID := make(map[int64]*repository.Department, len(all))
		for _, d := range all {
			byID[d.ID] = d
		}
		for _, id := range userDeptIDs {
			dept := byID[id]
			if dept == nil {
				continue
			}
			for _, d := range all {
				if !departmentIDs[d.ID] {
					continue
				}
				if dept.CodePath == d.CodePath || strings.HasPrefix(dept.CodePath, d.CodePath+"/") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// AllowedUserName 便捷入口：只拿得到用户名时（门户列表 / 令牌链路）自行加载用户。
// 用户不存在时返回 (false, nil) —— 不存在的人自然不在任何名单内。
func (s *ApplicationAccessService) AllowedUserName(ctx context.Context, app *repository.Application, institutionID int64, userName string) (bool, error) {
	if app == nil {
		return true, nil
	}
	if app.AccessMode != repository.AccessModeRestricted {
		return true, nil
	}
	user, err := s.users.GetByName(ctx, institutionID, userName)
	if err != nil {
		return false, nil
	}
	return s.Allowed(ctx, app, user)
}

// Enforce 判定并在未授权时返回 APP_ACCESS_DENIED（认证链路的统一拒绝入口）。
func (s *ApplicationAccessService) Enforce(ctx context.Context, app *repository.Application, user *repository.User) error {
	ok, err := s.Allowed(ctx, app, user)
	if err != nil {
		return err
	}
	if !ok {
		return bizerrors.NewWithCode(bizerrors.CodeAppAccessDenied).
			WithDetail("application", app.Code).
			WithDetail("user", user.Name)
	}
	return nil
}

// ── 名单读写 ──────────────────────────────────────────────────

// Access 读取应用的访问授权全景（含展示信息与"已失效"标记）。
func (s *ApplicationAccessService) Access(ctx context.Context, app *repository.Application) (*ApplicationAccess, error) {
	subjects, err := s.subjects.List(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	out := &ApplicationAccess{
		AccessMode:  normalizeAccessMode(app.AccessMode),
		Users:       []SubjectView{},
		Groups:      []SubjectView{},
		Departments: []SubjectView{},
	}
	for _, subject := range subjects {
		view := SubjectView{
			ID:          applicationSubjectID(subject),
			Name:        subject.SubjectName,
			DisplayName: firstNonEmptyStr(subject.SubjectDisplayName, subject.SubjectName),
			Path:        subject.SubjectPath,
			Email:       subject.SubjectEmail,
			Missing:     subject.SubjectName == "",
		}
		switch subject.SubjectType {
		case repository.SubjectTypeUser:
			out.Users = append(out.Users, view)
		case repository.SubjectTypeJobTitle:
			out.Groups = append(out.Groups, view)
		case repository.SubjectTypeDepartment:
			out.Departments = append(out.Departments, view)
		}
	}
	return out, nil
}

// SetAccess 整体替换应用的访问授权（访问模式 + 三类名单，单事务）。
//
// 校验：模式取值合法；三类主体必须**存在于本组织**（不存在即拒绝，不做静默忽略）。
// 提交后：写审计；把"被移除主体"展开出的用户在该应用下的会话失效（FR-7）。
func (s *ApplicationAccessService) SetAccess(ctx context.Context, actor string, app *repository.Application, in ApplicationAccessInput) (*ApplicationAccess, error) {
	mode := strings.TrimSpace(in.AccessMode)
	if !repository.IsAccessMode(mode) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("access_mode", in.AccessMode)
	}
	userIDs := dedupeInt64(in.UserIDs)
	jobTitleIDs := dedupeInt64(in.JobTitleIDs)
	departmentIDs := dedupeInt64(in.DepartmentIDs)
	if err := s.validateSubjects(ctx, app.InstitutionID, userIDs, jobTitleIDs, departmentIDs); err != nil {
		return nil, err
	}
	previous, err := s.subjects.List(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	next := make([]repository.ApplicationSubject, 0, len(userIDs)+len(jobTitleIDs)+len(departmentIDs))
	for _, id := range userIDs {
		next = append(next, repository.ApplicationSubject{SubjectType: repository.SubjectTypeUser, UserID: &id})
	}
	for _, id := range jobTitleIDs {
		next = append(next, repository.ApplicationSubject{SubjectType: repository.SubjectTypeJobTitle, JobTitleID: &id})
	}
	for _, id := range departmentIDs {
		next = append(next, repository.ApplicationSubject{SubjectType: repository.SubjectTypeDepartment, DepartmentID: &id})
	}
	if err := s.subjects.Replace(ctx, app, mode, next); err != nil {
		return nil, err
	}
	app.AccessMode = mode
	s.audits.Record(ctx, actor, app.InstitutionID, "application.access_update", app.Code, map[string]interface{}{
		"access_mode":    mode,
		"user_ids":       len(userIDs),
		"job_title_ids":  len(jobTitleIDs),
		"department_ids": len(departmentIDs),
	})
	if mode == repository.AccessModeRestricted {
		s.revokeRemovedSessions(ctx, app, previous, next)
	}
	return s.Access(ctx, app)
}

// SubjectCounts 批量统计各应用的授权主体数（应用列表「访问范围」列；key 为应用主键）。
func (s *ApplicationAccessService) SubjectCounts(ctx context.Context, apps []*repository.Application) (map[int64]int, error) {
	ids := make([]int64, 0, len(apps))
	for _, app := range apps {
		if app != nil && app.AccessMode == repository.AccessModeRestricted {
			ids = append(ids, app.ID)
		}
	}
	if len(ids) == 0 {
		return map[int64]int{}, nil
	}
	return s.subjects.CountByApplications(ctx, ids)
}

// SubjectOptions 返回候选主体（分页；部门的树形按层取）。
//
// 三种形态（对齐 IDaaS「添加授权」弹窗）：
//   - user / group：扁平分页列表，keyword 同时匹配标识与展示名；
//   - department + keyword：**扁平搜索结果**（跨层级，带层级路径，便于按名字直达）；
//   - department + 无 keyword：**按层取直接子节点**（parentID 为 nil 即顶层），
//     弹窗据此逐层懒加载，避免一次拉整棵树。
//
// parent_id 只对部门有意义：传给其它类型一律 INVALID_PARAMS（不静默忽略）。
func (s *ApplicationAccessService) SubjectOptions(
	ctx context.Context,
	institutionID int64,
	subjectType, keyword string,
	parentID *int64,
	offset, limit int,
) (*SubjectOptionPage, error) {
	if !repository.IsSubjectType(subjectType) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("type", subjectType)
	}
	keyword = strings.TrimSpace(keyword)
	if parentID != nil && subjectType != repository.SubjectTypeDepartment {
		return nil, bizerrors.ErrInvalidParams.WithDetail("parent_id", "only supported for department")
	}
	if parentID != nil && keyword != "" {
		return nil, bizerrors.ErrInvalidParams.WithDetail("parent_id", "mutually exclusive with keyword")
	}
	if limit <= 0 {
		limit = subjectOptionDefaultLimit
	}
	if limit > subjectOptionMaxLimit {
		limit = subjectOptionMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	page := &SubjectOptionPage{Items: []SubjectOption{}, Total: 0}
	q := repository.ListQuery{InstitutionID: institutionID, Keyword: keyword, Offset: offset, Limit: limit}

	switch subjectType {
	case repository.SubjectTypeUser:
		// 软删除账号不再作为候选（选中后再授权只会得到"看着在、其实进不去"）
		q.Filters = []repository.ColumnFilter{{Field: "is_deleted", Value: false}}
		items, err := s.users.List(ctx, q)
		if err != nil {
			return nil, err
		}
		total, err := s.users.Count(ctx, q)
		if err != nil {
			return nil, err
		}
		page.Total = total
		for _, u := range items {
			page.Items = append(page.Items, SubjectOption{
				ID: u.ID, Name: u.Name, DisplayName: firstNonEmptyStr(u.DisplayName, u.Name),
			})
		}
	case repository.SubjectTypeJobTitle:
		items, err := s.jobTitles.List(ctx, q)
		if err != nil {
			return nil, err
		}
		total, err := s.jobTitles.Count(ctx, q)
		if err != nil {
			return nil, err
		}
		page.Total = total
		for _, g := range items {
			page.Items = append(page.Items, SubjectOption{
				ID:          g.ID,
				Name:        g.Name,
				DisplayName: firstNonEmptyStr(g.DisplayName, g.Name),
				Path:        g.Name,
			})
		}
	case repository.SubjectTypeDepartment:
		if keyword != "" {
			items, err := s.depts.List(ctx, q)
			if err != nil {
				return nil, err
			}
			total, err := s.depts.Count(ctx, q)
			if err != nil {
				return nil, err
			}
			page.Total = total
			for _, d := range items {
				page.Items = append(page.Items, departmentOption(d))
			}
			return page, nil
		}
		items, total, err := s.depts.Children(ctx, institutionID, parentID, offset, limit)
		if err != nil {
			return nil, err
		}
		page.Total = total
		for _, d := range items {
			page.Items = append(page.Items, departmentOption(d))
		}
	}
	return page, nil
}

// departmentOption 部门候选（带 id / has_child，供前端按层组装树）
func departmentOption(d *repository.Department) SubjectOption {
	return SubjectOption{
		Name:        d.Name,
		DisplayName: firstNonEmptyStr(d.DisplayName, d.Name),
		Path:        firstNonEmptyStr(d.NamePath, d.Name),
		ID:          d.ID,
		HasChild:    d.HasChild,
	}
}

// ── 内部 ──────────────────────────────────────────────────────

// validateSubjects rejects IDs outside the institution instead of silently dropping them.
func (s *ApplicationAccessService) validateSubjects(
	ctx context.Context,
	institutionID int64,
	userIDs, jobTitleIDs, departmentIDs []int64,
) error {
	if len(userIDs) > 0 {
		have, err := s.users.ExistingIDs(ctx, institutionID, userIDs)
		if err != nil {
			return err
		}
		for _, id := range userIDs {
			if !have[id] {
				return bizerrors.ErrInvalidParams.WithDetail("unknown_user_id", id)
			}
		}
	}
	if len(jobTitleIDs) > 0 {
		have, err := s.jobTitles.ExistingIDs(ctx, institutionID, jobTitleIDs)
		if err != nil {
			return err
		}
		for _, id := range jobTitleIDs {
			if !have[id] {
				return bizerrors.ErrInvalidParams.WithDetail("unknown_job_title_id", id)
			}
		}
	}
	if len(departmentIDs) > 0 {
		have, err := s.depts.ExistingIDs(ctx, institutionID, departmentIDs)
		if err != nil {
			return err
		}
		for _, id := range departmentIDs {
			if !have[id] {
				return bizerrors.ErrInvalidParams.WithDetail("unknown_department_id", id)
			}
		}
	}
	return nil
}

// revokeRemovedSessions 让"被移除主体"的成员在**该应用**下的会话立即失效（FR-7）。
//
// 已知边界：从 all 切到 restricted 时，"因不在名单里而失去访问"的用户无法被枚举
// （那是全组织用户集），因此不做批量失效——他们仍会被"授权码 / 刷新令牌"的实时判定拦下。
func (s *ApplicationAccessService) revokeRemovedSessions(ctx context.Context, app *repository.Application, previous, next []repository.ApplicationSubject) {
	kept := map[applicationSubjectRef]bool{}
	for _, sub := range next {
		kept[applicationSubjectRefOf(sub)] = true
	}
	removed := make([]repository.ApplicationSubject, 0, len(previous))
	for _, sub := range previous {
		if !kept[applicationSubjectRefOf(sub)] {
			removed = append(removed, sub)
		}
	}
	if len(removed) == 0 {
		return
	}
	if s.sessions == nil {
		return // 会话管理器是可选基础设施（仅聚焦单测会缺省）
	}
	affected, err := s.ExpandUsers(ctx, app.InstitutionID, removed)
	if err != nil {
		logger.Warn("expand removed application subjects failed", "application", app.Code, "error", err)
		return
	}
	for _, name := range affected {
		if _, err := s.sessions.InvalidateUserApplication(ctx, app.InstitutionID, name, app.Code); err != nil {
			logger.Warn("invalidate application session failed", "application", app.Code, "user", name, "error", err)
		}
	}
}

// ExpandUsers expands subject IDs into usernames, including descendant departments.
func (s *ApplicationAccessService) ExpandUsers(ctx context.Context, institutionID int64, subjects []repository.ApplicationSubject) ([]string, error) {
	userIDs := map[int64]bool{}
	var jobTitleIDs, departmentIDs []int64
	for _, sub := range subjects {
		switch sub.SubjectType {
		case repository.SubjectTypeUser:
			if sub.UserID != nil {
				userIDs[*sub.UserID] = true
			}
		case repository.SubjectTypeJobTitle:
			if sub.JobTitleID != nil {
				jobTitleIDs = append(jobTitleIDs, *sub.JobTitleID)
			}
		case repository.SubjectTypeDepartment:
			if sub.DepartmentID != nil {
				departmentIDs = append(departmentIDs, *sub.DepartmentID)
			}
		}
	}
	if len(jobTitleIDs) > 0 {
		ids, err := s.jobTitles.MemberIDsByJobTitleIDs(ctx, institutionID, jobTitleIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			userIDs[id] = true
		}
	}
	if len(departmentIDs) > 0 {
		ids, err := s.userDepts.UserIDsOfDepartments(ctx, institutionID, departmentIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			userIDs[id] = true
		}
	}
	ids := make([]int64, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	users, err := s.users.ListByIDs(ctx, institutionID, ids)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(users))
	for _, user := range users {
		result = append(result, user.Name)
	}
	sort.Strings(result)
	return result, nil
}

type applicationSubjectRef struct {
	SubjectType string
	ID          int64
}

func applicationSubjectRefOf(subject repository.ApplicationSubject) applicationSubjectRef {
	return applicationSubjectRef{
		SubjectType: subject.SubjectType,
		ID:          applicationSubjectID(subject),
	}
}

func applicationSubjectID(subject repository.ApplicationSubject) int64 {
	switch subject.SubjectType {
	case repository.SubjectTypeUser:
		if subject.UserID != nil {
			return *subject.UserID
		}
	case repository.SubjectTypeJobTitle:
		if subject.JobTitleID != nil {
			return *subject.JobTitleID
		}
	case repository.SubjectTypeDepartment:
		if subject.DepartmentID != nil {
			return *subject.DepartmentID
		}
	}
	return 0
}

// normalizeAccessMode 空串按默认（全员可访问）解读——存量数据与未提交该字段的旧客户端。
func normalizeAccessMode(mode string) string {
	if repository.IsAccessMode(mode) {
		return mode
	}
	return repository.AccessModeAll
}
