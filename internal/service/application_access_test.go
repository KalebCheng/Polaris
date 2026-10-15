package service

import (
	"context"
	"errors"
	"strconv"
	"testing"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// ── 测试替身（嵌入接口，只覆盖用到的部分；其余方法为 nil = 一旦被调用即 panic，防止误用）──

type stubSubjectRepo struct {
	repository.ApplicationSubjectRepository
	items []repository.ApplicationSubject
	// byApp 按应用给名单（为空则回落到 items）：用于"同一用户对不同应用结论不同"的用例
	byApp      map[int64][]repository.ApplicationSubject
	replaced   []repository.ApplicationSubject
	lastMode   string
	replaceCnt int
}

func (r *stubSubjectRepo) List(_ context.Context, applicationID int64) ([]repository.ApplicationSubject, error) {
	if r.byApp != nil {
		if items, ok := r.byApp[applicationID]; ok {
			return items, nil
		}
		return nil, nil
	}
	return r.items, nil
}

func (r *stubSubjectRepo) Replace(_ context.Context, _ *repository.Application, mode string, subjects []repository.ApplicationSubject) error {
	r.lastMode, r.replaced, r.replaceCnt = mode, subjects, r.replaceCnt+1
	return nil
}

func (r *stubSubjectRepo) CountByApplications(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

type stubAccessUserRepo struct {
	repository.UserRepository
	users map[string]*repository.User
}

func (r *stubAccessUserRepo) GetByName(_ context.Context, institutionID int64, name string) (*repository.User, error) {
	if u, ok := r.users[name]; ok && u.InstitutionID == institutionID {
		return u, nil
	}
	return nil, errStubNotFound
}

func (r *stubAccessUserRepo) ListByNames(_ context.Context, institutionID int64, names []string) ([]*repository.User, error) {
	out := []*repository.User{}
	for _, n := range names {
		if u, ok := r.users[n]; ok && u.InstitutionID == institutionID {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *stubAccessUserRepo) List(context.Context, repository.ListQuery) ([]*repository.User, error) {
	return nil, nil
}

type stubAccessGroupRepo struct {
	repository.GroupRepository
	// groupsForUser：用户 → 直接归属 + 祖先组（模拟 GroupsForUserWithAncestors 的递归结果）
	groupsForUser map[string][]string
	all           []*repository.Group
	expand        map[string][]string
}

func (r *stubAccessGroupRepo) GroupsForUserWithAncestors(_ context.Context, _ int64, user string) ([]string, error) {
	return r.groupsForUser[user], nil
}

func (r *stubAccessGroupRepo) List(context.Context, repository.ListQuery) ([]*repository.Group, error) {
	return r.all, nil
}

func (r *stubAccessGroupRepo) ExistingNames(_ context.Context, _ int64, names []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, g := range r.all {
		for _, n := range names {
			if g.Name == n {
				out[n] = true
			}
		}
	}
	return out, nil
}

func (r *stubAccessGroupRepo) ExpandMembersWithDescendants(_ context.Context, _ int64, names []string) ([]string, error) {
	out := []string{}
	for _, n := range names {
		out = append(out, r.expand[n]...)
	}
	return out, nil
}

type stubAccessDeptRepo struct {
	repository.DepartmentRepository
	all    []*repository.Department
	expand map[string][]string
}

func (r *stubAccessDeptRepo) GetByID(_ context.Context, id int64) (*repository.Department, error) {
	for _, d := range r.all {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, errStubNotFound
}

func (r *stubAccessDeptRepo) List(context.Context, repository.ListQuery) ([]*repository.Department, error) {
	return r.all, nil
}

func (r *stubAccessDeptRepo) ExistingNames(_ context.Context, _ int64, names []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, d := range r.all {
		for _, n := range names {
			if d.Name == n {
				out[n] = true
			}
		}
	}
	return out, nil
}

func (r *stubAccessDeptRepo) ExpandMembersWithDescendants(_ context.Context, _ int64, names []string) ([]string, error) {
	out := []string{}
	for _, n := range names {
		out = append(out, r.expand[n]...)
	}
	return out, nil
}

// ── 夹具 ─────────────────────────────────────────────────────────

// 组织 1 的目录：
//
//	部门：技术部(tech, code_path=tech) └ 前端组(fe, code_path=tech/fe)
//	用户组：vpn-users(父) └ vpn-dev(子)
//	用户：zhangsan(前端组) / lisi(无部门) / admin(管理员，无部门)
func accessFixture(subjects []repository.ApplicationSubject) (*ApplicationAccessService, *stubSubjectRepo) {
	subjectRepo := &stubSubjectRepo{items: subjects}
	users := &stubAccessUserRepo{users: map[string]*repository.User{
		"zhangsan": {ID: 1, InstitutionID: 1, Name: "zhangsan", DepartmentID: deptIDPtr(2), IsAdmin: false},
		"lisi":     {ID: 2, InstitutionID: 1, Name: "lisi", IsAdmin: false},
		"admin":    {ID: 3, InstitutionID: 1, Name: "admin", IsAdmin: true},
	}}
	groups := &stubAccessGroupRepo{
		groupsForUser: map[string][]string{
			// vpn-dev 的成员在祖先闭包里同时含 vpn-users（"授权父组覆盖子组"的服务端结果）
			"zhangsan": {"vpn-dev", "vpn-users"},
		},
		all: []*repository.Group{
			{ID: 1, InstitutionID: 1, Name: "vpn-users", DisplayName: "VPN 用户"},
			{ID: 2, InstitutionID: 1, Name: "vpn-dev", DisplayName: "VPN 开发", ParentID: groupIDPtr(1)},
		},
		expand: map[string][]string{"vpn-users": {"zhangsan"}},
	}
	depts := &stubAccessDeptRepo{
		all: []*repository.Department{
			{ID: 1, InstitutionID: 1, Name: "tech", DisplayName: "技术部", CodePath: "tech", NamePath: "技术部"},
			{ID: 2, InstitutionID: 1, Name: "fe", DisplayName: "前端组", CodePath: "tech/fe", NamePath: "技术部 / 前端组"},
		},
		expand: map[string][]string{"tech": {"zhangsan"}},
	}
	return NewApplicationAccessService(subjectRepo, users, groups, depts, nil, &AuditRecorder{}), subjectRepo
}

func deptIDPtr(id int64) *int64 { return &id }
func groupIDPtr(id int64) *int64 {
	return &id
}

func accessApp(mode string) *repository.Application {
	return &repository.Application{ID: 7, InstitutionID: 1, Code: "vpn", AccessMode: mode}
}

// accessUser 从服务里取出某个用户夹具（唯一入口，避免在用例中做类型断言）。
func accessUser(t *testing.T, svc *ApplicationAccessService, name string) *repository.User {
	t.Helper()
	repo, ok := svc.users.(*stubAccessUserRepo)
	if !ok {
		t.Fatal("测试夹具的用户仓库类型不符")
	}
	user, ok := repo.users[name]
	if !ok {
		t.Fatalf("用户夹具缺失: %s", name)
	}
	return user
}

func allowedFor(t *testing.T, svc *ApplicationAccessService, app *repository.Application, userName string) bool {
	t.Helper()
	user, err := svc.users.GetByName(context.Background(), app.InstitutionID, userName)
	if err != nil {
		t.Fatalf("用户夹具缺失: %s", userName)
	}
	ok, err := svc.Allowed(context.Background(), app, user)
	if err != nil {
		t.Fatalf("Allowed(%s): %v", userName, err)
	}
	return ok
}

// ── 判定矩阵 ─────────────────────────────────────────────────────

// 非受限应用一律放行（默认 all = 与引入该能力之前的行为一致）。
func TestAccessAllowsWhenModeAll(t *testing.T) {
	svc, subjects := accessFixture(nil)
	if !allowedFor(t, svc, accessApp(repository.AccessModeAll), "lisi") {
		t.Fatal("all 模式应放行")
	}
	if subjects.replaceCnt != 0 || len(subjects.items) != 0 {
		t.Fatal("all 模式不应读取名单")
	}
}

// 受限 + 空名单 = 无人可访问；且**管理员同样被拒**（Spec Q5：没有管理员直通）。
func TestAccessDeniesEmptyRestrictedListIncludingAdmin(t *testing.T) {
	svc, _ := accessFixture(nil)
	app := accessApp(repository.AccessModeRestricted)
	for _, name := range []string{"zhangsan", "lisi", "admin"} {
		if allowedFor(t, svc, app, name) {
			t.Fatalf("受限 + 空名单不应放行任何用户（含管理员），但放行了 %s", name)
		}
	}
	// 空串模式（迁移前的旧行）按默认 all 处理
	if !allowedFor(t, svc, accessApp(""), "lisi") {
		t.Fatal("空模式应按默认全员可访问处理")
	}
}

// 用户主体：名单内放行、名单外拒绝。
func TestAccessUserSubject(t *testing.T) {
	svc, _ := accessFixture([]repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeUser, SubjectName: "zhangsan"},
	})
	app := accessApp(repository.AccessModeRestricted)
	if !allowedFor(t, svc, app, "zhangsan") {
		t.Fatal("名单内的用户应放行")
	}
	if allowedFor(t, svc, app, "lisi") {
		t.Fatal("名单外的用户应拒绝")
	}
}

// 用户组主体：授权父组覆盖子组（服务端取"直接归属 + 祖先组"后求交）。
func TestAccessGroupSubjectCoversChildGroup(t *testing.T) {
	svc, _ := accessFixture([]repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeGroup, SubjectName: "vpn-users"},
	})
	app := accessApp(repository.AccessModeRestricted)
	if !allowedFor(t, svc, app, "zhangsan") {
		t.Fatal("父组被授权时，子组成员应放行")
	}
	if allowedFor(t, svc, app, "lisi") {
		t.Fatal("未归属该组树的用户应拒绝")
	}
}

// 部门主体：授权父部门覆盖子部门（code_path 前缀）。
func TestAccessDepartmentSubjectCoversChildDepartment(t *testing.T) {
	svc, _ := accessFixture([]repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeDepartment, SubjectName: "tech"},
	})
	app := accessApp(repository.AccessModeRestricted)
	if !allowedFor(t, svc, app, "zhangsan") {
		t.Fatal("父部门被授权时，子部门成员应放行")
	}
	if allowedFor(t, svc, app, "lisi") {
		t.Fatal("无部门的用户不应命中部门授权")
	}
	if allowedFor(t, svc, app, "admin") {
		t.Fatal("管理员不在部门内时同样应拒绝（无直通）")
	}
}

