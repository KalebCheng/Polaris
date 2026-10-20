package oidc

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestIssuerURLNormalization discovery 与 ID Token 的 iss 必须逐字符一致：
// 配置尾部斜杠时统一去掉，避免验签方按 discovery issuer 校验 ID Token 失败。
func TestIssuerURLNormalization(t *testing.T) {
	cm, err := NewCertManager("", "")
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}
	issuer := NewIssuer(Config{CertManager: cm, IssuerURL: "https://idp.example.com/"})
	if got := issuer.IssuerURL(); got != "https://idp.example.com" {
		t.Fatalf("IssuerURL = %q，期望去掉尾部斜杠", got)
	}

	token, _, err := issuer.IssueIDToken(IDTokenParams{Subject: "uid-1", Audience: "client-abc"})
	if err != nil {
		t.Fatalf("IssueIDToken: %v", err)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims 类型异常")
	}
	if claims["iss"] != "https://idp.example.com" {
		t.Fatalf("ID Token iss = %v，期望与 discovery issuer 一致", claims["iss"])
	}
}

func TestIssueIDTokenIncludesSessionID(t *testing.T) {
	cm, err := NewCertManager("", "")
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}
	issuer := NewIssuer(Config{CertManager: cm, IssuerURL: "https://idp.example.com"})
	token, _, err := issuer.IssueIDToken(IDTokenParams{
		Subject: "uid-1", Audience: "client-abc", SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("IssueIDToken: %v", err)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["sid"] != "session-1" {
		t.Fatalf("ID Token sid = %v, want session-1", claims["sid"])
	}
}
