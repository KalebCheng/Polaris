package oidc

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestIssueLogoutToken 验证后端登出令牌的规范结构：
// typ=logout+jwt、含 aud/iss/jti/events，且至少含 sub 或 sid，并可被本签发器验签。
func TestIssueLogoutToken(t *testing.T) {
	cm, err := NewCertManager("", "") // 自动生成自签名证书
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}
	issuer := NewIssuer(Config{CertManager: cm, IssuerURL: "https://idp.example.com"})

	token, exp, err := issuer.IssueLogoutToken(LogoutTokenParams{
		Subject: "uid-1", Audience: "client-abc", SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("IssueLogoutToken: %v", err)
	}
	if exp.IsZero() {
		t.Fatal("exp 不应为零值")
	}

	// 解析 JOSE 头，校验 typ 与 kid
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	if typ, _ := parsed.Header["typ"].(string); typ != "logout+jwt" {
		t.Errorf("typ = %q，期望 logout+jwt", parsed.Header["typ"])
	}
	if kid, _ := parsed.Header["kid"].(string); kid == "" {
		t.Error("JOSE 头应包含 kid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims 类型异常")
	}
	if claims["aud"] != "client-abc" {
		t.Errorf("aud = %v，期望 client-abc", claims["aud"])
	}
	if claims["iss"] != "https://idp.example.com" {
		t.Errorf("iss = %v", claims["iss"])
	}
	if claims["sub"] != "uid-1" {
		t.Errorf("sub = %v，期望 uid-1", claims["sub"])
	}
	if claims["sid"] != "sess-1" {
		t.Errorf("sid = %v，期望 sess-1", claims["sid"])
	}
	if _, ok := claims["jti"]; !ok {
		t.Error("应包含 jti")
	}
	events, ok := claims["events"].(map[string]interface{})
	if !ok {
		t.Fatalf("events 类型异常: %T", claims["events"])
	}
	if _, ok := events["http://schemas.openid.net/event/backchannel-logout"]; !ok {
		t.Error("events 应包含 backchannel-logout 事件")
	}

	// 可被本签发器验签（复用同一密钥）
	verified := jwt.MapClaims{}
	if err := cm.Verify(token, verified); err != nil {
		t.Fatalf("logout_token 验签失败: %v", err)
	}
}
