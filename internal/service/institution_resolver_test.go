package service

import "testing"

/**
 * 组织解析策略：管理端登录页不再强制选择组织。
 * 优先级：显式指定 → 平台仅一个组织 → 账号唯一命中（优先于默认组织）→ 配置的默认组织；
 * 命中多个（同名账号存在于多个组织）时必须报错要求显式选择，不能猜。
 */
func TestChooseInstitution(t *testing.T) {
	cases := []struct {
		name       string
		requested  string
		orgTotal   int64
		defaultOrg string
		matches    int
		firstMatch string
		want       string
		wantOK     bool
	}{
		{"显式指定优先", "team-a", 5, "built-in", 3, "team-b", "team-a", true},
		{"单组织自动定位", "", 1, "", 1, "only-org", "only-org", true},
		{"多组织但有默认", "", 3, "built-in", 0, "", "built-in", true},
		{"多组织无默认但账号唯一命中", "", 3, "", 1, "team-c", "team-c", true},
		{"唯一命中优先于默认组织", "", 3, "built-in", 1, "team-c", "team-c", true},
		{"多组织且账号歧义", "", 3, "", 2, "team-a", "", false},
		{"账号不存在", "", 3, "", 0, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := chooseInstitution(tc.requested, tc.orgTotal, tc.defaultOrg, tc.matches, tc.firstMatch)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("chooseInstitution = (%q, %v), 期望 (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// normalizeHost：去掉端口、小写化，供域名解析租户使用（ADR D3）。
func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"ACME.Example.com:8100": "acme.example.com",
		"acme.example.com":      "acme.example.com",
		"ACME.EXAMPLE.COM":      "acme.example.com",
		"  acme.example.com  ":  "acme.example.com",
		"localhost:5173":        "localhost",
		"[::1]:8100":            "::1",
		"":                      "",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
