package errors

import "testing"

// TestCodeValuesUnique 业务码 wire 值必须全局唯一（同名不同义的 code 会导致前端分支错乱）。
func TestCodeValuesUnique(t *testing.T) {
	seen := make(map[string]int, len(allCodes))
	for i, c := range allCodes {
		if c.value == "" {
			t.Fatalf("第 %d 个业务码 value 为空", i)
		}
		if prev, ok := seen[c.value]; ok {
			t.Fatalf("业务码 %q 重复声明（索引 %d 与 %d）", c.value, prev, i)
		}
		seen[c.value] = i
	}
}

// TestCodeMetadataComplete 每个业务码都必须带默认文案、HTTP 状态码与开发说明。
func TestCodeMetadataComplete(t *testing.T) {
	for _, c := range allCodes {
		if c.message == "" {
			t.Errorf("业务码 %s 缺少默认 message", c.value)
		}
		if c.httpStatus < 100 || c.httpStatus > 599 {
			t.Errorf("业务码 %s 的 HTTP 状态码非法: %d", c.value, c.httpStatus)
		}
		if c.description == "" {
			t.Errorf("业务码 %s 缺少 description", c.value)
		}
	}
}

// TestCodeLookupAndMarshal 验证按 wire 值反查与 JSON 序列化行为。
func TestCodeLookupAndMarshal(t *testing.T) {
	got, ok := lookupCode("NOT_FOUND")
	if !ok || got != CodeNotFound {
		t.Fatalf("lookupCode(NOT_FOUND) = %v, %v", got, ok)
	}
	if _, ok := lookupCode("NO_SUCH_CODE"); ok {
		t.Fatal("未知 code 不应命中")
	}

	b, err := CodeInvalidParams.MarshalText()
	if err != nil || string(b) != "INVALID_PARAMS" {
		t.Fatalf("MarshalText = %q, %v", string(b), err)
	}
	var c Code
	if err := c.UnmarshalText([]byte("FORBIDDEN")); err != nil || c != CodeForbidden {
		t.Fatalf("UnmarshalText 失败: %v, %v", c, err)
	}
	if err := c.UnmarshalText([]byte("BOGUS")); err == nil {
		t.Fatal("未知 code 反序列化应当报错")
	}
}
