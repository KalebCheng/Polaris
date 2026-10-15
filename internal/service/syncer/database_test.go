package syncer

import "testing"

// 回归：数据库连接器的字段映射必须能从列名推断默认字段，且 user_mapping 优先。
func TestMapRowsDefaultColumns(t *testing.T) {
	columns := []string{"id", "username", "display_name", "email", "mobile", "title"}
	rows := [][]string{
		{"1", "zhangsan", "张三", "zhangsan@example.com", "13800000000", "工程师"},
	}
	users := mapRows(columns, rows, nil)
	if len(users) != 1 {
		t.Fatalf("解析出 %d 条用户, 期望 1", len(users))
	}
	u := users[0]
	if u.ID != "1" || u.Username != "zhangsan" || u.Name != "张三" ||
		u.Email != "zhangsan@example.com" || u.Phone != "13800000000" || u.Title != "工程师" {
		t.Fatalf("字段映射错误: %+v", u)
	}
}

func TestMapRowsUserMappingOverrides(t *testing.T) {
	columns := []string{"user_id", "account", "real_name", "mail_addr"}
	rows := [][]string{{"42", "lisi", "李四", "lisi@example.com"}}
	mapping := map[string]string{
		"id":          "user_id",
		"name":        "account",
		"displayName": "real_name",
		"email":       "mail_addr",
	}
	users := mapRows(columns, rows, mapping)
	if len(users) != 1 {
		t.Fatalf("解析出 %d 条用户, 期望 1", len(users))
	}
	u := users[0]
	if u.ID != "42" || u.Username != "lisi" || u.Name != "李四" || u.Email != "lisi@example.com" {
		t.Fatalf("user_mapping 未被优先使用: %+v", u)
	}
}

func TestMapRowsFallbacksForMissingColumns(t *testing.T) {
	// 只有 uid 列时，username 与 name 应回退为 uid
	columns := []string{"uid"}
	rows := [][]string{{"u-1"}}
	users := mapRows(columns, rows, nil)
	if len(users) != 1 {
		t.Fatalf("期望 1 条用户")
	}
	u := users[0]
	if u.ID == "" || u.Username == "" || u.Name == "" {
		t.Fatalf("缺失列时应回退填充: %+v", u)
	}
}

func TestRegistryHasBuiltinConnectors(t *testing.T) {
	for _, want := range []string{"dingtalk", "lark", "wecom", "database", "ldap", "scim"} {
		c, err := Get(want)
		if err != nil {
			t.Fatalf("内置连接器 %s 未注册: %v", want, err)
		}
		if c.Type() != want {
			t.Fatalf("连接器 %s 的 Type() = %q", want, c.Type())
		}
	}
	if _, err := Get("unknown-connector"); err == nil {
		t.Fatal("未注册类型应当报错")
	}
}

func TestQuoteIdentStripsUnsafeChars(t *testing.T) {
	if got := quoteIdent("users; DROP TABLE x"); got != "usersDROPTABLEx" {
		t.Fatalf("quoteIdent 应剔除不安全字符，实际 %q", got)
	}
	if got := quoteIdent("public.users"); got != "public.users" {
		t.Fatalf("quoteIdent 应保留 schema 前缀，实际 %q", got)
	}
}
