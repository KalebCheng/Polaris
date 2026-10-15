package syncer

import (
	"reflect"
	"sort"
	"testing"
)

// 回归：映射规则支持单源字段、备选源字段（首个非空）与固定值，且向后兼容历史的无前缀写法。
func TestResolveMapping(t *testing.T) {
	sources := map[string]string{"mobile": "", "email": "a@b.com", "name": "张三"}
	lookup := func(field string) string { return sources[field] }

	cases := []struct {
		name    string
		mapping map[string]string
		key     string
		want    string
		wantOK  bool
	}{
		{name: "历史的单源字段写法", mapping: map[string]string{"name": "name"}, key: "name", want: "张三", wantOK: true},
		{name: "取空值视为未命中", mapping: map[string]string{"phone": "mobile"}, key: "phone", want: "", wantOK: false},
		{name: "备选源字段取首个非空", mapping: map[string]string{"phone": "mobile|email"}, key: "phone", want: "a@b.com", wantOK: true},
		{name: "固定值", mapping: map[string]string{"location": "=远程"}, key: "location", want: "远程", wantOK: true},
		{name: "固定值优先于源字段", mapping: map[string]string{"name": "=固定|name"}, key: "name", want: "固定", wantOK: true},
		{name: "未声明的字段不命中", mapping: map[string]string{"name": "name"}, key: "email", want: "", wantOK: false},
		{name: "空规则不命中", mapping: map[string]string{"name": "  "}, key: "name", want: "", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := ResolveMapping(tc.mapping, tc.key, lookup)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%s: 期望 (%q,%v)，实际 (%q,%v)", tc.name, tc.want, tc.wantOK, got, ok)
		}
	}
}

// 回归：LDAP 需要按规则串抓取源属性——固定值不产生属性，备选字段都要抓。
func TestMappingSourceFields(t *testing.T) {
	got := MappingSourceFields(map[string]string{
		"name":  "sAMAccountName|uid",
		"email": "mail",
		"loc":   "=远程",
	})
	// map 遍历顺序随机：按集合比较（固定值不产生源字段）
	sort.Strings(got)
	want := []string{"mail", "sAMAccountName", "uid"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("引用到的源字段错误: got %v want %v", got, want)
	}
}

// 回归：连接器侧取值——未配置映射时用默认字段，配置后以映射为准，备选与固定值同样生效。
func TestPickMapped(t *testing.T) {
	raw := map[string]string{"userid": "u_1", "name": "张三", "mobile": "", "email": "z@example.com"}
	if got := PickMapped(nil, raw, "name", "userid"); got != "u_1" {
		t.Fatalf("未配置映射应回退默认字段，实际 %q", got)
	}
	if got := PickMapped(map[string]string{"name": "email"}, raw, "name", "userid"); got != "z@example.com" {
		t.Fatalf("应取映射指定的源字段，实际 %q", got)
	}
	if got := PickMapped(map[string]string{"name": "mobile|email"}, raw, "name", "userid"); got != "z@example.com" {
		t.Fatalf("备选源字段应取首个非空，实际 %q", got)
	}
	if got := PickMapped(map[string]string{"name": "=外部用户"}, raw, "name", "userid"); got != "外部用户" {
		t.Fatalf("固定值应直接生效，实际 %q", got)
	}
	if got := PickMapped(map[string]string{"name": "unknown"}, raw, "name", "name"); got != "张三" {
		t.Fatalf("映射取空应回退默认字段，实际 %q", got)
	}
}
