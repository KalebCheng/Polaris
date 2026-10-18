package connection

import (
	"strings"
	"testing"

	"polaris/internal/service/syncer"
)

// 目录中每个类型都必须有对应实现——杜绝"目录里能选、后端没实现"（AGENTS.md §3.1-10）。
func TestCatalogTypesAreImplemented(t *testing.T) {
	cat := Build()
	if len(cat.Directions) != 2 {
		t.Fatalf("目录应含入/出两个方向，实际 %d", len(cat.Directions))
	}
	for _, group := range cat.Directions {
		if len(group.Types) == 0 {
			t.Fatalf("方向 %s 的目录为空（实现应至少注册一个连接器）", group.Direction)
		}
		for _, info := range group.Types {
			if info.Key == "" || info.Name == "" || info.Icon.Key == "" {
				t.Fatalf("类型缺少 key/name/icon: %+v", info)
			}
			if _, ok := Find(group.Direction, info.Key); !ok {
				t.Fatalf("Find 未找到类型 %s", info.Key)
			}
		}
	}
}

// 本阶段不接入 SCIM（用户决定）：目录中不得出现（见 Spec §10）。
func TestCatalogExcludesSCIM(t *testing.T) {
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			if info.Key == "scim" {
				t.Fatalf("方向 %s 不应包含 scim（本阶段范围外）", group.Direction)
			}
		}
	}
}

// 字段声明必须自洽：key 唯一、敏感字段以 has_value 表达、条件字段指向同类型的 select 字段。
func TestCatalogFieldDeclarations(t *testing.T) {
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			seen := map[string]Field{}
			for _, f := range info.Fields {
				if f.Key == "" || f.Label == "" {
					t.Fatalf("%s: 字段缺少 key/label: %+v", info.Key, f)
				}
				if _, dup := seen[f.Key]; dup {
					t.Fatalf("%s: 字段 %s 重复声明", info.Key, f.Key)
				}
				if f.Secret && !f.HasValue {
					t.Fatalf("%s: 敏感字段 %s 必须以 has_value 表达\"是否已配置\"（禁止下发明文）", info.Key, f.Key)
				}
				if f.ShowWhen != nil {
					dep, ok := seen[f.ShowWhen.Field]
					if !ok {
						t.Fatalf("%s: 字段 %s 的 show_when 指向未声明字段 %s", info.Key, f.Key, f.ShowWhen.Field)
					}
					if dep.Type != FieldSelect {
						t.Fatalf("%s: show_when 依赖的字段 %s 必须是下拉选择", info.Key, dep.Key)
					}
				}
				if (f.Type == FieldSelect) != (len(f.Options) > 0) {
					t.Fatalf("%s: 字段 %s 的下拉选项与类型不匹配", info.Key, f.Key)
				}
				seen[f.Key] = f
			}
			if _, ok := seen["name"]; !ok {
				t.Fatalf("%s: 缺少标识字段 name", info.Key)
			}
		}
	}
}

// 入方向声明支持字段映射的类型，必须提供可映射源字段（否则「字段映射」页签下拉为空）。
// 出方向的映射源是本地用户属性，由账号策略承载，不需要 source_fields。
func TestCatalogMappingCapabilityRequiresSourceFields(t *testing.T) {
	for _, group := range Build().Directions {
		if group.Direction != DirectionIn {
			continue
		}
		for _, info := range group.Types {
			if info.Capabilities.Mapping && !hasAccountAndOrgSource(info.SourceFields) {
				t.Fatalf("%s: 声明了字段映射能力，但缺少账户/组织源字段", info.Key)
			}
		}
	}
}

// 变体（AD/OpenLDAP、MySQL/PostgreSQL、飞书/Lark）的 key 必须唯一且有图标。
func TestCatalogVariantsAreConsistent(t *testing.T) {
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			seen := map[string]bool{}
			for _, v := range info.Variants {
				if v.Key == "" || v.Name == "" || v.Icon == "" {
					t.Fatalf("%s: 变体声明不完整: %+v", info.Key, v)
				}
				if seen[v.Key] {
					t.Fatalf("%s: 变体 %s 重复", info.Key, v.Key)
				}
				seen[v.Key] = true
			}
		}
	}
}

