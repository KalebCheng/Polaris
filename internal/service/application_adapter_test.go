package service

import (
	"testing"

	"polaris/internal/repository"
)

// TestApplyAdapterDefault 验证 default 适配器不改动标准 claims。
func TestApplyAdapterDefault(t *testing.T) {
	user := &repository.User{Name: "alice", Email: "alice@example.com", InstitutionID: 1}
	claims := map[string]interface{}{"sub": "uid-1", "email": "alice@example.com"}
	app := &repository.Application{Adapter: "default", AdapterConfig: &repository.AdapterConfig{
		ClaimMap: map[string]string{"email": "phone"},
	}}
	applyAdapter(app, user, claims)
	if claims["email"] != "alice@example.com" {
		t.Fatalf("default 适配器不应改动 claim，实际 email=%v", claims["email"])
	}
}

// TestApplyAdapterCustomOverridesAndAdds 验证 custom 适配器按 claimMap 覆盖/新增 claim。
func TestApplyAdapterCustomOverridesAndAdds(t *testing.T) {
	user := &repository.User{
		Name: "alice", UID: "uid-1", Email: "alice@example.com", Phone: "13800001111",
		InstitutionID: 1, DisplayName: "Alice Wang",
	}
	claims := map[string]interface{}{"sub": "uid-1", "email": "alice@example.com"}
	app := &repository.Application{Adapter: "custom", AdapterConfig: &repository.AdapterConfig{
		ClaimMap: map[string]string{
			"email":      "phone",       // 覆盖既有 claim
			"employeeNo": "uid",         // 新增 claim
			"department": "title",       // 空值 → 不写入
			"bogus":      "not-an-attr", // 未知属性 → 忽略
			"":           "email",       // 空 claim 名 → 忽略
			"bad":        "",            // 空属性 → 忽略
		},
	}}
	applyAdapter(app, user, claims)

	if claims["email"] != "13800001111" {
		t.Errorf("email 应被覆盖为 phone，实际 %v", claims["email"])
	}
	if claims["employeeNo"] != "uid-1" {
		t.Errorf("employeeNo 应新增为 uid，实际 %v", claims["employeeNo"])
	}
	if _, ok := claims["department"]; ok {
		t.Errorf("空属性值不应写入 claim，实际 %v", claims["department"])
	}
	if _, ok := claims["bogus"]; ok {
		t.Error("未知属性不应写入 claim")
	}
	if _, ok := claims[""]; ok {
		t.Error("空 claim 名不应写入")
	}
	if _, ok := claims["bad"]; ok {
		t.Error("空属性名不应写入")
	}
}

// TestApplyAdapterCustomNoConfig 验证 custom 但无配置时不 panic 且不改动。
func TestApplyAdapterCustomNoConfig(t *testing.T) {
	user := &repository.User{Name: "alice"}
	claims := map[string]interface{}{"sub": "uid-1"}
	applyAdapter(&repository.Application{Adapter: "custom"}, user, claims)
	applyAdapter(nil, user, claims)
	applyAdapter(&repository.Application{Adapter: "custom", AdapterConfig: &repository.AdapterConfig{}}, user, claims)
	if len(claims) != 1 || claims["sub"] != "uid-1" {
		t.Fatalf("无配置时不应改动 claims，实际 %v", claims)
	}
}
