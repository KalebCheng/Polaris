package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/importer"
)

// 用户导入的默认限制（可经 configs/app.yaml 的 import 段与环境变量覆盖）
const (
	DefaultImportMaxRows      = 2000
	DefaultImportBatchSize    = 200
	DefaultImportMaxFileBytes = 10 << 20
	DefaultImportRunsPerHour  = 10
	// importRunTimeout 续跑硬上限：脱离请求上下文后也必须终止
	importRunTimeout = 30 * time.Minute
)

// defaultImportDelimiter 多值单元格的默认分隔符
const defaultImportDelimiter = ";"

// importUserNamePattern 账号名的字符集约束（与登录入口可接受的字符保持一致）
var importUserNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// UserImportConfig 导入限制
type UserImportConfig struct {
	MaxRows      int
	BatchSize    int
	MaxFileBytes int64
	RunsPerHour  int
}

// UserImportService 用户导入：解析 → 逐行校验 → 分批创建 → 报告 → 记录。
//
// 写入复用 UserService.CreateUserBatch（密码策略 / 审计 / 下游供给与手工建号同一条路径）；
// 导入只做新建：已存在的账号名 / 邮箱一律跳过，绝不改写存量用户。
type UserImportService struct {
	users    *UserService
	userRepo repository.UserRepository
	depts    repository.DepartmentRepository
	deptSvc  departmentCreator
	roles    repository.RoleRepository
	titles   repository.JobTitleRepository
	levels   repository.JobLevelRepository
	imports  repository.UserImportRepository
	audits   *AuditRecorder
	cfg      UserImportConfig
}

// NewUserImportService 创建导入服务。
func NewUserImportService(
	users *UserService,
	userRepo repository.UserRepository,
	depts repository.DepartmentRepository,
	deptSvc departmentCreator,
	roles repository.RoleRepository,
	titles repository.JobTitleRepository,
	levels repository.JobLevelRepository,
	imports repository.UserImportRepository,
	audits *AuditRecorder,
	cfg UserImportConfig,
) *UserImportService {
	if cfg.MaxRows <= 0 {
		cfg.MaxRows = DefaultImportMaxRows
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultImportBatchSize
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = DefaultImportMaxFileBytes
	}
	if cfg.RunsPerHour <= 0 {
		cfg.RunsPerHour = DefaultImportRunsPerHour
	}
	return &UserImportService{
		users: users, userRepo: userRepo, depts: depts, deptSvc: deptSvc, roles: roles, titles: titles,
		levels: levels, imports: imports, audits: audits, cfg: cfg,
	}
}

// Config 返回生效的导入限制（handler 在解析上传体前读取文件大小上限）。
func (s *UserImportService) Config() UserImportConfig { return s.cfg }

// departmentCreator 本服务实际用到的部门创建能力（DepartmentService 实现）。
type departmentCreator interface {
	Create(ctx context.Context, actor string, d *repository.Department) (*repository.Department, error)
}

// UserImportOptions 一次导入的选项
type UserImportOptions struct {
	InstitutionID   int64
	Operator        string
	CanAssignRoles  bool
	DefaultJobLevel *int64
	Delimiter       string
	AutoCreateDepts bool
}

// UserImportRowResult 逐行结果
type UserImportRowResult struct {
	RowNumber     int                             `json:"row"`
	UserName      string                          `json:"name"`
	Email         string                          `json:"email"`
	Status        string                          `json:"status"`
	CreatedUserID *int64                          `json:"created_user_id,omitempty"`
	Errors        []repository.UserImportRowIssue `json:"errors"`
	Warnings      []repository.UserImportRowIssue `json:"warnings"`
}

// UserImportSummary 汇总
type UserImportSummary struct {
	Total              int      `json:"total"`
	Created            int      `json:"created"`
	Skipped            int      `json:"skipped"`
	Failed             int      `json:"failed"`
	CreatedDepartments []string `json:"created_departments"`
}

// UserImportReport 导入报告（请求内返回，同时落库）
type UserImportReport struct {
	ImportID int64                 `json:"import_id"`
	Status   string                `json:"status"`
	Summary  UserImportSummary     `json:"summary"`
	Rows     []UserImportRowResult `json:"rows"`
}

