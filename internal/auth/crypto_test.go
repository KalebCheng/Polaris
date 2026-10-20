package auth

import "testing"

func TestSecretCipherRoundTrip(t *testing.T) {
	key := DeriveKey("unit-test-key")
	c, err := NewSecretCipher(key)
	if err != nil {
		t.Fatalf("NewSecretCipher 失败: %v", err)
	}
	plain := "JBSWY3DPEHPK3PXP"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	if enc == plain {
		t.Fatal("密文不应等于明文")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt 失败: %v", err)
	}
	if dec != plain {
		t.Fatalf("解密结果 = %q, 期望 %q", dec, plain)
	}

	// 相同明文两次加密应得到不同密文（随机 nonce）
	enc2, _ := c.Encrypt(plain)
	if enc == enc2 {
		t.Fatal("相同明文两次加密结果不应相同")
	}

	// 空串往返
	if v, err := c.Encrypt(""); err != nil || v != "" {
		t.Fatalf("空串加密应为空，实际 %q, %v", v, err)
	}
	if v, err := c.Decrypt(""); err != nil || v != "" {
		t.Fatalf("空串解密应为空，实际 %q, %v", v, err)
	}
}

func TestSecretCipherRejectsBadInput(t *testing.T) {
	if _, err := NewSecretCipher([]byte("too-short")); err == nil {
		t.Fatal("密钥长度非 32 字节应当报错")
	}
	c, _ := NewSecretCipher(DeriveKey("k"))
	if _, err := c.Decrypt("not-base64!!"); err == nil {
		t.Fatal("非法 base64 应当报错")
	}
	if _, err := c.Decrypt("YWJj"); err == nil {
		t.Fatal("过短密文应当报错")
	}
}
