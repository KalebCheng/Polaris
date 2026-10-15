package auth

import (
	"testing"
	"time"
)

// TestJWTOnlyCarriesSessionID 红线：JWT 仅携带 session ID，不得内嵌身份信息。
func TestJWTOnlyCarriesSessionID(t *testing.T) {
	m := NewJWTManager("test-secret", "polaris", 3600)
	token, err := m.Generate("session-abc")
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if claims.SessionID != "session-abc" {
		t.Fatalf("SessionID = %q", claims.SessionID)
	}
	if claims.Issuer != "polaris" {
		t.Fatalf("issuer = %q", claims.Issuer)
	}
	if claims.Subject != "session-abc" {
		t.Fatalf("subject 应等于 session id，实际 %q", claims.Subject)
	}
}

func TestJWTRejectsWrongSecret(t *testing.T) {
	issuer := NewJWTManager("secret-a", "polaris", 3600)
	token, err := issuer.Generate("s1")
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	attacker := NewJWTManager("secret-b", "polaris", 3600)
	if _, err := attacker.Parse(token); err == nil {
		t.Fatal("使用不同密钥不应解析成功")
	}
}

func TestJWTRejectsExpired(t *testing.T) {
	m := NewJWTManager("test-secret", "polaris", -1) // 立即过期
	token, err := m.Generate("s1")
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := m.Parse(token); err == nil {
		t.Fatal("过期令牌不应解析成功")
	}
}