// Import 解析文件并执行导入。参数类错误（格式 / 表头 / 行数超限）不产生导入记录。
func (s *UserImportService) Import(
	ctx context.Context, opts UserImportOptions, fileName string, content []byte,
) (*UserImportReport, error) {
	if opts.InstitutionID == 0 {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id is required")
	}
	if len(content) == 0 {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "file is required")
	}
	if int64(len(content)) > s.cfg.MaxFileBytes {
		return nil, bizerrors.NewWithCode(bizerrors.CodeFileSizeExceeded).WithDetail("max_bytes", s.cfg.MaxFileBytes)
	}
	delimiter := strings.TrimSpace(opts.Delimiter)
	if delimiter == "" {
		delimiter = defaultImportDelimiter
	}
	parsed, err := importer.Parse(importer.ParseInput{
		FileName: fileName, Content: content, Delimiter: delimiter, MaxRows: s.cfg.MaxRows,
	})
	if err != nil {
		return nil, mapImportParseError(err, s.cfg.MaxRows)
	}
	p := &importRun{svc: s, ctx: ctx, opts: opts, delimiter: delimiter}
	return p.run(parsed.Rows, fileName, string(parsed.Format))
}

// importRun 单次导入的执行状态
type importRun struct {
	svc       *UserImportService
	ctx       context.Context
	opts      UserImportOptions
	delimiter string
	record    *repository.UserImport
	org       *repository.Institution
	rows      []*rowWork
	// autoCreated 本次自动创建的部门路径（去重，按首次出现顺序）
	autoCreated []string
}

// rowWork 单行的预校验与执行状态
type rowWork struct {
	raw       importer.Row
	result    UserImportRowResult
	input     *CreateUserBatchInput
	persisted bool
}

func (p *importRun) run(rows []importer.Row, fileName, format string) (*UserImportReport, error) {
	s := p.svc
	p.record = &repository.UserImport{
		InstitutionID: p.opts.InstitutionID, FileName: fileName, FileFormat: format,
		Status: repository.UserImportQueued, TotalRows: len(rows),
		AutoCreateDepts: p.opts.AutoCreateDepts, Operator: p.opts.Operator,
	}
	if err := s.imports.Create(p.ctx, p.record); err != nil {
		return nil, err
	}
	// 客户端断开不应中断已开始的导入：状态更新与写入都脱离请求取消信号，并受硬上限约束
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(p.ctx), importRunTimeout)
	defer cancel()
	p.ctx = runCtx
	p.record.Status = repository.UserImportRunning
	_ = s.imports.Update(p.ctx, p.record)

	p.precheck(rows)
	pending := make([]*rowWork, 0, len(rows))
	for i := range p.rows {
		if p.rows[i].input != nil {
			pending = append(pending, p.rows[i])
		}
	}
	p.persistProgress()
	for start := 0; start < len(pending); start += s.cfg.BatchSize {
		end := min(start+s.cfg.BatchSize, len(pending))
		p.commitBatch(pending[start:end])
		p.persistProgress()
	}
	p.finish()
	return p.report(), nil
}

// precheck 逐行校验：必填 / 格式 / 密码策略 / 冲突 / 依赖名称解析。
func (p *importRun) precheck(rows []importer.Row) {
	s := p.svc
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Name != "" {
			names = append(names, r.Name)
		}
	}
	existingNames := map[string]bool{}
	if items, err := s.userRepo.ListByNames(p.ctx, p.opts.InstitutionID, names); err == nil {
		for _, u := range items {
			existingNames[u.Name] = true
		}
	}
	seenName := make(map[string]int, len(rows))
	seenEmail := make(map[string]int, len(rows))
	p.rows = make([]*rowWork, 0, len(rows))
	for _, r := range rows {
		w := &rowWork{raw: r, result: UserImportRowResult{
			RowNumber: r.Number, UserName: r.Name, Email: r.Email,
			Errors: []repository.UserImportRowIssue{}, Warnings: []repository.UserImportRowIssue{},
		}}
		p.precheckRow(w, existingNames, seenName, seenEmail)
		w.raw.Password = ""
		p.rows = append(p.rows, w)
	}
}

