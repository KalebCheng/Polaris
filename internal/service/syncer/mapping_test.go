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

// 回归：用户归属部门不通过实体映射配置；历史错误配置即使指向
// dept_id_list，也只能回退到账号职务字段，不能生成数字职务。
func TestPickJobTitleIgnoresDepartmentSource(t *testing.T) {
	raw := map[string]string{
		"dept_id_list": "101,102",
		"title":        "工程师",
		"position":     "研发经理",
	}
	jobTitleMapping := map[string]string{
		"department": "dept_id_list",
		"job_title":  "dept_id_list",
	}
	if got := PickJobTitle(nil, jobTitleMapping, raw, "title"); got != "工程师" {
		t.Fatalf("历史用户归属映射应被忽略并回退 title，实际 %q", got)
	}

	jobTitleMapping = map[string]string{"name": "dept_id_list|title"}
	if got := PickJobTitle(nil, jobTitleMapping, raw, "title"); got != "工程师" {
		t.Fatalf("职务实体映射应只保留允许的 title 字段，实际 %q", got)
	}

	accountMapping := map[string]string{"title": "position"}
	if got := PickJobTitle(accountMapping, jobTitleMapping, raw, "title"); got != "工程师" {
		t.Fatalf("组织映射生效时不应被账号映射覆盖，实际 %q", got)
	}
	jobTitleMapping = nil
	if got := PickJobTitle(accountMapping, jobTitleMapping, raw, "title"); got != "研发经理" {
		t.Fatalf("组织映射缺失时应回退账号 title 映射，实际 %q", got)
	}

	accountMapping["title"] = "dept_id_list"
	if got := PickJobTitle(accountMapping, jobTitleMapping, raw, "title"); got != "工程师" {
		t.Fatalf("账号 title 映射串到部门字段时应回退默认职务字段，实际 %q", got)
	}
}

func TestNormalizeEntityMappings(t *testing.T) {
	if got := NormalizeDepartmentMapping("dingtalk", map[string]string{"name": "dept_id_list"}); got != nil {
		t.Fatalf("部门名不能取部门 ID 字段，实际 %v", got)
	}
	if got := NormalizeDepartmentMapping("dingtalk", map[string]string{"name": "name"}); got["name"] != "name" {
		t.Fatalf("部门名映射应按实体白名单保留，实际 %v", got)
	}
	gotJobTitle := NormalizeJobTitleMapping("dingtalk", map[string]string{"name": "dept_id_list|title"})
	if gotJobTitle["name"] != "title" {
		t.Fatalf("职务名映射应过滤部门字段，实际 %v", gotJobTitle)
	}
	if got := NormalizeJobTitleMapping("unknown", map[string]string{"name": "custom|title"}); got["name"] != "custom|title" {
		t.Fatalf("未知连接器不应误删实体映射，实际 %v", got)
	}

	accountMapping := map[string]string{"title": "dept_id_list", "email": "mail"}
	gotAccount := NormalizeAccountMapping("dingtalk", accountMapping)
	if _, ok := gotAccount["title"]; ok {
		t.Fatalf("账号 title 的部门字段映射应被删除，实际 %v", gotAccount)
	}
	if gotAccount["email"] != "mail" {
		t.Fatalf("无关账号映射应保持不变，实际 %v", gotAccount)
	}
}

func TestPickDepartmentsIsIndependentFromEntityMapping(t *testing.T) {
	raw := map[string]string{"dept_id_list": "101,102", "name": "技术部"}
	got := PickDepartments(raw, "dept_id_list")
	if len(got) != 2 || got[0] != "101" || got[1] != "102" {
		t.Fatalf("用户部门归属必须按官方字段自动同步，实际 %v", got)
	}
}

// 回归：raw 构造函数产出的键必须覆盖对外声明的 key 全集——
// 否则"目录里能选、运行时取不到"（钉钉 dept_id / 企微 department 的历史缺陷）。
func TestUserRawKeysCoverDeclared(t *testing.T) {
	cases := []struct {
		name     string
		raw      map[string]string
		declared []string
	}{
		{"dingtalk", dingtalkUserRaw(dingtalkUser{}), DingTalkUserSourceFieldKeys()},
		{"wecom", wecomUserRaw(wecomUser{}), WeComUserSourceFieldKeys()},
		{"lark", feishuUserRaw(feishuUser{}), FeishuUserSourceFieldKeys()},
	}
	for _, tc := range cases {
		for _, key := range tc.declared {
			if _, ok := tc.raw[key]; !ok {
				t.Fatalf("%s：声明了源字段 %q 但 raw 集合里没有（界面选了也取不到）", tc.name, key)
			}
		}
	}
}
