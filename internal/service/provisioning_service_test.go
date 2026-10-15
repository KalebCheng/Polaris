package service

import (
	"testing"

	"polaris/internal/repository"
)

// 回归：建号触发策略缺省应为 onCreate。
func TestPolicyCreateWhenDefaults(t *testing.T) {
	if got := policyCreateWhen(nil); got != CreateWhenOnCreate {
		t.Fatalf("nil 策略应缺省 onCreate，实际 %q", got)
	}
	if got := policyCreateWhen(&repository.AccountStrategy{}); got != CreateWhenOnCreate {
		t.Fatalf("空 policy 应缺省 onCreate，实际 %q", got)
	}
	s := &repository.AccountStrategy{Policy: map[string]string{"createWhen": CreateWhenJIT}}
	if got := policyCreateWhen(s); got != CreateWhenJIT {
		t.Fatalf("应读取到 jit，实际 %q", got)
	}
}

// 回归：回收触发策略缺省应为 disable。
func TestPolicyRevokeWhenDefaults(t *testing.T) {
	if got := policyRevokeWhen(nil); got != RevokeWhenDisable {
		t.Fatalf("nil 策略应缺省 disable，实际 %q", got)
	}
	s := &repository.AccountStrategy{Policy: map[string]string{"revokeWhen": RevokeWhenOffboard}}
	if got := policyRevokeWhen(s); got != RevokeWhenOffboard {
		t.Fatalf("应读取到 offboard，实际 %q", got)
	}
}

// 回归：角色授予触发只针对"本次新增"的用户，避免重复派发。
func TestNewlyAdded(t *testing.T) {
	got := newlyAdded([]string{"a", "b"}, []string{"b", "c", "c", "", "d"})
	want := []string{"c", "d"}
	if len(got) != len(want) {
		t.Fatalf("新增用户集合错误: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("新增用户集合错误: %v", got)
		}
	}
}

// 回归：本地属性映射应能取到常用用户字段。
func TestLocalAttribute(t *testing.T) {
	u := &repository.User{Name: "alice", DisplayName: "Alice", Email: "a@example.com", UID: "u-1"}
	cases := map[string]string{
		"name": "alice", "username": "alice", "displayName": "Alice",
		"email": "a@example.com", "uid": "u-1",
	}
	for field, want := range cases {
		got, ok := localAttribute(u, field)
		if !ok || got != want {
			t.Fatalf("字段 %s 应映射为 %q，实际 %q (ok=%v)", field, want, got, ok)
		}
	}
	if _, ok := localAttribute(u, "unknown"); ok {
		t.Fatal("未知字段应返回 ok=false")
	}
}

// 回归：供给任务的单飞键必须与 app_accounts 的幂等键一致（机构×应用×用户），
// 否则同一账号会被重复排队、下游重复建号。
func TestProvisionUniqueKey(t *testing.T) {
	key := ProvisionUniqueKey(7, "salesforce", "alice")
	if key != "provision:7:salesforce:alice" {
		t.Fatalf("单飞键格式错误: %s", key)
	}
	if ProvisionUniqueKey(7, "salesforce", "alice") == ProvisionUniqueKey(7, "salesforce", "bob") {
		t.Fatal("不同用户的单飞键不应相同")
	}
	if ProvisionUniqueKey(7, "a", "alice") == ProvisionUniqueKey(8, "a", "alice") {
		t.Fatal("不同组织的单飞键不应相同")
	}
}

// 回归：任务载荷解析（asyncd 传入的 JSON 数字是 float64）
func TestProvisionTaskFromPayload(t *testing.T) {
	task := ProvisionTaskFromPayload(map[string]any{
		"institution_id": float64(12), "application": "salesforce", "user": "alice", "action": "create",
	})
	if task.InstitutionID != 12 || task.Application != "salesforce" || task.User != "alice" || task.Action != "create" {
		t.Fatalf("载荷解析错误: %+v", task)
	}
	empty := ProvisionTaskFromPayload(nil)
	if empty.InstitutionID != 0 || empty.Application != "" {
		t.Fatalf("空载荷应得到零值: %+v", empty)
	}
}

// 回归：下游不支持口令时（如钉钉），不得生成也不得保存下游口令。
// 否则库里会存一个"下游根本用不到"的密钥，且界面上会显示 has_password=true（假信息）。
func TestBuildAccountSkipsPasswordWhenUnsupported(t *testing.T) {
	svc := &ProvisioningService{}
	user := &repository.User{Name: "alice", DisplayName: "Alice"}

	account, plain := svc.buildAccount(user, nil, nil, false)
	if plain != "" || account.Password != "" {
		t.Fatalf("下游不支持口令时不应生成口令: plain=%q account.Password=%q", plain, account.Password)
	}

	withPassword, plainWith := svc.buildAccount(user, nil, nil, true)
	if plainWith == "" || withPassword.Password != plainWith {
		t.Fatalf("支持口令时应生成并带上明文口令: %+v", withPassword)
	}
}
