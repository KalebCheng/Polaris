package auth

import "testing"

func TestAuthContextEnforce(t *testing.T) {
	ctx := &AuthContext{
		UserID: 1,
		Name:   "alice",
		Permissions: []PermissionRule{
			{Resource: "user", Action: "read", Effect: "Allow"},
			{Resource: "application", Action: "*", Effect: "Allow"},
			{Resource: "role", Action: "write", Effect: "Deny"},
		},
	}

	cases := []struct {
		resource, action string
		want             bool
	}{
		{"user", "read", true},
		{"user", "delete", false},
		{"application", "delete", true},
		{"application", "read", true},
		{"role", "write", false}, // Deny 优先
		{"role", "read", false},
	}
	for _, tc := range cases {
		if got := ctx.Enforce(tc.resource, tc.action); got != tc.want {
			t.Errorf("Enforce(%s,%s) = %v, 期望 %v", tc.resource, tc.action, got, tc.want)
		}
	}
}

func TestAuthContextEnforceAdminBypass(t *testing.T) {
	ctx := &AuthContext{UserID: 1, IsAdmin: true}
	if !ctx.Enforce("anything", "at-all") {
		t.Fatal("管理员应直通任意资源/动作")
	}
}

func TestAuthContextEnforceWildcardAndDenyPriority(t *testing.T) {
	ctx := &AuthContext{
		UserID: 1,
		Permissions: []PermissionRule{
			{Resource: "*", Action: "*", Effect: "Allow"},
			{Resource: "secret", Action: "read", Effect: "Deny"},
		},
	}
	if !ctx.Enforce("user", "read") {
		t.Fatal("通配 Allow 应生效")
	}
	if ctx.Enforce("secret", "read") {
		t.Fatal("精确 Deny 应覆盖通配 Allow")
	}
}

func TestAuthContextNilSafety(t *testing.T) {
	var ctx *AuthContext
	if ctx.IsAuthenticated() {
		t.Fatal("nil 上下文不应视为已认证")
	}
	if ctx.Enforce("user", "read") {
		t.Fatal("nil 上下文不应放行")
	}
}

func TestHasRole(t *testing.T) {
	ctx := &AuthContext{UserID: 1, Roles: []string{"admin", "auditor"}}
	if !ctx.HasRole("auditor") {
		t.Fatal("应识别 auditor 角色")
	}
	if ctx.HasRole("user") {
		t.Fatal("不应识别不存在的角色")
	}
}
