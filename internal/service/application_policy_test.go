package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"polaris/internal/config"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type stubAppRepo struct {
	repository.ApplicationRepository
	byName map[string]*repository.Application
	all    []*repository.Application
}

func (r *stubAppRepo) GetByCode(_ context.Context, institutionID int64, code string) (*repository.Application, error) {
	if app := r.byName[strconv.FormatInt(institutionID, 10)+"/"+code]; app != nil {
		return app, nil
	}
	return nil, errors.New("application not found")
}

func (r *stubAppRepo) List(_ context.Context, q repository.ListQuery) ([]*repository.Application, error) {
	out := make([]*repository.Application, 0, len(r.all))
	for _, app := range r.all {
		if q.InstitutionID == 0 || app.InstitutionID == q.InstitutionID {
			out = append(out, app)
		}
	}
	return out, nil
}

type stubOrgRepo struct {
	repository.InstitutionRepository
	byName map[string]*repository.Institution
}

func (r *stubOrgRepo) GetByID(_ context.Context, id int64) (*repository.Institution, error) {
	for _, org := range r.byName {
		if org.ID == id {
			return org, nil
		}
	}
	return &repository.Institution{ID: id, Name: "acme"}, nil
}

func (r *stubOrgRepo) GetByName(_ context.Context, name string) (*repository.Institution, error) {
	if org := r.byName[name]; org != nil {
		return org, nil
	}
	return nil, errors.New("institution not found")
}

func newPolicyService(apps *stubAppRepo, orgs *stubOrgRepo, defaultInstitution string) *AuthService {
	return &AuthService{
		apps:     apps,
		orgs:     orgs,
		oauthCfg: config.OAuthConfig{DefaultInstitution: defaultInstitution},
	}
}

// 组织无任何应用时，宽松解析返回 (nil, nil)——登录不应因缺应用而失败。
func TestResolveApplicationOptionalNoApps(t *testing.T) {
	svc := newPolicyService(&stubAppRepo{}, &stubOrgRepo{}, "built-in")
	app, err := svc.resolveApplicationOptional(context.Background(), "acme", "")
	if err != nil {
		t.Fatalf("期望无错误，得到 %v", err)
	}
	if app != nil {
		t.Fatalf("期望 nil 应用，得到 %+v", app)
	}
}

func TestAppInfoWithoutApplicationReturnsPlatformInfo(t *testing.T) {
	apps := &stubAppRepo{all: []*repository.Application{
		{InstitutionID: 1, Code: "new-app", Name: "New App", SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodMagicLink}}},
	}}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{
		repository.BuiltInInstitution: {ID: 1, Name: repository.BuiltInInstitution, DisplayName: "Polaris"},
	}}
	svc := newPolicyService(apps, orgs, repository.BuiltInInstitution)

	info, err := svc.AppInfo(context.Background(), repository.BuiltInInstitution, "")
	if err != nil {
		t.Fatalf("AppInfo: %v", err)
	}
	if info.Code != "" || info.Name != "" {
		t.Fatalf("platform info must not inherit an application: %+v", info)
	}
	if info.Institution != repository.BuiltInInstitution || info.OrgDisplayName != "Polaris" {
		t.Fatalf("organization info = %+v", info)
	}
	if len(info.SigninMethods) != 1 || info.SigninMethods[0].Name != repository.SigninMethodPassword {
		t.Fatalf("platform signin methods = %+v", info.SigninMethods)
	}
}

