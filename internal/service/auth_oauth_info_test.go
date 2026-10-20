package service

import (
	"testing"

	"polaris/internal/config"
	"polaris/internal/repository"
)

// 回归：控制台「开发信息」展示的地址必须与登录链路**同源**——
// 曾出现"界面上写的回调地址与实际使用的不一致"这类隐蔽故障。
func TestOAuthPlatformInfo(t *testing.T) {
	cases := []struct {
		name         string
		cfg          config.OAuthConfig
		wantCallback string
		wantHomepage string
	}{
		{
			name:         "默认：门户地址 + /sso/callback（前端自己的命名空间）",
			cfg:          config.OAuthConfig{BaseURL: "https://id.example.com/"},
			wantCallback: "https://id.example.com/sso/callback",
			wantHomepage: "https://id.example.com",
		},
		{
			name:         "显式配置 redirect_uri 时以它为准",
			cfg:          config.OAuthConfig{BaseURL: "https://id.example.com", RedirectURI: "https://sso.example.com/cb"},
			wantCallback: "https://sso.example.com/cb",
			wantHomepage: "https://id.example.com",
		},
		{
			name:         "相对路径按 base_url 补成绝对地址（IdP 不接受相对 redirect_uri）",
			cfg:          config.OAuthConfig{BaseURL: "https://id.example.com", RedirectURI: "/custom/callback"},
			wantCallback: "https://id.example.com/custom/callback",
			wantHomepage: "https://id.example.com",
		},
	}
	for _, tc := range cases {
		svc := &AuthService{oauthCfg: tc.cfg}
		callback, homepage := svc.OAuthPlatformInfo()
		if callback != tc.wantCallback {
			t.Errorf("%s: 回调地址错误 got %q want %q", tc.name, callback, tc.wantCallback)
		}
		if homepage != tc.wantHomepage {
			t.Errorf("%s: 门户地址错误 got %q want %q", tc.name, homepage, tc.wantHomepage)
		}
	}
}

// 提供商级「回调地址覆盖」同样支持站内相对路径，并与开发信息展示保持一致。
func TestResolveRedirectURIProviderOverride(t *testing.T) {
	svc := &AuthService{oauthCfg: config.OAuthConfig{BaseURL: "https://id.example.com"}}
	item := &repository.Provider{RedirectURI: "/custom/callback"}
	if got := svc.resolveRedirectURI(item, ""); got != "https://id.example.com/custom/callback" {
		t.Fatalf("提供商覆盖应补成绝对地址，实际 %q", got)
	}
	if got := svc.resolveRedirectURI(item, "/override/callback"); got != "https://id.example.com/override/callback" {
		t.Fatalf("请求覆盖应补成绝对地址，实际 %q", got)
	}
}