// 授权子部门时不覆盖父部门（前缀判定必须是有向的）。
func TestAccessChildDepartmentDoesNotCoverParent(t *testing.T) {
	svc, _ := accessFixture([]repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeDepartment, SubjectName: "fe"},
	})
	// zhangsan 属于 fe（命中）；构造一个属于父部门 tech 的用户验证不命中
	svc.users.(*stubAccessUserRepo).users["wangwu"] = &repository.User{ID: 4, InstitutionID: 1, Name: "wangwu", DepartmentID: deptIDPtr(1)}
	app := accessApp(repository.AccessModeRestricted)
	if !allowedFor(t, svc, app, "zhangsan") {
		t.Fatal("子部门成员应命中子部门授权")
	}
	if allowedFor(t, svc, app, "wangwu") {
		t.Fatal("父部门成员不应命中子部门授权")
	}
}

// Enforce：拒绝时返回 APP_ACCESS_DENIED（认证链路统一拒绝入口）。
func TestEnforceReturnsAppAccessDenied(t *testing.T) {
	svc, _ := accessFixture(nil)
	app := accessApp(repository.AccessModeRestricted)
	user := accessUser(t, svc, "lisi")
	err := svc.Enforce(context.Background(), app, user)
	if err == nil || !bizerrors.Is(err, bizerrors.CodeAppAccessDenied) {
		t.Fatalf("应返回 APP_ACCESS_DENIED，实际 %v", err)
	}
}