// 出方向类型必须能绑定到某个应用（供给的幂等键是 应用×用户）。
func TestOutboundTypesBindApplication(t *testing.T) {
	for _, group := range Build().Directions {
		if group.Direction != DirectionOut {
			continue
		}
		for _, info := range group.Types {
			found := false
			for _, f := range info.Fields {
				if f.Key == "application" && f.Required {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: 出方向类型必须声明必填的 application 字段", info.Key)
			}
		}
	}
}

// 目标字段（字段映射的行）必须包含账号名，且为必填；键名必须是连接器真正读取的本地键。
func TestTargetFieldsRequireKeyFields(t *testing.T) {
	required := map[string]bool{}
	for _, f := range targetFields() {
		if f.Required {
			required[f.Key] = true
		}
	}
	for _, key := range []string{"name"} {
		if !required[key] {
			t.Fatalf("目标字段 %s 必须为必填（连接器要按该键取值）", key)
		}
	}
}

func hasAccountAndOrgSource(fields []SourceField) bool {
	account, org := false, false
	for _, f := range fields {
		switch f.Group {
		case "account":
			account = true
		case "organization":
			org = true
		}
	}
	return account && org
}

// 知识性断言：入方向图标 key 必须能在前端素材命名空间里找到对应（文件名或内联兜底）。
func TestIconKeysAreSlug(t *testing.T) {
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			keys := []string{info.Icon.Key}
			for _, v := range info.Icon.BySubType {
				keys = append(keys, v)
			}
			for _, v := range info.Variants {
				keys = append(keys, v.Icon)
			}
			for _, k := range keys {
				if k != strings.ToLower(k) || strings.ContainsAny(k, " /") {
					t.Fatalf("%s: 图标 key %q 必须是小写 slug（与素材文件名一致）", info.Key, k)
				}
			}
		}
	}
}

// 回归：每个字段都必须有归属步骤。
// 曾出现"通用字段（标识/名称）与同步范围字段被塞进类型字段列表，结果在多个步骤里重复渲染"。
func TestCatalogFieldsHaveStepGroup(t *testing.T) {
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			counts := map[string]int{}
			for _, field := range info.Fields {
				switch field.Group {
				case FieldGroupBasic, FieldGroupConnection, FieldGroupScope:
					counts[field.Group]++
				default:
					t.Fatalf("%s: 字段 %s 没有归属步骤（group=%q）", info.Key, field.Key, field.Group)
				}
			}
			if counts[FieldGroupBasic] == 0 {
				t.Fatalf("%s: 缺少基本信息字段", info.Key)
			}
			if counts[FieldGroupConnection] == 0 {
				t.Fatalf("%s: 缺少连接配置字段（该类型没有任何可填的连接配置）", info.Key)
			}
		}
	}
}

// 回归：通用字段与范围字段只能出现在对应的那一步（不得跨步重复）。
func TestCatalogFieldGroupsAreExclusive(t *testing.T) {
	want := map[string]string{
		"name": FieldGroupBasic, "display_name": FieldGroupBasic, "is_enabled": FieldGroupBasic,
		"sync_users": FieldGroupScope, "sync_groups": FieldGroupScope, "soft_delete": FieldGroupScope,
		"application": FieldGroupScope, "state": FieldGroupScope,
	}
	for _, group := range Build().Directions {
		for _, info := range group.Types {
			for _, field := range info.Fields {
				expected, ok := want[field.Key]
				if !ok {
					continue
				}
				if field.Group != expected {
					t.Fatalf("%s: 字段 %s 应归属 %s，实际 %s", info.Key, field.Key, expected, field.Group)
				}
			}
		}
	}
}

