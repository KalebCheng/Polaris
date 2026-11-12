package service

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/importer"
)

// —— 测试替身 ——

type importUserRepo struct {
	repository.UserRepository
	existing    []*repository.User
	created     []repository.UserBatchItem
	failForName map[string]bool
}

func (r *importUserRepo) ListByNames(context.Context, int64, []string) ([]*repository.User, error) {
	return r.existing, nil
}

func (r *importUserRepo) GetByEmail(context.Context, int64, string) (*repository.User, error) {
	return nil, bizerrors.ErrNotFound
}

func (r *importUserRepo) CreateManyWithRelations(_ context.Context, items []repository.UserBatchItem) error {
	for _, item := range items {
		if r.failForName[item.User.Name] {
			return bizerrors.New(bizerrors.CodeInternalError, "boom")
		}
	}
	for i := range items {
		items[i].User.ID = int64(100 + len(r.created) + i + 1)
		r.created = append(r.created, items[i])
	}
	return nil
}

type importOrgRepo struct {
	repository.InstitutionRepository
}

func (r *importOrgRepo) GetByID(context.Context, int64) (*repository.Institution, error) {
	return &repository.Institution{ID: 7, Name: "acme"}, nil
}

type importDeptRepo struct {
	repository.DepartmentRepository
	depts map[string]*repository.Department
}

func (r *importDeptRepo) GetByName(_ context.Context, _ int64, name string) (*repository.Department, error) {
	if d, ok := r.depts[name]; ok {
		return d, nil
	}
	return nil, bizerrors.New(bizerrors.CodeInternalError, "not found")
}

// importDeptSvc 自动建部门：在父部门路径下追加一级（与 DepartmentService 的回填语义一致）。
type importDeptSvc struct{ repo *importDeptRepo }

func (s *importDeptSvc) Create(_ context.Context, _ string, d *repository.Department) (*repository.Department, error) {
	created := *d
	created.ID = int64(1000 + len(s.repo.depts))
	created.NamePath = created.Name
	if d.ParentID != nil {
		for _, parent := range s.repo.depts {
			if parent.ID == *d.ParentID {
				created.NamePath = parent.NamePath + "/" + created.Name
				break
			}
		}
	}
	s.repo.depts[created.Name] = &created
	return &created, nil
}

type importJobTitleRepo struct {
	repository.JobTitleRepository
	items map[string]int64
}

func (r *importJobTitleRepo) GetByName(_ context.Context, _ int64, name string) (*repository.JobTitle, error) {
	if id, ok := r.items[name]; ok {
		return &repository.JobTitle{ID: id}, nil
	}
	return nil, bizerrors.ErrNotFound
}

type importRoleRepo struct {
	repository.RoleRepository
	items map[string]int64
}

func (r *importRoleRepo) GetByName(_ context.Context, _ int64, name string) (*repository.Role, error) {
	if id, ok := r.items[name]; ok {
		return &repository.Role{ID: id}, nil
	}
	return nil, bizerrors.ErrNotFound
}

type importLevelRepo struct {
	repository.JobLevelRepository
}

func (r *importLevelRepo) GetByName(context.Context, int64, string) (*repository.JobLevel, error) {
	return nil, bizerrors.ErrNotFound
}

type importRecordRepo struct {
	repository.UserImportRepository
	record *repository.UserImport
	rows   map[int]*repository.UserImportRow
}

func (r *importRecordRepo) Create(_ context.Context, item *repository.UserImport) error {
	item.ID = 42
	r.record = item
	r.rows = map[int]*repository.UserImportRow{}
	return nil
}

func (r *importRecordRepo) Update(_ context.Context, item *repository.UserImport) error {
	r.record = item
	return nil
}

func (r *importRecordRepo) CreateRows(_ context.Context, rows []*repository.UserImportRow) error {
	for _, row := range rows {
		r.rows[row.RowNumber] = row
	}
	return nil
}

type importFixture struct {
	svc         *UserImportService
	users       *importUserRepo
	records     *importRecordRepo
	departments *importDeptRepo
}

