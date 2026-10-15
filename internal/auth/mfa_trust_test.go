package auth

import (
	"testing"
	"time"
)

// 「信任设备」令牌：签发 → 校验往返一致；机构/用户不匹配或已过期一律无效。
func TestMfaTrustToken(t *testing.T) {
	m := NewJWTManager("test-secret", "polaris", 3600)

	token, err := m.GenerateMfaTrust(7, "alice", time.Hour)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if !m.VerifyMfaTrust(token, 7, "alice") {
		t.Fatal("同机构同用户的信任令牌应有效")
	}
	if m.VerifyMfaTrust(token, 7, "bob") {
		t.Fatal("用户不匹配不应通过")
	}
	if m.VerifyMfaTrust(token, 8, "alice") {
		t.Fatal("机构不匹配不应通过")
	}
	if m.VerifyMfaTrust("", 7, "alice") {
		t.Fatal("空令牌不应通过")
	}
	if m.VerifyMfaTrust("not-a-jwt", 7, "alice") {
		t.Fatal("非法令牌不应通过")
	}
}

// 会话令牌不得被当作「信任设备」令牌使用（声明结构不同）。
func TestSessionTokenIsNotMfaTrust(t *testing.T) {
	m := NewJWTManager("test-secret", "polaris", 3600)
	sessionToken, err := m.Generate("session-123")
	if err != nil {
		t.Fatal(err)
	}
	if m.VerifyMfaTrust(sessionToken, 7, "alice") {
		t.Fatal("会话令牌不应通过信任设备校验")
	}
}