// 回归：入方向类型都声明了字段映射能力（连接器侧已统一按映射取值），
// 否则界面不显示「字段映射」，用户会以为"字段映射不见了"。
func TestInboundTypesSupportFieldMapping(t *testing.T) {
	for _, group := range Build().Directions {
		if group.Direction != DirectionIn {
			continue
		}
		for _, info := range group.Types {
			if !info.Capabilities.Mapping {
				t.Fatalf("%s: 入方向类型应支持字段映射", info.Key)
			}
		}
	}
}

// 回归：源字段下拉要与连接器"真正取得到"的字段一致——键名必须是连接器 raw 集合里的键。
// 曾出现"目录里列了字段、连接器根本不读"与"能取到的字段（工号/区号等）没列出来"两类问题。
func TestInboundSourceFieldsMatchConnectors(t *testing.T) {
	// 与各连接器 raw 集合保持一致（改动连接器取值时同步更新此清单，测试会挡住遗漏）
	known := map[string]map[string]bool{
		"dingtalk": keys("userid", "unionid", "name", "mobile", "state_code", "telephone",
			"email", "org_email", "job_number", "title", "avatar", "work_place", "dept_id", "dept_name"),
		"wecom": keys("userid", "name", "alias", "mobile", "telephone", "email", "title",
			"avatar", "address", "dept_id", "dept_name"),
		// lark 的账号侧候选直接取自连接器声明（目录/连接器/测试不再各写一份清单）
		"lark": keys(append(syncer.FeishuUserSourceFieldKeys(), "open_department_id", "department_name")...),
	}
	for _, group := range Build().Directions {
		if group.Direction != DirectionIn {
			continue
		}
		for _, info := range group.Types {
			allowed, ok := known[info.Key]
			if !ok {
				continue // 数据库/LDAP 的源字段由部署环境决定，不在本用例约束内
			}
			for _, field := range info.SourceFields {
				if !allowed[field.Key] {
					t.Fatalf("%s: 源字段 %s 在连接器取值集合里不存在（下拉里选了也取不到）", info.Key, field.Key)
				}
			}
			for _, field := range info.SourceFields {
				delete(allowed, field.Key)
			}
			if len(allowed) > 0 {
				t.Fatalf("%s: 连接器能取到但目录未声明（界面里选不到）: %v", info.Key, allowed)
			}
		}
	}
}

func keys(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

// 回归：跨类型策略字段（同步目标 / 定时校验 / 同步保护）必须声明落库路径，
// 否则界面上能填、后端却读不到（"配了不生效"）。
func TestPolicyFieldsDeclareStorePath(t *testing.T) {
	wantStore := map[string]string{
		"target_department_id":      "target_department_id",
		"schedule_mode":             "sync_config.schedule.mode",
		"schedule_interval_seconds": "sync_config.schedule.interval_seconds",
		"schedule_times":            "sync_config.schedule.times",
		"guard_max_user_deletes":    "sync_config.guard.max_user_deletes",
	}
	for _, group := range Build().Directions {
		if group.Direction != DirectionIn {
			continue
		}
		for _, info := range group.Types {
			found := map[string]string{}
			for _, field := range info.Fields {
				if expected, ok := wantStore[field.Key]; ok {
					if field.Store != expected {
						t.Fatalf("%s: 字段 %s 的落库路径应为 %s，实际 %q", info.Key, field.Key, expected, field.Store)
					}
					if field.Group != FieldGroupScope {
						t.Fatalf("%s: 策略字段 %s 应归属同步范围步骤", info.Key, field.Key)
					}
					found[field.Key] = field.Store
				}
			}
			for key := range wantStore {
				if _, ok := found[key]; !ok {
					t.Fatalf("%s: 缺少策略字段 %s", info.Key, key)
				}
			}
			// 间隔/时刻字段必须按 schedule_mode 条件展示，避免"两个都填"造成歧义
			for _, field := range info.Fields {
				if field.Key == "schedule_interval_seconds" && field.ShowWhenMode != "interval" {
					t.Fatalf("%s: 间隔字段应仅在 interval 模式展示", info.Key)
				}
				if field.Key == "schedule_times" && field.ShowWhenMode != "daily" {
					t.Fatalf("%s: 时刻字段应仅在 daily 模式展示", info.Key)
				}
			}
		}
	}
}