func (p *importRun) precheckRow(
	w *rowWork, existingNames map[string]bool, seenName, seenEmail map[string]int,
) {
	s := p.svc
	r := w.raw
	fail := func(column, msg string) {
		w.result.Errors = append(w.result.Errors, repository.UserImportRowIssue{Column: column, Message: msg})
	}
	skip := func(column, msg string) {
		w.result.Status = repository.UserImportRowSkipped
		w.result.Warnings = append(w.result.Warnings, repository.UserImportRowIssue{Column: column, Message: msg})
	}
	// 1) 行内格式与密码策略
	if r.Name == "" {
		fail("name", "name is required")
	} else if !importUserNamePattern.MatchString(r.Name) {
		fail("name", "name may only contain letters, digits, dot, underscore and hyphen")
	}
	if r.DisplayName == "" {
		fail("display_name", "display_name is required")
	}
	if r.Email != "" && !strings.Contains(r.Email, "@") {
		fail("email", "invalid email address")
	}
	if r.Password == "" {
		fail("password", "password is required")
	} else if err := p.validatePassword(r.Password); err != nil {
		fail("password", err.Error())
	}
	if len(w.result.Errors) > 0 {
		w.result.Status = repository.UserImportRowFailed
		return
	}

	// 2) 冲突：库内已存在 → 跳过；文件内重复 → 跳过并指向首次出现的行
	if existingNames[r.Name] {
		skip("name", "user already exists")
		return
	}
	if first, ok := seenName[r.Name]; ok {
		skip("name", fmt.Sprintf("duplicate of row %d in this file", first))
		return
	}
	seenName[r.Name] = r.Number
	if r.Email != "" {
		key := strings.ToLower(r.Email)
		if first, ok := seenEmail[key]; ok {
			skip("email", fmt.Sprintf("duplicate of row %d in this file", first))
			return
		}
		seenEmail[key] = r.Number
		if u, err := s.userRepo.GetByEmail(p.ctx, p.opts.InstitutionID, r.Email); err == nil && u != nil {
			skip("email", "email already used by another user")
			return
		}
	}
	if len(r.Roles) > 0 && !p.opts.CanAssignRoles {
		fail("roles", "assigning roles requires role:update permission")
	}

	// 3) 依赖资源按名称解析（严格：任一不存在该行失败）
	input := &CreateUserBatchInput{
		User: &repository.User{
			InstitutionID:  p.opts.InstitutionID,
			Name:           r.Name,
			DisplayName:    r.DisplayName,
			FirstName:      r.FirstName,
			LastName:       r.LastName,
			Email:          r.Email,
			Phone:          r.Phone,
			CountryCode:    r.CountryCode,
			Title:          r.Title,
			RegisterType:   "import",
			RegisterSource: p.record.FileName,
		},
		Password: r.Password,
		// RegisterSource 在批写入时落到实体（prepareImportUser 统一赋值）
		RegisterSource: p.record.FileName,
	}
	if r.Department != "" {
		deptID, err := p.resolveDepartment(r.Department)
		if err != nil {
			fail("department", err.Error())
		} else if deptID != nil {
			input.Departments = []int64{*deptID}
		}
	}
	if r.JobLevel != "" {
		lv, err := s.levels.GetByName(p.ctx, p.opts.InstitutionID, r.JobLevel)
		if err != nil || lv == nil {
			fail("job_level", fmt.Sprintf("job level not found: %s", r.JobLevel))
		} else {
			input.User.JobLevelID = &lv.ID
		}
	} else if p.opts.DefaultJobLevel != nil {
		input.User.JobLevelID = p.opts.DefaultJobLevel
	}
	for _, name := range r.JobTitles {
		jt, err := s.titles.GetByName(p.ctx, p.opts.InstitutionID, name)
		if err != nil || jt == nil {
			fail("job_titles", fmt.Sprintf("job title not found: %s", name))
			continue
		}
		input.JobTitleIDs = append(input.JobTitleIDs, jt.ID)
	}
	for _, name := range r.Roles {
		if !p.opts.CanAssignRoles {
			break
		}
		role, err := s.roles.GetByName(p.ctx, p.opts.InstitutionID, name)
		if err != nil || role == nil {
			fail("roles", fmt.Sprintf("role not found: %s", name))
			continue
		}
		input.RoleIDs = append(input.RoleIDs, role.ID)
		input.RoleNames = append(input.RoleNames, role.Name)
	}
	if len(w.result.Errors) > 0 {
		w.result.Status = repository.UserImportRowFailed
		return
	}
	w.input = input
}

// validatePassword 按组织密码策略校验（与手工建号同一规则；新用户没有密码历史故不查重）。
func (p *importRun) validatePassword(password string) error {
	if p.org == nil {
		org, err := p.svc.users.orgs.GetByID(p.ctx, p.opts.InstitutionID)
		if err != nil {
			return fmt.Errorf("institution not found")
		}
		p.org = org
	}
	return auth.FromOptions(p.org.PasswordOptions).Validate(password)
}