// ── 名单读写 ─────────────────────────────────────────────────────

// 空名单允许保存（Spec Q4：不做限制，后续再编辑）。
func TestSetAccessAllowsEmptyList(t *testing.T) {
	svc, subjects := accessFixture([]repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeUser, SubjectName: "zhangsan"},
	})
	app := accessApp(repository.AccessModeAll)
	got, err := svc.SetAccess(context.Background(), "admin", app,
		ApplicationAccessInput{AccessMode: repository.AccessModeRestricted})
	if err != nil {
		t.Fatalf("空名单应允许保存: %v", err)
	}
	if subjects.lastMode != repository.AccessModeRestricted || len(subjects.replaced) != 0 {
		t.Fatalf("应写入 restricted + 空名单，实际 mode=%s subjects=%d", subjects.lastMode, len(subjects.replaced))
	}
	if got.AccessMode != repository.AccessModeRestricted {
		t.Fatalf("返回的访问模式应为 restricted，实际 %s", got.AccessMode)
	}
}

// 非法访问模式被拒绝（不允许用空串把受限应用"静默放宽"）。
func TestSetAccessRejectsInvalidMode(t *testing.T) {
	svc, subjects := accessFixture(nil)
	_, err := svc.SetAccess(context.Background(), "admin", accessApp(repository.AccessModeRestricted),
		ApplicationAccessInput{AccessMode: ""})
	if err == nil || !bizerrors.Is(err, bizerrors.CodeInvalidParams) {
		t.Fatalf("空模式应返回 INVALID_PARAMS，实际 %v", err)
	}
	if subjects.replaceCnt != 0 {
		t.Fatal("校验失败不应写入")
	}
}

