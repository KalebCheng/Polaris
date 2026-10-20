package syncer

import "strings"

// 字段映射的取值编码（存储形态：map[本地字段] = 规则串）
//
//	"mobile"              → 取源字段 mobile
//	"mobile|email"        → 依次取第一个非空的源字段（备选源字段，对应界面的「添加」）
//	"=未知"               → 固定值（对应界面的「映射规则：固定值」）
//
// 历史数据是无前缀的单值（等价于只有一段的规则串），因此本编码向后兼容。
const (
	// MappingConstantPrefix 固定值前缀
	MappingConstantPrefix = "="
	// MappingFallbackSeparator 备选源字段分隔符
	MappingFallbackSeparator = "|"
)

// ResolveMapping 按映射规则取值：lookup 负责按源字段名取原始值，返回是否命中。
// 全部候选都为空（或规则为空）时返回 false，调用方沿用内置默认字段。
func ResolveMapping(mapping map[string]string, localKey string, lookup func(field string) string) (string, bool) {
	spec, ok := mapping[localKey]
	if !ok {
		return "", false
	}
	for _, candidate := range ParseMappingSpec(spec) {
		if candidate.Constant != "" {
			return candidate.Constant, true
		}
		if candidate.Field == "" {
			continue
		}
		if v := lookup(candidate.Field); v != "" {
			return v, true
		}
	}
	return "", false
}

// MappingCandidate 一条映射候选（要么是源字段，要么是固定值）
type MappingCandidate struct {
	Field    string
	Constant string
}

// ParseMappingSpec 解析规则串为候选列表（固定值优先返回，忽略空白项）。
func ParseMappingSpec(spec string) []MappingCandidate {
	out := make([]MappingCandidate, 0, 2)
	for _, raw := range strings.Split(spec, MappingFallbackSeparator) {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, MappingConstantPrefix) {
			value := strings.TrimSpace(strings.TrimPrefix(item, MappingConstantPrefix))
			if value != "" {
				out = append(out, MappingCandidate{Constant: value})
			}
			continue
		}
		out = append(out, MappingCandidate{Field: item})
	}
	return out
}

// userDepartmentSourceFieldsByType lists user fields that carry department
// membership. They are synchronized automatically and must never leak into the
// account title fallback used to derive job titles.
var userDepartmentSourceFieldsByType = map[string]map[string]bool{
	"dingtalk": {"dept_id_list": true},
	"wecom":    {"department": true, "main_department": true},
	"lark":     {"department_ids": true},
	"ldap":     {"memberOf": true},
	"database": {"dept_name": true, "dept_id": true},
}

var allUserDepartmentSourceFields = map[string]bool{
	"dept_id_list": true, "department": true, "main_department": true,
	"department_ids": true, "memberOf": true, "dept_name": true, "dept_id": true,
}

// Entity mappings are local department/job-title fields, not user relationship
// fields. Each mapping is restricted to sources that belong to that entity, so a
// department ID can never become a job-title name.
var departmentEntitySourceFieldsByType = map[string]map[string]bool{
	"dingtalk": {"name": true},
	"wecom":    {"name": true},
	"lark":     {"name": true},
	"ldap":     {"cn": true},
}

var jobTitleEntitySourceFieldsByType = map[string]map[string]bool{
	"dingtalk": {"title": true},
	"wecom":    {"title": true},
	"lark":     {"job_title": true},
	"ldap":     {"title": true},
	"database": {"title": true},
}

var jobTitleBlockedSourceFields = map[string]bool{
	"dept_id_list":       true,
	"department":         true,
	"main_department":    true,
	"department_ids":     true,
	"open_department_id": true,
	"department_name":    true,
	"memberOf":           true,
	"distinguishedName":  true,
	"dept_name":          true,
	"dept_id":            true,
}

// NormalizeDepartmentMapping keeps the department entity's name mapping and
// filters it to source fields that belong to a department object.
func NormalizeDepartmentMapping(syncerType string, mapping map[string]string) map[string]string {
	return normalizeEntityNameMapping(mapping, departmentEntitySourceFieldsByType[syncerType], nil)
}

// NormalizeJobTitleMapping keeps the job-title entity's name mapping and filters
// it to source fields that belong to a user's job-title attribute.
func NormalizeJobTitleMapping(syncerType string, mapping map[string]string) map[string]string {
	return normalizeEntityNameMapping(mapping, jobTitleEntitySourceFieldsByType[syncerType], jobTitleBlockedSourceFields)
}

