package cred

import "testing"

// 全量密码类型必须满足「哈希 → 校验」往返一致，且错误密码不通过。
func TestAllManagersRoundTrip(t *testing.T) {
	const password = "P@ssw0rd-测试"
	const salt = "b3JnLXNhbHQ="
	for _, typ := range SupportedTypes() {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			m := Get(typ)
			if m.Type() != typ {
				t.Fatalf("Type() = %q, want %q", m.Type(), typ)
			}
			hash, err := m.Hash(password, salt)
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			if !m.Verify(password, hash, salt) {
				t.Fatalf("正确密码校验失败: type=%s hash=%q", typ, hash)
			}
			if m.Verify("wrong-password", hash, salt) {
				t.Fatalf("错误密码不应通过: type=%s", typ)
			}
		})
	}
}

// 空密码不可哈希（plain 例外：明文原样返回由上层校验）。
func TestEmptyPasswordRejected(t *testing.T) {
	for _, typ := range SupportedTypes() {
		if typ == TypePlain {
			continue
		}
		if _, err := Get(typ).Hash("", "salt"); err == nil {
			t.Fatalf("类型 %s 应拒绝空密码", typ)
		}
	}
}

// 未知类型回退到默认实现（bcrypt），避免配置异常锁死登录。
func TestUnknownTypeFallsBackToDefault(t *testing.T) {
	m := Get("does-not-exist")
	if m.Type() != DefaultType {
		t.Fatalf("未知类型未回退默认: got %s", m.Type())
	}
}

// 随机盐算法（bcrypt/argon2id）两次哈希应不同，但都能校验通过。
func TestRandomSaltAlgorithmsProduceDistinctHashes(t *testing.T) {
	for _, typ := range []string{TypeBcrypt, TypeArgon2id} {
		m := Get(typ)
		h1, _ := m.Hash("same-password", "")
		h2, _ := m.Hash("same-password", "")
		if h1 == h2 {
			t.Fatalf("%s 应生成不同的盐而非相同哈希", typ)
		}
		if !m.Verify("same-password", h1, "") || !m.Verify("same-password", h2, "") {
			t.Fatalf("%s 两个哈希都应校验通过", typ)
		}
	}
}

// 固定盐算法：盐不同 → 哈希不同；盐错 → 校验失败。
func TestFixedSaltAlgorithms(t *testing.T) {
	for _, typ := range []string{TypeSHA256Salt, TypeSHA512Salt, TypeMD5Salt, TypePbkdf2Salt} {
		m := Get(typ)
		h1, _ := m.Hash("pw", "salt-a")
		h2, _ := m.Hash("pw", "salt-b")
		if h1 == h2 {
			t.Fatalf("%s 不同盐应产生不同哈希", typ)
		}
		if !m.Verify("pw", h1, "salt-a") {
			t.Fatalf("%s 正确盐应通过", typ)
		}
		if m.Verify("pw", h1, "salt-b") {
			t.Fatalf("%s 错误盐不应通过", typ)
		}
	}
}

// pbkdf2-django 哈希串自描述，无需外部盐即可校验（salt 参数被忽略）。
func TestPbkdf2DjangoSelfDescribing(t *testing.T) {
	m := Get(TypePbkdf2Django)
	hash, err := m.Hash("pw", "explicitsalt")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Verify("pw", hash, "") {
		t.Fatal("django 风格哈希应可在无外部盐时校验")
	}
	if m.Verify("bad", hash, "") {
		t.Fatal("错误密码不应通过")
	}
}

// 全部受支持类型都应可枚举且互不重复。
func TestSupportedTypesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, typ := range SupportedTypes() {
		if seen[typ] {
			t.Fatalf("类型重复: %s", typ)
		}
		seen[typ] = true
		if !IsSupported(typ) {
			t.Fatalf("类型 %s 应受支持", typ)
		}
	}
	if seen[TypeBcrypt] == false {
		t.Fatal("默认类型 bcrypt 必须在受支持集合中")
	}
}