func newImportFixture() *importFixture {
	users := &importUserRepo{
		existing:    []*repository.User{{ID: 1, Name: "existing"}},
		failForName: map[string]bool{},
	}
	userSvc := NewUserService(users, &importOrgRepo{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	records := &importRecordRepo{}
	depts := &importDeptRepo{depts: map[string]*repository.Department{
		"研发部": {ID: 11, Name: "研发部", NamePath: "研发部"},
	}}
	svc := NewUserImportService(
		userSvc, users, depts, &importDeptSvc{repo: depts},
		&importRoleRepo{items: map[string]int64{"开发者": 31}},
		&importJobTitleRepo{items: map[string]int64{"前端负责人": 21}},
		&importLevelRepo{},
		records, &AuditRecorder{}, UserImportConfig{BatchSize: 2, MaxRows: 100},
	)
	return &importFixture{svc: svc, users: users, records: records, departments: depts}
}

type importRow map[string]string

func buildCSV(t *testing.T, rows ...importRow) []byte {
	t.Helper()
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	header := make([]string, 0, len(importer.Columns()))
	for _, c := range importer.Columns() {
		header = append(header, c.HeaderEN)
	}
	_ = writer.Write(header)
	for _, row := range rows {
		line := make([]string, 0, len(header))
		for _, c := range importer.Columns() {
			line = append(line, row[c.Key])
		}
		_ = writer.Write(line)
	}
	writer.Flush()
	return []byte(buf.String())
}

func importRowNamed(name string) importRow {
	return importRow{
		"name": name, "display_name": name, "password": "Passw0rd!",
		"department": "研发部", "job_titles": "前端负责人",
	}
}

func (f *importFixture) run(t *testing.T, content []byte, mutate func(*UserImportOptions)) *UserImportReport {
	t.Helper()
	opts := UserImportOptions{InstitutionID: 7, Operator: "admin"}
	if mutate != nil {
		mutate(&opts)
	}
	report, err := f.svc.Import(context.Background(), opts, "batch.csv", content)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	return report
}

// 成功 + 冲突跳过 + 依赖不存在，各自带行号与原因；跳过行不改写存量用户。
func TestImportCreatesSkipsAndFailsRows(t *testing.T) {
	f := newImportFixture()
	report := f.run(t, buildCSV(t,
		importRowNamed("alice"),
		importRowNamed("existing"),
		importRow{"name": "bob", "display_name": "Bob", "password": "Passw0rd!", "department": "不存在的部门"},
	), nil)

	if report.Summary.Total != 3 || report.Summary.Created != 1 || report.Summary.Skipped != 1 || report.Summary.Failed != 1 {
		t.Fatalf("汇总不符: %+v", report.Summary)
	}
	if report.Status != repository.UserImportPartial {
		t.Fatalf("状态应为 partial: %s", report.Status)
	}
	if report.Rows[0].Status != repository.UserImportRowCreated || report.Rows[0].CreatedUserID == nil {
		t.Fatalf("第 2 行应创建成功: %+v", report.Rows[0])
	}
	if report.Rows[1].Status != repository.UserImportRowSkipped || report.Rows[1].Warnings[0].Message != "user already exists" {
		t.Fatalf("第 3 行应因已存在被跳过: %+v", report.Rows[1])
	}
	if report.Rows[2].Status != repository.UserImportRowFailed || !strings.Contains(report.Rows[2].Errors[0].Message, "department not found") {
		t.Fatalf("第 4 行应因部门不存在失败: %+v", report.Rows[2])
	}
	if len(f.users.created) != 1 {
		t.Fatalf("只应创建 1 个用户（跳过行不写入），实际 %d", len(f.users.created))
	}
	created := f.users.created[0]
	if len(created.Departments) != 1 || created.Departments[0] != 11 || len(created.JobTitleIDs) != 1 {
		t.Fatalf("部门 / 职务关联未写入: %+v", created)
	}
	if created.User.PasswordHash == "" || created.User.PasswordHash == "Passw0rd!" {
		t.Fatal("密码必须以哈希写入，且不得明文落库")
	}
	if created.User.RegisterType != "import" || created.User.RegisterSource != "batch.csv" {
		t.Fatalf("账号来源应标记为导入: %q / %q", created.User.RegisterType, created.User.RegisterSource)
	}
	if f.records.record.Status != repository.UserImportPartial || len(f.records.rows) != 3 {
		t.Fatalf("导入记录应为 partial 且逐行落库: %+v rows=%d", f.records.record.Status, len(f.records.rows))
	}
	for _, row := range f.records.rows {
		if strings.Contains(row.UserName, "Passw0rd") {
			t.Fatal("导入记录不得包含密码")
		}
	}
}

// 文件内重复：后者跳过并指向首次出现的行号。
func TestImportSkipsInFileDuplicates(t *testing.T) {
	f := newImportFixture()
	report := f.run(t, buildCSV(t, importRowNamed("carol"), importRowNamed("carol")), nil)
	if report.Summary.Created != 1 || report.Summary.Skipped != 1 {
		t.Fatalf("文件内重复应只创建一次: %+v", report.Summary)
	}
	if msg := report.Rows[1].Warnings[0].Message; msg != "duplicate of row 2 in this file" {
		t.Fatalf("重复行应指向首次出现的行号: %s", msg)
	}
}

// 角色列需要 role:update：无权限时该行失败（整单仍可导入其它行）。
func TestImportRolesRequirePermission(t *testing.T) {
	f := newImportFixture()
	row := importRowNamed("dave")
	row["roles"] = "开发者"
	report := f.run(t, buildCSV(t, row), nil)
	if report.Summary.Failed != 1 || report.Rows[0].Errors[0].Column != "roles" {
		t.Fatalf("无 role:update 时角色列应报错: %+v", report.Rows[0])
	}

	report = f.run(t, buildCSV(t, row), func(o *UserImportOptions) { o.CanAssignRoles = true })
	if report.Summary.Created != 1 {
		t.Fatalf("有权限时应导入成功: %+v", report.Summary)
	}
	if got := f.users.created[len(f.users.created)-1].RoleIDs; len(got) != 1 || got[0] != 31 {
		t.Fatalf("角色关联未写入: %v", got)
	}
}

// 密码策略按组织校验，且错误只带列名不重复密码本身。
func TestImportValidatesPasswordPolicy(t *testing.T) {
	f := newImportFixture()
	row := importRowNamed("erin")
	row["password"] = "短"
	report := f.run(t, buildCSV(t, row), nil)
	if report.Rows[0].Status != repository.UserImportRowFailed || report.Rows[0].Errors[0].Column != "password" {
		t.Fatalf("弱密码应判失败: %+v", report.Rows[0])
	}
	if strings.Contains(report.Rows[0].Errors[0].Message, "短") {
		t.Fatal("错误信息不得回显密码")
	}
}

// 批次整批失败时逐行重试：仅失败行落 failed，其余仍创建。
func TestImportBatchFailureIsIsolatedPerRow(t *testing.T) {
	f := newImportFixture()
	f.users.failForName["bad"] = true
	report := f.run(t, buildCSV(t,
		importRowNamed("u1"), importRowNamed("bad"), importRowNamed("u3"),
	), nil)
	if report.Summary.Created != 2 || report.Summary.Failed != 1 {
		t.Fatalf("批失败应逐行隔离: %+v", report.Summary)
	}
	if report.Rows[1].Status != repository.UserImportRowFailed || report.Rows[2].Status != repository.UserImportRowCreated {
		t.Fatalf("失败行定位错误: %+v", report.Rows)
	}
}

// 自动创建部门：逐级补建并记入报告；关闭时严格失败。
func TestImportAutoCreateDepartments(t *testing.T) {
	f := newImportFixture()
	row := importRowNamed("frank")
	row["department"] = "研发部/平台组/基础架构"
	report := f.run(t, buildCSV(t, row), func(o *UserImportOptions) { o.AutoCreateDepts = true })
	if report.Summary.Created != 1 {
		t.Fatalf("开启自动建部门后应成功: %+v", report.Rows[0])
	}
}