// resolveDepartment 解析部门路径 "a/b/c"（按名称逐级匹配，校验父子关系）。
// 默认严格：任一级不存在即失败；auto_create_departments=true 时逐级补建（新建路径记入报告）。
func (p *importRun) resolveDepartment(path string) (*int64, error) {
	s := p.svc
	clean := strings.Trim(strings.TrimSpace(path), "/")
	segments := strings.Split(clean, "/")
	for i, seg := range segments {
		segments[i] = strings.TrimSpace(seg)
		if segments[i] == "" {
			return nil, fmt.Errorf("invalid department path: %s", path)
		}
	}
	var parent *repository.Department
	for _, name := range segments {
		d, err := s.depts.GetByName(p.ctx, p.opts.InstitutionID, name)
		if err != nil || d == nil {
			if !p.opts.AutoCreateDepts {
				return nil, fmt.Errorf("department not found: %s", clean)
			}
			var parentID *int64
			if parent != nil {
				id := parent.ID
				parentID = &id
			}
			created, cerr := s.deptSvc.Create(p.ctx, p.opts.Operator, &repository.Department{
				InstitutionID: p.opts.InstitutionID, Name: name, DisplayName: name, ParentID: parentID,
			})
			if cerr != nil {
				return nil, fmt.Errorf("create department %s failed", name)
			}
			path := clean
			if created.NamePath != "" {
				path = created.NamePath
			}
			if !containsString(p.autoCreated, path) {
				p.autoCreated = append(p.autoCreated, path)
			}
			parent = created
			continue
		}
		// 同名部门在路径中间出现时，必须真的挂在上一级之下（同组织内部门名允许重复）
		if parent != nil && !inAncestorChain(d, parent.ID) {
			return nil, fmt.Errorf("department path mismatch: %s is not under %s", name, parent.NamePath)
		}
		parent = d
	}
	if parent == nil {
		return nil, nil
	}
	id := parent.ID
	return &id, nil
}

// inAncestorChain 判断 parentID 是否在 d 的祖先链中（含直接父级；祖先链不含自身）。
func inAncestorChain(d *repository.Department, parentID int64) bool {
	if d.ParentID != nil && *d.ParentID == parentID {
		return true
	}
	for _, id := range d.ParentIDChain {
		if id == parentID {
			return true
		}
	}
	return false
}

func emptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// commitBatch 写入一批；整批失败时逐行重试以定位失败行（其余行仍算成功）。
func (p *importRun) commitBatch(batch []*rowWork) {
	s := p.svc
	inputs := make([]CreateUserBatchInput, len(batch))
	for i := range batch {
		inputs[i] = *batch[i].input
	}
	if results, err := s.users.CreateUserBatch(p.ctx, p.opts.Operator, p.opts.InstitutionID, inputs); err == nil {
		for i := range batch {
			p.markCreated(batch[i], results[i].User.ID)
		}
		return
	}
	for _, w := range batch {
		retry, retryErr := s.users.CreateUserBatch(p.ctx, p.opts.Operator, p.opts.InstitutionID,
			[]CreateUserBatchInput{*w.input})
		if retryErr != nil {
			w.result.Status = repository.UserImportRowFailed
			w.result.Errors = append(w.result.Errors, repository.UserImportRowIssue{
				Message: importErrorText(retryErr),
			})
			continue
		}
		p.markCreated(w, retry[0].User.ID)
	}
}

func (p *importRun) markCreated(w *rowWork, id int64) {
	w.result.Status = repository.UserImportRowCreated
	w.result.CreatedUserID = &id
}

// persistProgress 落定尚未写库的行并刷新汇总（进度轮询读的就是这份）。
func (p *importRun) persistProgress() {
	s := p.svc
	dirty := make([]*repository.UserImportRow, 0, len(p.rows))
	for _, w := range p.rows {
		if w.result.Status == "" || w.persisted {
			continue
		}
		w.persisted = true
		row := &repository.UserImportRow{
			ImportID: p.record.ID, RowNumber: w.result.RowNumber, UserName: w.result.UserName,
			Email: w.result.Email, Status: w.result.Status,
			Errors: w.result.Errors, Warnings: w.result.Warnings,
		}
		if id := w.result.CreatedUserID; id != nil {
			v := *id
			row.CreatedUserID = &v
		}
		dirty = append(dirty, row)
	}
	if len(dirty) > 0 {
		_ = s.imports.CreateRows(p.ctx, dirty)
	}
	p.recount()
	p.record.CreatedDepartments = emptyStrings(p.autoCreated)
	p.record.Status = repository.UserImportRunning
	_ = s.imports.Update(p.ctx, p.record)
}

func (p *importRun) recount() {
	p.record.CreatedCount, p.record.SkippedCount, p.record.FailedCount = 0, 0, 0
	for _, w := range p.rows {
		switch w.result.Status {
		case repository.UserImportRowCreated:
			p.record.CreatedCount++
		case repository.UserImportRowSkipped:
			p.record.SkippedCount++
		case repository.UserImportRowFailed:
			p.record.FailedCount++
		}
	}
}

