package service

import (
	"context"
	"testing"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

func providerInfo(names ...string) []auth.OAuthProviderInfo {
	out := make([]auth.OAuthProviderInfo, 0, len(names))
	for _, name := range names {
		out = append(out, auth.OAuthProviderInfo{Name: name, Type: "dingtalk"})
	}
	return out
}

func providerNames(items []auth.OAuthProviderInfo) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

// 回归（真实缺陷）：应用**没有配置**任何提供方时，登录页却出现组织级启用的第三方入口
// （"net-bird 没配钉钉，登录页却有钉钉按钮"）。
//
// 空的语义只能是"一个都不要"：写入侧 validateApplication 要求"启用第三方登录必须带具体提供方"、
// 下发侧 availableSigninMethods 在 providers 为空时不下发 Provider 登录方式——
// 读取侧必须与它们一致。
func TestAppSigninProvidersEmptyWhitelistMeansNone(t *testing.T) {
	all := providerInfo("dingtalk", "wechat")
	app := &repository.Application{
		Code:          "net-bird",
		SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodPassword}},
		Providers:     nil,
	}
	got := appSigninProviders(all, app)
	if len(got) != 0 {
		t.Fatalf("未配置提供方的应用不应有任何第三方入口，实际 %v", providerNames(got))
	}
}

// 白名单生效：只保留 canSignIn=true 的项（canSignUp-only 的项不出现在登录页）。
func TestAppSigninProvidersFiltersWhitelist(t *testing.T) {
	all := providerInfo("dingtalk", "wechat", "feishu")
	app := &repository.Application{
		SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodProvider}},
		Providers: []repository.ProviderItem{
			{Name: "dingtalk", CanSignIn: true},
			{Name: "wechat", CanSignIn: false},
		},
	}
	got := providerNames(appSigninProviders(all, app))
	if len(got) != 1 || got[0] != "dingtalk" {
		t.Fatalf("白名单过滤结果错误：%v", got)
	}
}

// 应用把「第三方登录」从登录方式里去掉后，即使残留了提供方配置也不得下发入口。
func TestAppSigninProvidersRequiresProviderSigninMethod(t *testing.T) {
	all := providerInfo("dingtalk")
	app := &repository.Application{
		SigninMethods: []repository.SigninMethod{{Name: repository.SigninMethodPassword}},
		Providers:     []repository.ProviderItem{{Name: "dingtalk", CanSignIn: true}},
	}
	if got := appSigninProviders(all, app); len(got) != 0 {
		t.Fatalf("未启用 Provider 登录方式时不应下发第三方入口，实际 %v", providerNames(got))
	}
}

// 没有应用上下文（组织级登录页）时保持原行为：列出组织内全部启用的提供方。
func TestAppSigninProvidersWithoutApplicationReturnsAll(t *testing.T) {
	all := providerInfo("dingtalk", "wechat")
	got := providerNames(appSigninProviders(all, nil))
	if len(got) != 2 {
		t.Fatalf("组织级登录页应列出全部提供方，实际 %v", got)
	}
}

// 回归：指定了**不存在**的应用标识时，不得返回任何第三方入口。
// 曾经的实现解析失败就退化成"列出组织内全部提供方"（fail-open）：
// 传一个不存在的 application 就能拿到应用白名单之外的全部登录入口。
func TestListAppProvidersFailsClosedForUnknownApplication(t *testing.T) {
	apps := &stubAppRepo{byName: map[string]*repository.Application{}}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{"acme": {ID: 1, Name: "acme"}}}
	svc := &AuthService{apps: apps, orgs: orgs, providerRuntime: nil}

	got := svc.ListAppProviders(context.Background(), "acme", "not-exist-app")
	if len(got) != 0 {
		t.Fatalf("未知应用不应返回任何第三方入口，实际 %d 条", len(got))
	}
}