// 组织首个应用作为兜底。
func TestResolveApplicationOptionalFallsBackToFirst(t *testing.T) {
	apps := &stubAppRepo{all: []*repository.Application{{InstitutionID: 1, Code: "app-a", Name: "App A"}}}
	svc := newPolicyService(apps, &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme"}}}, "built-in")
	app, err := svc.resolveApplicationOptional(context.Background(), "acme", "")
	if err != nil || app == nil || app.Code != "app-a" {
		t.Fatalf("期望兜底到 app-a，得到 app=%+v err=%v", app, err)
	}
}

// 组织默认应用存在时优先于首个应用。
func TestResolveApplicationOptionalPrefersDefault(t *testing.T) {
	apps := &stubAppRepo{
		byName: map[string]*repository.Application{"1/app-default": {InstitutionID: 1, Code: "app-default", Name: "App Default"}},
		all:    []*repository.Application{{InstitutionID: 1, Code: "app-first", Name: "App First"}},
	}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme", DefaultApplication: "app-default"}}}
	svc := newPolicyService(apps, orgs, "built-in")
	app, err := svc.resolveApplicationOptional(context.Background(), "acme", "")
	if err != nil || app == nil || app.Code != "app-default" {
		t.Fatalf("期望默认应用 app-default，得到 app=%+v err=%v", app, err)
	}
}

// 显式指定但不存在的应用仍应报错（应用驱动场景需要明确失败）。
func TestResolveApplicationOptionalExplicitMissing(t *testing.T) {
	svc := newPolicyService(&stubAppRepo{}, &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme"}}}, "built-in")
	app, err := svc.resolveApplicationOptional(context.Background(), "acme", "ghost")
	if err == nil {
		t.Fatalf("期望错误，得到 app=%+v", app)
	}
}

// 回归：登录页公开信息必须携带组织的展示名与描述（供左上角品牌与欢迎语使用），且不含敏感字段。
func TestBuildAppInfoCarriesOrgDisplayNameAndDescription(t *testing.T) {
	apps := &stubAppRepo{
		byName: map[string]*repository.Application{"1/app-a": {InstitutionID: 1, Code: "app-a", Name: "App A"}},
	}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{
		"acme": {ID: 1, Name: "acme", DisplayName: "示例组织", Description: "统一身份入口", Logo: "/logo.png", Favicon: "/fav.ico", PasswordSalt: "secret"},
	}}
	svc := newPolicyService(apps, orgs, "built-in")
	info := svc.buildAppInfo(context.Background(), &repository.Application{InstitutionID: 1, Code: "app-a", Name: "App A"})

	if info.OrgDisplayName != "示例组织" {
		t.Fatalf("组织展示名未下发: %q", info.OrgDisplayName)
	}
	if info.OrgDescription != "统一身份入口" {
		t.Fatalf("组织描述未下发: %q", info.OrgDescription)
	}
	if info.OrgLogo != "/logo.png" || info.OrgFavicon != "/fav.ico" {
		t.Fatalf("组织 Logo/Favicon 未下发: %q %q", info.OrgLogo, info.OrgFavicon)
	}
	// 组织展示名缺失时回退到组织标识
	orgs2 := &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {Name: "acme"}}}
	svc2 := newPolicyService(apps, orgs2, "built-in")
	if got := svc2.buildAppInfo(context.Background(), &repository.Application{InstitutionID: 1}).OrgDisplayName; got != "acme" {
		t.Fatalf("展示名缺失时应回退组织标识，得到 %q", got)
	}
}

// 配置即生效：signin_methods 为空时仅密码可用（兼容未回填的历史数据）。
func TestHasSigninMethodFallsBackToPassword(t *testing.T) {
	app := &repository.Application{}
	if !app.HasSigninMethod(repository.SigninMethodPassword) {
		t.Fatal("空登录方式应兜底为仅密码可用")
	}
	if app.HasSigninMethod(repository.SigninMethodMagicLink) {
		t.Fatal("空登录方式不应放行魔法链接")
	}
}

