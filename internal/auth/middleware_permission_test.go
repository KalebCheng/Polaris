package auth

import "testing"

// 回归：跨资源聚合端点（身份连接目录）只要任一资源有读权限即放行，全无则拒绝。
func TestAnyPermissionAllowed(t *testing.T) {
	read := func(resource string) PermissionRule {
		return PermissionRule{Resource: resource, Action: "read", Effect: "Allow"}
	}
	cases := []struct {
		name   string
		ctx    *AuthContext
		action string
		want   bool
	}{
		{name: "仅 syncer 读权限", ctx: &AuthContext{Name: "u", Permissions: []PermissionRule{read("syncer")}}, action: "read", want: true},
		{name: "仅 connector 读权限", ctx: &AuthContext{Name: "u", Permissions: []PermissionRule{read("connector")}}, action: "read", want: true},
		{name: "两者皆无", ctx: &AuthContext{Name: "u", Permissions: []PermissionRule{read("user")}}, action: "read", want: false},
		{name: "动作不匹配", ctx: &AuthContext{Name: "u", Permissions: []PermissionRule{read("syncer")}}, action: "update", want: false},
		{name: "管理员直通", ctx: &AuthContext{Name: "admin", IsAdmin: true}, action: "read", want: true},
		{
			name: "Deny 优先于 Allow（同资源）",
			ctx: &AuthContext{Name: "u", Permissions: []PermissionRule{
				read("connector"),
				{Resource: "connector", Action: "read", Effect: "Deny"},
			}},
			action: "read", want: false,
		},
	}
	for _, tc := range cases {
		if got := anyPermissionAllowed(tc.ctx, tc.action, []string{"syncer", "connector"}); got != tc.want {
			t.Errorf("%s: 期望 %v，实际 %v", tc.name, tc.want, got)
		}
	}
	if anyPermissionAllowed(&AuthContext{Name: "u"}, "read", nil) {
		t.Error("无资源清单时应拒绝")
	}
}
