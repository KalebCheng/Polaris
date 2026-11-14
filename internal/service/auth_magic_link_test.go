package service

import (
	"testing"

	"polaris/internal/config"
)

// 魔法链接落地地址必须只指向本平台，并携带 token（可选 client_id）。
func TestMagicLinkURLUsesConfiguredBaseURL(t *testing.T) {
	svc := &AuthService{oauthCfg: config.OAuthConfig{BaseURL: "https://id.example.com/"}}
	got := svc.magicLinkURL("ignored.example.com", "tok-123", "cid-1")
	want := "https://id.example.com/magic?token=tok-123&client_id=cid-1"
	if got != want {
		t.Fatalf("期望 %q，实际 %q", want, got)
	}
}

// 未配置 base_url 时回退到请求 Host；本机地址降级为 http。
func TestMagicLinkURLFallsBackToRequestHost(t *testing.T) {
	svc := &AuthService{}
	if got := svc.magicLinkURL("localhost:8100", "tok", ""); got != "http://localhost:8100/magic?token=tok" {
		t.Fatalf("本机应使用 http 且不带 client_id，实际 %q", got)
	}
	if got := svc.magicLinkURL("id.example.com", "tok", ""); got != "https://id.example.com/magic?token=tok" {
		t.Fatalf("非本机应使用 https，实际 %q", got)
	}
}