// 不存在的主体必须被拒绝（不做静默忽略，避免"以为加了却不生效"）。
func TestSetAccessRejectsUnknownSubjects(t *testing.T) {
	cases := []ApplicationAccessInput{
		{AccessMode: repository.AccessModeRestricted, Users: []string{"nobody"}},
		{AccessMode: repository.AccessModeRestricted, Groups: []string{"nobody"}},
		{AccessMode: repository.AccessModeRestricted, Departments: []string{"nobody"}},
	}
	for _, in := range cases {
		svc, subjects := accessFixture(nil)
		_, err := svc.SetAccess(context.Background(), "admin", accessApp(repository.AccessModeAll), in)
		if err == nil || !bizerrors.Is(err, bizerrors.CodeInvalidParams) {
			t.Fatalf("未知主体应返回 INVALID_PARAMS，实际 %v", err)
		}
		if subjects.replaceCnt != 0 {
			t.Fatal("校验失败不应写入")
		}
	}
}

// 合法名单写入：去重 + 三类主体齐全。
func TestSetAccessWritesNormalizedList(t *testing.T) {
	svc, subjects := accessFixture(nil)
	_, err := svc.SetAccess(context.Background(), "admin", accessApp(repository.AccessModeAll), ApplicationAccessInput{
		AccessMode:  repository.AccessModeRestricted,
		Users:       []string{"zhangsan", "zhangsan", " "},
		Groups:      []string{"vpn-users"},
		Departments: []string{"tech"},
	})
	if err != nil {
		t.Fatalf("SetAccess: %v", err)
	}
	if len(subjects.replaced) != 3 {
		t.Fatalf("应写入 3 条（去重后），实际 %d", len(subjects.replaced))
	}
}

// ExpandUsers：三类主体展开为受影响用户集合（含子部门 / 子用户组的成员）。
func TestExpandUsersCoversSubtreeMembers(t *testing.T) {
	svc, _ := accessFixture(nil)
	names, err := svc.ExpandUsers(context.Background(), 1, []repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeUser, SubjectName: "lisi"},
		{SubjectType: repository.SubjectTypeGroup, SubjectName: "vpn-users"},
		{SubjectType: repository.SubjectTypeDepartment, SubjectName: "tech"},
	})
	if err != nil {
		t.Fatalf("ExpandUsers: %v", err)
	}
	want := []string{"lisi", "zhangsan"}
	if len(names) != len(want) {
		t.Fatalf("受影响用户应为 %v，实际 %v", want, names)
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("受影响用户应为 %v，实际 %v", want, names)
		}
	}
}

// ── ApplicationService：access_mode 的默认值与"更新不得静默放宽"回归 ──