// finish 汇总终态并刷新仍未落库的行，随后审计一条 user.import。
func (p *importRun) finish() {
	s := p.svc
	p.persistProgress()
	switch {
	case p.record.CreatedCount == len(p.rows):
		p.record.Status = repository.UserImportSucceeded
	case p.record.CreatedCount == 0:
		p.record.Status = repository.UserImportFailed
	default:
		p.record.Status = repository.UserImportPartial
	}
	_ = s.imports.Update(p.ctx, p.record)
	s.audits.Record(p.ctx, p.opts.Operator, p.opts.InstitutionID, "user.import", p.record.FileName,
		map[string]interface{}{
			"import_id": p.record.ID, "total": p.record.TotalRows, "created": p.record.CreatedCount,
			"skipped": p.record.SkippedCount, "failed": p.record.FailedCount,
		})
}

func (p *importRun) report() *UserImportReport {
	rows := make([]UserImportRowResult, 0, len(p.rows))
	for _, w := range p.rows {
		rows = append(rows, w.result)
	}
	return &UserImportReport{
		ImportID: p.record.ID,
		Status:   p.record.Status,
		Summary: UserImportSummary{
			Total: p.record.TotalRows, Created: p.record.CreatedCount, Skipped: p.record.SkippedCount,
			Failed: p.record.FailedCount, CreatedDepartments: emptyStrings(p.autoCreated),
		},
		Rows: rows,
	}
}

// MarkStaleFailed 把超时仍处于 queued/running 的记录收敛为 failed
// （处理请求的实例在续跑前崩溃时；见 Spec R6）。
func (s *UserImportService) MarkStaleFailed(ctx context.Context, olderThan time.Duration) (int64, error) {
	return s.imports.MarkStaleFailed(ctx, time.Now().Add(-olderThan))
}

// List 导入记录分页
func (s *UserImportService) List(ctx context.Context, q repository.UserImportListQuery) ([]*repository.UserImport, int64, error) {
	items, err := s.imports.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	if items == nil {
		items = []*repository.UserImport{}
	}
	total, err := s.imports.Count(ctx, q.InstitutionID)
	return items, total, err
}

// Detail 导入记录 + 逐行结果
func (s *UserImportService) Detail(ctx context.Context, institutionID, importID int64) (*repository.UserImport, []*repository.UserImportRow, error) {
	rec, err := s.imports.Get(ctx, institutionID, importID)
	if err != nil {
		return nil, nil, bizerrors.ErrNotFound.WithDetail("import_id", importID)
	}
	rows, err := s.imports.ListRows(ctx, importID)
	if err != nil {
		return nil, nil, err
	}
	if rows == nil {
		rows = []*repository.UserImportRow{}
	}
	return rec, rows, nil
}

// mapImportParseError 解析错误 → 业务错误（参数类错误，不产生导入记录）。
func mapImportParseError(err error, maxRows int) error {
	var headerErr *importer.HeaderError
	var tooMany importer.ErrTooManyRows
	switch {
	case errors.As(err, &headerErr):
		return bizerrors.New(bizerrors.CodeInvalidParams, "import header does not match the template").
			WithDetail("header_missing", headerErr.Missing).
			WithDetail("header_unexpected", headerErr.Unexpected).
			WithDetail("header_out_of_order", headerErr.OutOfOrder)
	case errors.As(err, &tooMany):
		return bizerrors.New(bizerrors.CodeInvalidParams, fmt.Sprintf("import file exceeds %d rows", maxRows)).
			WithDetail("max_rows", maxRows)
	case errors.Is(err, importer.ErrUnsupportedFormat):
		return bizerrors.New(bizerrors.CodeInvalidParams, "only .xlsx and .csv files are supported")
	case errors.Is(err, importer.ErrFileMismatch):
		return bizerrors.New(bizerrors.CodeInvalidParams, "file content does not match its extension")
	case errors.Is(err, importer.ErrEmptyFile):
		return bizerrors.New(bizerrors.CodeInvalidParams, "import file has no data rows")
	default:
		return bizerrors.Wrap(bizerrors.CodeInternalError, "parse import file failed", err)
	}
}

// importErrorText 把写入错误转成逐行错误文案（不暴露底层细节；业务错误用其 message）。
func importErrorText(err error) string {
	var be *bizerrors.Error
	if errors.As(err, &be) {
		return be.Error()
	}
	return "failed to create user"
}
