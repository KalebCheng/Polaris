package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Str0ng@Pass")
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}
	if hash == "Str0ng@Pass" {
		t.Fatal("哈希不应等于明文")
	}
	if !VerifyPassword(hash, "Str0ng@Pass") {
		t.Fatal("正确密码校验应通过")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("错误密码校验应失败")
	}
	if VerifyPassword("", "anything") {
		t.Fatal("空哈希不应通过校验")
	}
	if _, err := HashPassword(""); err == nil {
		t.Fatal("空密码不应允许哈希")
	}
}

func TestPasswordPolicy(t *testing.T) {
	cases := []struct {
		name     string
		options  []string
		password string
		wantErr  bool
	}{
		{"默认策略-太短", nil, "abc123", true},
		{"默认策略-刚好8位", nil, "abcd1234", false},
		{"需要大写", []string{"At least one uppercase letter"}, "abcd1234", true},
		{"满足大写", []string{"At least one uppercase letter"}, "Abcd1234", false},
		{"需要数字", []string{"At least one digit"}, "Abcdefgh", true},
		{"需要特殊字符", []string{"At least one special character"}, "Abcdefg1", true},
		{"满足特殊字符", []string{"At least one special character"}, "Abcdefg1!", false},
		{"提升最小长度", []string{"At least 12 characters"}, "Abcdefg1234", true},
		{"满足最小长度", []string{"At least 12 characters"}, "Abcdefg12345", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := FromOptions(tc.options).Validate(tc.password)
			if tc.wantErr && err == nil {
				t.Fatalf("期望校验失败，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际失败: %v", err)
			}
		})
	}
}

func TestExtractNumber(t *testing.T) {
	if got := extractNumber("at least 8 characters", 0); got != 8 {
		t.Fatalf("extractNumber = %d, 期望 8", got)
	}
	if got := extractNumber("at least 16 characters long", 0); got != 16 {
		t.Fatalf("extractNumber = %d, 期望 16", got)
	}
	if got := extractNumber("no digits here", 7); got != 7 {
		t.Fatalf("无数字时应回退默认值，实际 %d", got)
	}
}

// 回归：枚举码强度策略（与 Casdoor 对齐）必须生效。
func TestPasswordPolicyEnumOptions(t *testing.T) {
	cases := []struct {
		name     string
		options  []string
		password string
		wantErr  bool
	}{
		{"AtLeast6-5位失败", []string{OptionAtLeast6}, "Ab1!x", true},
		{"AtLeast6-6位通过", []string{OptionAtLeast6}, "Ab1!xy", false},
		{"AtLeast8-7位失败", []string{OptionAtLeast8}, "Ab1!xyz", true},
		{"Aa123-缺大写", []string{OptionAa123}, "abcd1234", true},
		{"Aa123-满足", []string{OptionAa123}, "Abcd1234", false},
		{"SpecialChar-缺特殊", []string{OptionSpecialChar}, "Abcd1234", true},
		{"SpecialChar-满足", []string{OptionSpecialChar}, "Abcd123!", false},
		{"NoRepeat-连续重复", []string{OptionNoRepeat}, "aabcd1234", true},
		{"NoRepeat-满足", []string{OptionNoRepeat}, "Abcd1234", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := FromOptions(tc.options).Validate(tc.password)
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际失败: %v", err)
			}
		})
	}
}

// 按类型 + 盐哈希后，必须能用同样的类型/盐校验通过。
func TestHashVerifyPasswordWithTypes(t *testing.T) {
	for _, typ := range SupportedPasswordTypes() {
		hash, err := HashPasswordWith("P@ssw0rd", typ, "org-salt")
		if err != nil {
			t.Fatalf("类型 %s 哈希失败: %v", typ, err)
		}
		if !VerifyPasswordWith("P@ssw0rd", hash, typ, "org-salt") {
			t.Fatalf("类型 %s 正确密码应通过", typ)
		}
		if VerifyPasswordWith("wrong", hash, typ, "org-salt") {
			t.Fatalf("类型 %s 错误密码不应通过", typ)
		}
	}
}

// 未知/空类型回退默认算法，历史用户（password_type 为空）仍可登录。
func TestVerifyPasswordWithUnknownTypeFallsBack(t *testing.T) {
	hash, _ := HashPassword("Str0ng@Pass")
	if !VerifyPasswordWith("Str0ng@Pass", hash, "", "") {
		t.Fatal("空类型应回退 bcrypt 并校验通过")
	}
	if !VerifyPasswordWith("Str0ng@Pass", hash, "obsolete-type", "") {
		t.Fatal("未知类型应回退 bcrypt 并校验通过")
	}
}
