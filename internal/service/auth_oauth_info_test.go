package service

import (
	"testing"

	"polaris/internal/config"
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