// 能力降级：组织缺邮件提供商时，邮箱验证码与魔法链接不下发给登录页。
func TestAvailableSigninMethodsFiltersByOrgCapability(t *testing.T) {
	app := &repository.Application{
		SigninMethods: []repository.SigninMethod{
			{Name: repository.SigninMethodPassword},
			{Name: repository.SigninMethodEmailCode},
			{Name: repository.SigninMethodMagicLink},
			{Name: repository.SigninMethodSMSCode},
			{Name: repository.SigninMethodProvider},
			{Name: repository.SigninMethodGuest},
		},
	}
	// 无任何提供商：只剩密码（访客未实现运行时，会被过滤）
	got := availableSigninMethods(app, nil, nil)
	if len(got) != 1 || got[0].Name != repository.SigninMethodPassword {
		t.Fatalf("缺提供商时应仅保留密码，实际 %+v", got)
	}
	// 有邮件与短信提供商：邮箱验证码、短信验证码与魔法链接恢复
	got = availableSigninMethods(app, []string{"mail-1"}, []string{"sms-1"})
	if len(got) != 4 {
		t.Fatalf("具备邮件/短信能力时应保留 4 种（第三方仍需应用配置提供方），实际 %+v", got)
	}
	// 应用配置了提供方后第三方登录可用
	app.Providers = []repository.ProviderItem{{Name: "dingtalk", CanSignIn: true}}
	got = availableSigninMethods(app, []string{"mail-1"}, []string{"sms-1"})
	if len(got) != 5 {
		t.Fatalf("配置提供方后应保留 5 种，实际 %+v", got)
	}
	// 访客登录尚未实现运行时：即便被写入配置也不会下发
	app.SigninMethods = append(app.SigninMethods, repository.SigninMethod{Name: repository.SigninMethodGuest})
	got = availableSigninMethods(app, []string{"mail-1"}, []string{"sms-1"})
	for _, m := range got {
		if m.Name == repository.SigninMethodGuest {
			t.Fatalf("未实现的登录方式不应下发: %+v", got)
		}
	}
}

// 空 signin_methods 的历史应用：下发与判定都要兜底为"仅密码"，两边语义必须一致。
func TestAvailableSigninMethodsFallsBackToPasswordWhenEmpty(t *testing.T) {
	app := &repository.Application{}
	got := availableSigninMethods(app, nil, nil)
	if len(got) != 1 || got[0].Name != repository.SigninMethodPassword {
		t.Fatalf("空登录方式应兜底为仅密码，实际 %+v", got)
	}
	if !app.HasSigninMethod(repository.SigninMethodPassword) {
		t.Fatal("后端判定也应放行密码登录（与下发结果一致）")
	}
}

// 回归：max_sessions = 0 表示"不限制"，不得在保存时被改写为 1（曾导致界面上填 0 实际变成独占）。
func TestNormalizeApplicationKeepsZeroMaxSessions(t *testing.T) {
	app := &repository.Application{
		Code:        "app",
		MaxSessions: 0,
		GrantTypes:  []string{"client_credentials"}, // 避免触发"授权码必须有回调地址"的跨字段校验
	}
	if err := normalizeApplication(app); err != nil {
		t.Fatalf("归一化失败: %v", err)
	}
	if app.MaxSessions != 0 {
		t.Fatalf("max_sessions = 0 应保留为不限制，实际 %d", app.MaxSessions)
	}
}

// 登录方式校验：未实现的方式、以及"第三方登录却没有提供方"都必须被拒绝。
func TestValidateApplicationSigninMethods(t *testing.T) {
	base := func() *repository.Application {
		return &repository.Application{
			Code:          "app",
			SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodPassword}},
		}
	}
	if err := validateApplication(base()); err != nil {
		t.Fatalf("单密码方式应通过: %v", err)
	}

	bad := base()
	bad.SigninMethods = append(bad.SigninMethods, repository.SigninMethod{Name: repository.SigninMethodGuest})
	if err := validateApplication(bad); err == nil {
		t.Fatal("未实现运行时的 Guest 方式应被拒绝")
	}

	noProvider := base()
	noProvider.SigninMethods = append(noProvider.SigninMethods,
		repository.SigninMethod{Name: repository.SigninMethodProvider})
	if err := validateApplication(noProvider); err == nil {
		t.Fatal("第三方登录缺少提供方应被拒绝")
	}

	withProvider := base()
	withProvider.SigninMethods = append(withProvider.SigninMethods,
		repository.SigninMethod{Name: repository.SigninMethodProvider})
	withProvider.Providers = []repository.ProviderItem{{Name: "dingtalk", CanSignIn: true}}
	if err := validateApplication(withProvider); err != nil {
		t.Fatalf("带提供方的第三方登录应通过: %v", err)
	}
}

