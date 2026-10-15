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
