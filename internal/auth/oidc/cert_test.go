package oidc

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestCertManagerMultiKeyRotation 验证多密钥并存：JWKS 发布全部密钥、签发用激活密钥、
// 验签可同时通过激活密钥与旧密钥签发的令牌（平滑轮换）。
func TestCertManagerMultiKeyRotation(t *testing.T) {
	cert1, key1, kid1, err := GenerateCert(2048, 10)
	if err != nil {
		t.Fatalf("生成证书1失败: %v", err)
	}
	cert2, key2, kid2, err := GenerateCert(2048, 10)
	if err != nil {
		t.Fatalf("生成证书2失败: %v", err)
	}
	if kid1 == kid2 {
		t.Fatal("两张证书 kid 不应相同")
	}

	// 激活密钥为 cert1
	cm, err := NewCertManager(cert1, key1)
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}
	if cm.KID() != kid1 {
		t.Fatalf("激活 kid = %q，期望 %q", cm.KID(), kid1)
	}

	// 用激活密钥签发令牌
	token1, err := cm.Sign(jwt.MapClaims{"sub": "u1"})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// 追加旧密钥 cert2（模拟待退场的旧密钥）
	if err := cm.AddKey(cert2, key2); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	// 幂等：重复添加不报错、不重复
	if err := cm.AddKey(cert2, key2); err != nil {
		t.Fatalf("AddKey 幂等失败: %v", err)
	}

	// 激活 kid 不变（仍为 cert1）
	if cm.KID() != kid1 {
		t.Fatalf("追加后激活 kid = %q，期望 %q", cm.KID(), kid1)
	}

	// JWKS 应发布两个密钥
	jwks := cm.JWKS()
	keys, ok := jwks["keys"].([]map[string]interface{})
	if !ok || len(keys) != 2 {
		t.Fatalf("JWKS 应包含 2 个密钥，实际 %d", len(keys))
	}
	kids := map[string]bool{}
	for _, k := range keys {
		kids[k["kid"].(string)] = true
	}
	if !kids[kid1] || !kids[kid2] {
		t.Fatalf("JWKS 应同时包含 %q 与 %q，实际 %v", kid1, kid2, kids)
	}

	// 验签：激活密钥签发的令牌可通过
	claims := jwt.MapClaims{}
	if err := cm.Verify(token1, claims); err != nil {
		t.Fatalf("激活密钥令牌验签失败: %v", err)
	}
	if claims["sub"] != "u1" {
		t.Fatalf("claims.sub = %v，期望 u1", claims["sub"])
	}

	// 旧密钥签发的令牌也可通过（轮换期间既有令牌不失效）
	cmOld, err := NewCertManager(cert2, key2)
	if err != nil {
		t.Fatalf("NewCertManager(old): %v", err)
	}
	token2, err := cmOld.Sign(jwt.MapClaims{"sub": "u2"})
	if err != nil {
		t.Fatalf("旧密钥 Sign: %v", err)
	}
	claims2 := jwt.MapClaims{}
	if err := cm.Verify(token2, claims2); err != nil {
		t.Fatalf("旧密钥令牌应可验签（轮换期），实际: %v", err)
	}
	if claims2["sub"] != "u2" {
		t.Fatalf("claims2.sub = %v，期望 u2", claims2["sub"])
	}

	// 未知密钥签发的令牌不得通过
	cert3, key3, _, err := GenerateCert(2048, 10)
	if err != nil {
		t.Fatalf("生成证书3失败: %v", err)
	}
	_ = cert3
	cmUnknown, err := NewCertManager(cert3, key3)
	if err != nil {
		t.Fatalf("NewCertManager(unknown): %v", err)
	}
	token3, err := cmUnknown.Sign(jwt.MapClaims{"sub": "u3"})
	if err != nil {
		t.Fatalf("未知密钥 Sign: %v", err)
	}
	if err := cm.Verify(token3, jwt.MapClaims{}); err == nil {
		t.Fatal("未知密钥令牌不应通过验签")
	}
}