func normalizeEntityNameMapping(mapping map[string]string, allowed, blocked map[string]bool) map[string]string {
	if len(mapping) == 0 {
		return mapping
	}
	spec, ok := mapping["name"]
	if !ok {
		return nil
	}
	normalized, ok := normalizeMappingSpec(spec, func(candidate MappingCandidate) bool {
		if candidate.Constant != "" {
			return true
		}
		if blocked != nil && blocked[candidate.Field] {
			return false
		}
		return allowed == nil || allowed[candidate.Field]
	})
	if !ok {
		return nil
	}
	return map[string]string{"name": normalized}
}

// NormalizeAccountMapping prevents the account-level title mapping from consuming
// department membership fields. The account title is also the fallback source
// for the derived job title.
func NormalizeAccountMapping(syncerType string, mapping map[string]string) map[string]string {
	if len(mapping) == 0 {
		return mapping
	}
	spec, ok := mapping["title"]
	if !ok {
		return mapping
	}
	blocked := userDepartmentSourceFieldsByType[syncerType]
	if blocked == nil {
		blocked = allUserDepartmentSourceFields
	}
	normalized, ok := normalizeMappingSpec(spec, func(candidate MappingCandidate) bool {
		return candidate.Constant != "" || blocked == nil || !blocked[candidate.Field]
	})
	out := make(map[string]string, len(mapping))
	for key, value := range mapping {
		if key != "title" {
			out[key] = value
		}
	}
	if ok {
		out["title"] = normalized
	}
	return out
}

func normalizeMappingSpec(spec string, keep func(MappingCandidate) bool) (string, bool) {
	kept := make([]MappingCandidate, 0, 2)
	for _, candidate := range ParseMappingSpec(spec) {
		if keep(candidate) {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	return encodeMappingCandidates(kept), true
}

func encodeMappingCandidates(candidates []MappingCandidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Constant != "" {
			parts = append(parts, MappingConstantPrefix+candidate.Constant)
			continue
		}
		if candidate.Field != "" {
			parts = append(parts, candidate.Field)
		}
	}
	return strings.Join(parts, MappingFallbackSeparator)
}

// MappingSourceFields 列出规则串里引用到的**源字段**（供 LDAP 决定要抓取哪些属性）。
func MappingSourceFields(mapping map[string]string) []string {
	seen := map[string]bool{}
	fields := make([]string, 0, len(mapping))
	for _, spec := range mapping {
		for _, candidate := range ParseMappingSpec(spec) {
			if candidate.Field == "" || seen[candidate.Field] {
				continue
			}
			seen[candidate.Field] = true
			fields = append(fields, candidate.Field)
		}
	}
	return fields
}

// PickMapped 按字段映射从"源字段 → 值"的原始集合里取值；未配置或取空时回退到该连接器的默认字段。
// 供各连接器统一实现字段映射（钉钉/飞书/企业微信/LDAP/数据库共用同一套规则语义）。
func PickMapped(mapping map[string]string, raw map[string]string, localKey, defaultField string) string {
	if v, ok := ResolveMapping(mapping, localKey, func(field string) string { return raw[field] }); ok {
		return v
	}
	return raw[defaultField]
}

// SplitDepartments 把源端的部门标识串拆成列表。
// 分隔符按各平台官方文档的取值形态统一处理（逗号 / 分号），并去掉空白项。
func SplitDepartments(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// PickJobTitle derives the local job-title name from job_title_mapping. If it
// is not configured, the account title mapping/default remains the fallback.
func PickJobTitle(accountMapping, jobTitleMapping map[string]string, raw map[string]string, defaultField string) string {
	lookup := func(field string) string { return raw[field] }
	jobTitleMapping = NormalizeJobTitleMapping("", jobTitleMapping)
	if v, ok := ResolveMapping(jobTitleMapping, "name", lookup); ok {
		return v
	}
	accountMapping = NormalizeAccountMapping("", accountMapping)
	return PickMapped(accountMapping, raw, "title", defaultField)
}

// PickDepartments returns the user's official department membership. User
// relationships are never configured through department_mapping.
func PickDepartments(raw map[string]string, defaultField string) []string {
	return SplitDepartments(raw[defaultField])
}