// 回归：app-info 的"组织标识"字段名必须是 `institution`（与请求契约、前端 `AppInfo.institution` 一致）。
//
// 曾写成驼峰 `institutionID`：应用驱动登录页虽能渲染组织名/Logo（org_display_name / org_logo），
// 但前端读 `info.institution` 永远是 undefined，于是登录、发码、发起 SSO 全都不带组织——
// 表现为"密码登录报账号或密码错误、点钉钉登录直接失败"。
func TestAppInfoInstitutionFieldNameMatchesContract(t *testing.T) {
	raw, err := json.Marshal(AppInfo{Institution: "测试", Code: "net-bird", Name: "Net Bird"})
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"institution":"测试"`) {
		t.Fatalf("app-info 必须以蛇形 institution 下发组织标识: %s", body)
	}
	if strings.Contains(body, "institutionID") {
		t.Fatalf("不得出现驼峰 institutionID（前端读不到）: %s", body)
	}
}

// access_token 映射：左侧 claim、右侧系统字段；部门保持数组语义。
func TestTokenExtraClaimMapping(t *testing.T) {
	user := &repository.User{
		ID: 7, UID: "uid-7", InstitutionID: 1, Name: "zhangsan",
		DisplayName: "张三", Email: "zhangsan@example.com", IsAdmin: true,
	}
	svc := &OIDCService{
		userDepts: &tokenUserDeptRepo{ids: []int64{2, 3}},
		depts: &tokenDeptRepo{items: []*repository.Department{
			{ID: 1, Name: "built-in", DisplayName: "Polaris", ParentIDChain: []int64{}},
			{ID: 2, Name: "fe", DisplayName: "前端组", ParentIDChain: []int64{1}},
			{ID: 3, Name: "tech", DisplayName: "技术部", ParentIDChain: []int64{1}},
		}},
	}
	got, err := svc.tokenExtra(context.Background(), &repository.Application{TokenFormat: TokenFormatCustom, TokenAttributes: []repository.TokenAttribute{
		{Name: "username", Category: TokenAttributeCategoryExisting, Value: "username", Type: TokenAttributeTypeString},
		{Name: "display_name", Category: TokenAttributeCategoryExisting, Value: "display_name", Type: TokenAttributeTypeString},
		{Name: "departments", Category: TokenAttributeCategoryExisting, Value: "department_ids", Type: TokenAttributeTypeArray},
		{Name: "department_paths", Category: TokenAttributeCategoryExisting, Value: "departments", Type: TokenAttributeTypeArray},
		{Name: "admin", Category: TokenAttributeCategoryExisting, Value: "is_admin", Type: TokenAttributeTypeString},
		{Name: "env", Category: TokenAttributeCategoryStatic, Value: "prod, test", Type: TokenAttributeTypeArray},
		{Name: "issuer_label", Category: TokenAttributeCategoryStatic, Value: "polaris", Type: TokenAttributeTypeString},
	}}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}

	if got["username"] != "zhangsan" || got["display_name"] != "张三" {
		t.Fatalf("标量字段映射错误: %+v", got)
	}
	if !reflect.DeepEqual(got["departments"], []int64{2, 3}) {
		t.Fatalf("departments 应为数组，实际 %#v", got["departments"])
	}
	if !reflect.DeepEqual(got["department_paths"], []string{"Polaris / 前端组", "Polaris / 技术部"}) {
		t.Fatalf("department_paths 应为数组，实际 %#v", got["department_paths"])
	}
	if got["admin"] != true {
		t.Fatalf("admin = %#v, want true", got["admin"])
	}
	if !reflect.DeepEqual(got["env"], []string{"prod", "test"}) {
		t.Fatalf("静态数组 = %#v, want [prod test]", got["env"])
	}
	if got["issuer_label"] != "polaris" {
		t.Fatalf("静态字符串 = %#v", got["issuer_label"])
	}
}

// 空配置默认只下发 username 与 institution（组织主键）。
func TestTokenExtraDefaults(t *testing.T) {
	user := &repository.User{InstitutionID: 9, Name: "lisi"}
	svc := &OIDCService{}
	got, err := svc.tokenExtra(context.Background(), &repository.Application{TokenFormat: TokenFormatCustom}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}
	want := map[string]interface{}{"username": "lisi", "institution": int64(9)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("默认字段应为 %+v，实际 %+v", want, got)
	}
}

func TestValidateApplicationTokenFormat(t *testing.T) {
	base := func(format string) *repository.Application {
		return &repository.Application{
			TokenFormat:   format,
			SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodPassword}},
		}
	}

	if err := validateApplication(base(TokenFormatStandard)); err != nil {
		t.Fatalf("standard 模式应通过: %v", err)
	}
	legacy := base("jwt")
	if err := validateApplication(legacy); err != nil {
		t.Fatalf("历史 jwt 模式应兼容: %v", err)
	}
	if legacy.TokenFormat != TokenFormatCustom {
		t.Fatalf("jwt 应归一化为 custom，实际 %q", legacy.TokenFormat)
	}
	if err := validateApplication(base("opaque")); err == nil {
		t.Fatal("未知 token_format 应被拒绝")
	}
}

// 未知字段必须在落库前拒绝，而不是静默丢弃。
func TestValidateApplicationRejectsUnknownTokenField(t *testing.T) {
	app := &repository.Application{
		SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodPassword}},
		TokenAttributes: []repository.TokenAttribute{
			{Name: "username", Category: TokenAttributeCategoryExisting, Value: "username"},
			{Name: "bad", Category: TokenAttributeCategoryExisting, Value: "department_names_typo"},
		},
	}
	err := validateApplication(app)
	if err == nil || !bizerrors.Is(err, bizerrors.CodeInvalidParams) {
		t.Fatalf("未知字段应返回 INVALID_PARAMS，实际 %v", err)
	}
	app.TokenAttributes = []repository.TokenAttribute{
		{Name: " department_ids ", Category: TokenAttributeCategoryExisting, Value: " department_ids "},
		{Name: "username", Category: TokenAttributeCategoryExisting, Value: "username"},
	}
	if err := validateApplication(app); err != nil {
		t.Fatalf("合法字段不应报错: %v", err)
	}
	if app.TokenAttributes[0].Name != "department_ids" || app.TokenAttributes[0].Type != TokenAttributeTypeArray {
		t.Fatalf("应去空白并按字段推断数组类型，实际 %+v", app.TokenAttributes[0])
	}
	for _, bad := range [][]repository.TokenAttribute{
		{{Name: "bad claim", Category: TokenAttributeCategoryExisting, Value: "username"}},
		{
			{Name: "x", Category: TokenAttributeCategoryExisting, Value: "username"},
			{Name: "x", Category: TokenAttributeCategoryExisting, Value: "email"},
		},
		{{Name: "static_empty", Category: TokenAttributeCategoryStatic, Value: ""}},
		{{Name: "bad_category", Category: "unknown", Value: "username"}},
	} {
		app.TokenAttributes = bad
		if err := validateApplication(app); err == nil {
			t.Fatalf("非法映射 %+v 应被拒绝", bad)
		}
	}
}

// 登录页预检：未授权用户在自动续走 authorize 之前就能得到明确结论。
func TestAppAccessByClientID(t *testing.T) {
	app := &repository.Application{
		ID: 7, InstitutionID: 1, Code: "vpn", Name: "VPN", ClientID: "client-vpn",
		State: "active", AccessMode: repository.AccessModeRestricted,
	}
	apps := &stubAppRepo{all: []*repository.Application{app}}
	// 授权部门 = 技术部（父）；zhangsan 的主部门是前端组（子），应命中。
	accessSvc, _ := accessFixture([]repository.ApplicationSubject{
		departmentSubject(1),
	})
	svc := &AuthService{apps: apps, users: accessSvc.users, access: accessSvc}

	allowed, err := svc.AppAccessByClientID(context.Background(), "client-vpn", "zhangsan")
	if err != nil || allowed == nil || !allowed.Allowed {
		t.Fatalf("授权部门成员应放行，实际 %+v err=%v", allowed, err)
	}
	denied, err := svc.AppAccessByClientID(context.Background(), "client-vpn", "lisi")
	if err != nil || denied == nil || denied.Allowed || denied.Reason != "app_access_denied" {
		t.Fatalf("未授权用户应返回 app_access_denied，实际 %+v err=%v", denied, err)
	}
}
