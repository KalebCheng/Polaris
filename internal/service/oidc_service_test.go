package service

import (
	"testing"

	"polaris/internal/repository"
)

func TestAppAllowsRedirect(t *testing.T) {
	app := &repository.Application{
		RedirectURIs: []string{
			"http://localhost:5173/oauth/callback",
			"https://app.example.com/*",
		},
	}
	cases := []struct {
		uri  string
		want bool
	}{
		{"http://localhost:5173/oauth/callback", true},
		{"https://app.example.com/oauth/callback", true},
		{"https://app.example.com/anything/deep", true},
		{"http://localhost:5173/evil", false},
		{"https://evil.example.com/callback", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := appAllowsRedirect(app, tc.uri); got != tc.want {
			t.Errorf("appAllowsRedirect(%q) = %v, 期望 %v", tc.uri, got, tc.want)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "openid profile email"},
		{"profile", "openid profile"},
		{"openid email", "openid email"},
	}
	for _, tc := range cases {
		if got := normalizeScope(tc.in); got != tc.want {
			t.Errorf("normalizeScope(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

// TestShouldIssueRefresh 验证 refresh_token 仅在 scope 含 offline_access 且应用允许刷新时下发。
func TestShouldIssueRefresh(t *testing.T) {
	app := &repository.Application{RefreshExpireInHours: 24}
	noRefresh := &repository.Application{RefreshExpireInHours: 0}

	cases := []struct {
		name  string
		app   *repository.Application
		scope string
		want  bool
	}{
		{"含 offline_access 且允许刷新", app, "openid offline_access", true},
		{"无 offline_access", app, "openid profile email", false},
		{"子串不应误判", app, "openid offline_access_x", false},
		{"应用不允许刷新", noRefresh, "openid offline_access", false},
		{"nil 应用", nil, "openid offline_access", false},
	}
	for _, tc := range cases {
		if got := shouldIssueRefresh(tc.app, tc.scope); got != tc.want {
			t.Errorf("%s: shouldIssueRefresh = %v, 期望 %v", tc.name, got, tc.want)
		}
	}
}

// TestHasScope 验证按空格切分的 scope 匹配（避免子串误判）。
func TestHasScope(t *testing.T) {
	scope := normalizeScope("openid profile email")
	if !hasScope(scope, "openid") || !hasScope(scope, "profile") || !hasScope(scope, "email") {
		t.Fatal("应匹配已声明的 scope")
	}
	if hasScope(scope, "phone") || hasScope(scope, "offline_access") {
		t.Fatal("不应匹配未声明的 scope")
	}
	if hasScope("openid offline_access_x", "offline_access") {
		t.Fatal("不应把 offline_access_x 误判为 offline_access")
	}
}

func TestVerifyPKCE(t *testing.T) {
	// RFC 7636 附录 B 的示例向量
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if !verifyPKCE(challenge, "S256", verifier) {
		t.Fatal("S256 校验应通过")
	}
	if verifyPKCE(challenge, "S256", "wrong-verifier") {
		t.Fatal("错误的 code_verifier 不应通过")
	}
	if !verifyPKCE("plain-value", "plain", "plain-value") {
		t.Fatal("plain 方式应逐字比对")
	}
	if !verifyPKCE("plain-value", "", "plain-value") {
		t.Fatal("method 为空时按 plain 处理")
	}
	if verifyPKCE(challenge, "S256", "") {
		t.Fatal("空 verifier 不应通过")
	}
	if verifyPKCE(challenge, "unknown-method", verifier) {
		t.Fatal("未知 method 不应通过")
	}
}

func TestSanitizeFrom(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/users", "/users"},
		{"", ""},
		{"https://evil.com", ""},
		{"//evil.com", ""},
		{"/login", ""},
		{"/login?from=x", ""},
		{"users", ""},
	}
	for _, tc := range cases {
		if got := sanitizeFrom(tc.in); got != tc.want {
			t.Errorf("sanitizeFrom(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestHashTokenStable(t *testing.T) {
	a := hashToken("token-value")
	b := hashToken("token-value")
	if a != b {
		t.Fatal("同一令牌的哈希应稳定")
	}
	if a == hashToken("token-value-2") {
		t.Fatal("不同令牌哈希不应相同")
	}
	if a == "token-value" {
		t.Fatal("哈希不应等于明文")
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"zhangsan", "zhangsan"},
		{"zhang san", "zhang-san"},
		{"a@b.com", "a-b.com"},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeName(tc.in)
		if tc.in == "" {
			if got == "" {
				t.Errorf("空输入应回退为随机用户名，实际为空")
			}
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeName(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestDomainAllowed(t *testing.T) {
	if !domainAllowed("a@example.com", []string{"example.com"}) {
		t.Fatal("域名应匹配")
	}
	if !domainAllowed("a@EXAMPLE.com", []string{"example.com"}) {
		t.Fatal("域名匹配应忽略大小写")
	}
	if domainAllowed("a@other.com", []string{"example.com"}) {
		t.Fatal("不匹配的域名不应通过")
	}
	if domainAllowed("no-at-sign", []string{"example.com"}) {
		t.Fatal("非法邮箱不应通过")
	}
}

func TestPage(t *testing.T) {
	cases := []struct {
		page, size            int
		wantOffset, wantLimit int
	}{
		{1, 20, 0, 20},
		{3, 10, 20, 10},
		{0, 20, 0, 20},
		{1, 0, 0, 20},
		{1, 1000, 0, 20},
	}
	for _, tc := range cases {
		offset, limit := Page(tc.page, tc.size)
		if offset != tc.wantOffset || limit != tc.wantLimit {
			t.Errorf("Page(%d,%d) = (%d,%d), 期望 (%d,%d)", tc.page, tc.size, offset, limit, tc.wantOffset, tc.wantLimit)
		}
	}
}
