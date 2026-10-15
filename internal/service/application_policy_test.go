package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"polaris/internal/config"
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