func (r *stubAppRepo) Create(_ context.Context, app *repository.Application) (int64, error) {
	app.ID = int64(len(r.all) + 1)
	r.all = append(r.all, app)
	if r.byName == nil {
		r.byName = map[string]*repository.Application{}
	}
	r.byName[strconv.FormatInt(app.InstitutionID, 10)+"/"+app.Code] = app
	return app.ID, nil
}

func (r *stubAppRepo) Update(_ context.Context, app *repository.Application) error {
	key := strconv.FormatInt(app.InstitutionID, 10) + "/" + app.Code
	if _, ok := r.byName[key]; !ok {
		return errors.New("application not found")
	}
	r.byName[key] = app
	return nil
}

func (r *stubAppRepo) GetByClientID(_ context.Context, clientID string) (*repository.Application, error) {
	for _, app := range r.all {
		if app.ClientID == clientID {
			return app, nil
		}
	}
	return nil, errors.New("application not found")
}

// 新建应用一律"全员可访问"：既不能落空串（违反 CHECK 约束），也不能凭空变成受限。
func TestApplicationCreateDefaultsAccessModeAll(t *testing.T) {
	apps := &stubAppRepo{}
	svc := NewApplicationService(apps, &stubOrgRepo{}, &AuditRecorder{})
	created, err := svc.Create(context.Background(), "admin", &repository.Application{
		InstitutionID: 1, Code: "app1", RedirectURIs: []string{"https://rp.example.com/cb"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.AccessMode != repository.AccessModeAll {
		t.Fatalf("新建应用应为 all，实际 %q", created.AccessMode)
	}
}

// 回归：更新应用主表单（整行 UPDATE）**不得**改动 access_mode。
//
// 反例（本用例锁定）：少了从 existing 的继承，请求体零值 "" 会落库——要么违反 CHECK 约束报 DB_ERROR，
// 要么被兜底成 all 从而**静默把受限应用改成全员可访问**（放宽准入，比"配了不生效"更危险）。
func TestApplicationUpdateKeepsAccessMode(t *testing.T) {
	existing := &repository.Application{
		ID: 9, InstitutionID: 1, Code: "vpn", Name: "VPN",
		AccessMode: repository.AccessModeRestricted, ClientID: "cid", ClientSecret: "cs", State: "active",
		RedirectURIs: []string{"https://rp.example.com/cb"},
	}
	apps := &stubAppRepo{byName: map[string]*repository.Application{"1/vpn": existing}}
	svc := NewApplicationService(apps, &stubOrgRepo{}, &AuditRecorder{})
	// 模拟前端表单：只提交名称等字段，**不带** access_mode
	if err := svc.Update(context.Background(), "admin", &repository.Application{
		InstitutionID: 1, Code: "vpn", Name: "VPN 2", RedirectURIs: []string{"https://rp.example.com/cb"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := apps.byName["1/vpn"].AccessMode; got != repository.AccessModeRestricted {
		t.Fatalf("更新主表单不得改动 access_mode，实际被改成 %q", got)
	}
}

// ── 判定作用域（Spec Q6）：只有显式携带应用上下文才解析出应用 ──

func TestResolveAppContextScope(t *testing.T) {
	app := &repository.Application{ID: 1, InstitutionID: 1, Code: "vpn", ClientID: "cid-vpn"}
	apps := &stubAppRepo{
		byName: map[string]*repository.Application{"1/vpn": app},
		all:    []*repository.Application{app},
	}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{
		// 组织配了默认应用：**不得**被用于准入判定（否则误配会锁死平台）
		"acme": {ID: 1, Name: "acme", DefaultApplication: "vpn"},
	}}
	svc := &AuthService{apps: apps, orgs: orgs}

	// 1) 两者都不带 = 登录平台 → 不解析应用（即便组织有默认应用）
	got, err := svc.resolveAppContext(context.Background(), 1, "acme", "", "")
	if err != nil || got != nil {
		t.Fatalf("未携带应用上下文时应返回 (nil, nil)，实际 (%v, %v)", got, err)
	}
	// 2) client_id 优先
	got, err = svc.resolveAppContext(context.Background(), 1, "acme", "", "cid-vpn")
	if err != nil || got == nil || got.Code != "vpn" {
		t.Fatalf("client_id 应解析出应用，实际 (%v, %v)", got, err)
	}
	// 3) 只有 application（旧链接）
	got, err = svc.resolveAppContext(context.Background(), 1, "acme", "vpn", "")
	if err != nil || got == nil || got.Code != "vpn" {
		t.Fatalf("application 应解析出应用，实际 (%v, %v)", got, err)
	}
	// 4) 未知标识 → APPLICATION_NOT_FOUND（应用驱动入口需明确失败）
	_, err = svc.resolveAppContext(context.Background(), 1, "acme", "nope", "")
	if err == nil || !bizerrors.Is(err, bizerrors.CodeApplicationNotFound) {
		t.Fatalf("未知应用应返回 APPLICATION_NOT_FOUND，实际 %v", err)
	}
}

// ── 门户应用中心按授权过滤（FR-5）──────────────────────────────────

// stubAppAccountRepo 只覆盖 List（门户列表按"有效下游账号"驱动）。
type stubAppAccountRepo struct {
	repository.AppAccountRepository
	items []*repository.AppAccount
}

func (r *stubAppAccountRepo) List(context.Context, repository.ListQuery) ([]*repository.AppAccount, error) {
	return r.items, nil
}

// 回归：门户应用中心必须按访问授权过滤——未授权的应用不出现，也就没有"一键 SSO"入口。
// 判定与登录 / 发令牌同源，不复制第二套逻辑。
func TestPortalApplicationsFilteredByAccess(t *testing.T) {
	subjectRepo := &stubSubjectRepo{
		byApp: map[int64][]repository.ApplicationSubject{
			// 1 = 允许（技术部，zhangsan 命中）；2 = 受限但空名单（无人可访问）；3 = 全员可访问
			1: {{SubjectType: repository.SubjectTypeDepartment, SubjectName: "tech"}},
		},
	}
	users := &stubAccessUserRepo{users: map[string]*repository.User{
		"zhangsan": {ID: 1, InstitutionID: 1, Name: "zhangsan", DepartmentID: deptIDPtr(2)},
	}}
	groups := &stubAccessGroupRepo{}
	depts := &stubAccessDeptRepo{all: []*repository.Department{
		{ID: 1, InstitutionID: 1, Name: "tech", CodePath: "tech"},
		{ID: 2, InstitutionID: 1, Name: "fe", CodePath: "tech/fe"},
	}}
	access := NewApplicationAccessService(subjectRepo, users, groups, depts, nil, &AuditRecorder{})

	apps := &stubAppRepo{byName: map[string]*repository.Application{
		"1/allowed": {ID: 1, InstitutionID: 1, Code: "allowed", Name: "允许", AccessMode: repository.AccessModeRestricted,
			State: "active", RedirectURIs: []string{"https://rp.example.com/cb"}},
		"1/denied": {ID: 2, InstitutionID: 1, Code: "denied", Name: "拒绝", AccessMode: repository.AccessModeRestricted,
			State: "active", RedirectURIs: []string{"https://rp.example.com/cb"}},
		"1/open": {ID: 3, InstitutionID: 1, Code: "open", Name: "开放", AccessMode: repository.AccessModeAll,
			State: "active", RedirectURIs: []string{"https://rp.example.com/cb"}},
	}}
	accounts := &stubAppAccountRepo{items: []*repository.AppAccount{
		{InstitutionID: 1, Application: "allowed", User: "zhangsan", Status: "active"},
		{InstitutionID: 1, Application: "denied", User: "zhangsan", Status: "active"},
		{InstitutionID: 1, Application: "open", User: "zhangsan", Status: "active"},
	}}
	svc := &ProvisioningService{accounts: accounts, connectors: &stubConnectorRepo{}, apps: apps, access: access}

	items, err := svc.ListApplicationsForUser(context.Background(), 1, "zhangsan")
	if err != nil {
		t.Fatalf("ListApplicationsForUser: %v", err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Name)
	}
	if len(got) != 2 || got[0] != "allowed" || got[1] != "open" {
		t.Fatalf("门户应用中心应按授权过滤（受限未授权的不出现），实际 %v", got)
	}
}
