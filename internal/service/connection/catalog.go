// Package connection 提供「身份连接」的类型目录与字段 schema（单一来源）。
//
// 设计要点（见 docs/specs/261013-identity-connections-phase3.md FR-2 / FR-15）：
//   - 后端是类型的**唯一声明处**：类型、字段、图标、能力、可映射源字段都在这里声明；
//     前端按本目录渲染目录卡片与向导表单，不再自行硬编码类型清单。
//   - 目录只声明**已实现**的类型：入方向由 syncer 注册表校验，出方向由 provisioner 注册表校验，
//     未实现的类型不会出现在目录里（杜绝"能选但没实现"）。
//   - 字段 key 与对外 API/DTO 保持同一套扁平命名；存储层（config JSONB）的变化不改变对外契约。
package connection

import (
	"sort"
	"strings"

	"polaris/internal/service/provisioner"
	"polaris/internal/service/syncer"
)

// Direction 连接方向
type Direction string

const (
	// DirectionIn 入方向：从外部系统拉取组织与账号
	DirectionIn Direction = "in"
	// DirectionOut 出方向：把本地账号推送给下游应用
	DirectionOut Direction = "out"
)

// FieldType 字段控件类型（前端按此选择渲染器）
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldPassword FieldType = "password"
	FieldNumber   FieldType = "number"
	FieldSwitch   FieldType = "switch"
	FieldSelect   FieldType = "select"
	// FieldTags 多值标签（如"每天多个时刻"）
	FieldTags FieldType = "tags"
	// FieldDepartment 组织（部门树）单选
	FieldDepartment FieldType = "department"
)

// FieldOption 下拉选项
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// 字段分组：决定该字段出现在向导的哪一步（后端声明，前端照此渲染，避免"字段乱串"）
const (
	FieldGroupBasic      = "basic"      // 基本信息：标识 / 名称 / 启用
	FieldGroupConnection = "connection" // 连接配置：凭据与地址（类型专属）
	FieldGroupScope      = "scope"      // 同步范围与计划：同步账号/组织、软删、关联应用、状态
)

// Condition 条件展示（按同类型内另一个字段的取值决定是否展示）
type Condition struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// Field 一个表单字段（含校验与展示元数据）
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Group 该字段归属的向导步骤（basic | connection | scope）
	Group string `json:"group"`
	// Store 该字段在资源里的落库路径（点分）；为空表示与 Key 同名（如 host、app_id）。
	// 用于"同步目标/定时校验/同步保护"这类跨类型策略字段（如 sync_config.schedule.mode）。
	Store       string        `json:"store,omitempty"`
	Type        FieldType     `json:"type"`
	Required    bool          `json:"required,omitempty"`
	Secret      bool          `json:"secret,omitempty"`
	HasValue    bool          `json:"has_value,omitempty"` // 敏感字段以 has_* 表达是否已配置
	Placeholder string        `json:"placeholder,omitempty"`
	Hint        string        `json:"hint,omitempty"`
	Options     []FieldOption `json:"options,omitempty"`
	// OptionsSource marks a field whose options are loaded from a runtime resource.
	OptionsSource string `json:"options_source,omitempty"`
	Default       string `json:"default,omitempty"`
	// ShowWhen 条件展示：当同类型配置里某个字段等于指定值时展示（避免"无关字段铺一屏"）
	ShowWhen *Condition `json:"show_when,omitempty"`
	// ShowWhenMode 便捷写法：仅当 schedule_mode 等于该值时展示
	ShowWhenMode string `json:"show_when_mode,omitempty"`
}

// SourceField 连接器可提供的源字段（字段映射下拉用）
type SourceField struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Sample  string `json:"sample,omitempty"`
	Section string `json:"section"` // account | department | job_title | relationship | identity
}

// TargetField 可映射的目标字段（本地模型字段；字段映射表的行）
type TargetField struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Section string `json:"section"` // account | department | job_title
	// Display 界面展示用的技术名（如 user.name），仅用于渲染，不参与存储
	Display  string `json:"display,omitempty"`
	Required bool   `json:"required,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

// Capabilities 类型能力声明（决定界面展示哪些配置与动作，避免"配了不生效"）
type Capabilities struct {
	// Mapping 支持自定义字段映射（不支持的类型不渲染「字段映射」页签）
	Mapping bool `json:"mapping"`
	// Incremental 支持增量同步（按更新时间游标）。
	// 说明：连接器尚未实现增量游标 → 目前一律 false（声明为 true 而运行时读不到就是"配了不生效"）。
	Incremental bool `json:"incremental"`
	// GroupScope 支持同步组织（部门）
	GroupScope bool `json:"group_scope"`
	// SoftDelete 支持"源端缺失即停用/软删除"
	SoftDelete bool `json:"soft_delete"`
	// Test 支持连通性自检
	Test bool `json:"test"`
	// TargetDepartment 支持选择同步目标节点（部门树）
	TargetDepartment bool `json:"target_department"`
}

// Icon 图标声明：后端只下发 key，前端做唯一映射（官方素材文件或内联矢量兜底）
type Icon struct {
	Key string `json:"key"`
	// BySubType 子类型 → 图标 key（如 database 按 mysql / postgres 区分）
	BySubType map[string]string `json:"by_sub_type,omitempty"`
}

// Variant 同一实现下的接入变体（如 ldap 之下的 AD / OpenLDAP；database 之下的 MySQL / PostgreSQL）
//
// 目录里每个变体渲染成一张卡片；创建时写入 type + sub_type，图标按 sub_type 解析。
type Variant struct {
	Key         string `json:"key"` // 写入 sub_type
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// TypeInfo 一个连接类型（目录中的一类；可能带多个变体）
type TypeInfo struct {
	Key          string        `json:"key"` // 写入 type
	Direction    Direction     `json:"direction"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Vendor       string        `json:"vendor,omitempty"`
	Icon         Icon          `json:"icon"`
	AuthKind     string        `json:"auth_kind,omitempty"` // app_credentials | bind | dsn | basic | bearer
	Fields       []Field       `json:"fields"`
	Variants     []Variant     `json:"variants,omitempty"`
	Capabilities Capabilities  `json:"capabilities"`
	SourceFields []SourceField `json:"source_fields,omitempty"`
}

// Catalog 目录响应
type Catalog struct {
	Directions []DirectionCatalog `json:"directions"`
	// TargetFields 字段映射的目标字段（按 group 分组渲染）
	TargetFields []TargetField `json:"target_fields"`
}

// DirectionCatalog 单个方向的目录
type DirectionCatalog struct {
	Direction Direction  `json:"direction"`
	Types     []TypeInfo `json:"types"`
}

// ── 通用字段 ──────────────────────────────────────────────────

// nameField 连接名称（创建后不可改，进 URL 不涉及，但作为幂等键使用）
func nameField() Field {
	return Field{Key: "name", Label: "标识", Type: FieldText, Required: true, Hint: "仅小写字母、数字与中划线；创建后不可修改"}
}

func displayNameField() Field {
	return Field{Key: "display_name", Label: "名称", Type: FieldText}
}

// isEnabledField 启用开关（入方向连接）。
//
// 缺了它的后果（2026-10-22 实测）：编辑页没有开关、新建时该值为 undefined →
// 提交 `is_enabled: false` → 连接建好即停用 → 它持有的计划任务也一并停用（任务中心显示"已停用"）。
func isEnabledField() Field {
	return Field{Key: "is_enabled", Label: "启用", Type: FieldSwitch, Default: "true",
		Hint: "停用后不再自动同步，手工触发也会被拒绝；其计划任务一并停用（保留定义与历史）"}
}

// syncScopeFields 入方向通用同步范围与计划字段（所有类型共用；Store 指向 sync_config）
func syncScopeFields() []Field {
	return []Field{
		{Key: "sync_users", Label: "同步账号", Type: FieldSwitch, Default: "true"},
		{Key: "sync_groups", Label: "同步组织", Type: FieldSwitch},
		{Key: "soft_delete", Label: "源端缺失即停用", Type: FieldSwitch, Hint: "开启后，外部已删除的账号在本地被停用而不是保留"},
		// 同步目标：外部组织与账号挂到该部门节点之下（为空 = 组织根）
		{Key: "target_department_id", Label: "同步目标", Type: FieldDepartment,
			Store: "target_department_id", Hint: "导入的组织与账号挂到该节点之下；留空表示挂在组织根"},
		// 定时校验（执行窗口固定为前后 30 分钟，由服务端判定）
		{Key: "schedule_mode", Label: "定时校验", Type: FieldSelect, Default: "",
			Store: "sync_config.schedule.mode", Hint: "定时全量同步，让本地数据与源端保持一致",
			Options: []FieldOption{
				{Value: "", Label: "不自动同步"},
				{Value: "interval", Label: "每隔一段时间"},
				{Value: "daily", Label: "每天定时"},
				{Value: "cron", Label: "Cron 表达式"},
			}},
		{Key: "schedule_interval_seconds", Label: "间隔（秒）", Type: FieldNumber, Default: "3600",
			Store: "sync_config.schedule.interval_seconds", ShowWhenMode: "interval",
			Hint: "间隔模式下的同步周期，例如 3600 = 每小时"},
		{Key: "schedule_cron", Label: "Cron 表达式", Type: FieldText, Placeholder: "0 3 * * *",
			Store: "sync_config.schedule.cron", ShowWhenMode: "cron",
			Hint: "5 段：分 时 日 月 周（如 0 3 * * * 表示每天 03:00；0 9-17 * * 1-5 表示工作日每小时）"},
		{Key: "schedule_times", Label: "执行时刻", Type: FieldTags,
			Store: "sync_config.schedule.times", ShowWhenMode: "daily",
			Hint: "每天在这些时刻（如 03:00）执行；到点前后 30 分钟内只执行一次，最多 24 个"},
		// 同步保护：待停用账号达到阈值即取消本次任务（0 = 不限制）
		{Key: "guard_max_user_deletes", Label: "删除保护", Type: FieldNumber, Default: "30",
			Store: "sync_config.guard.max_user_deletes",
			Hint:  "需要停用的账号达到该数量即取消本次同步（0 = 不限制），避免误删"},
	}
}

// ── 字段映射目标字段 ──────────────────────────────────────────

// targetFields 字段映射的目标字段（**连接器真正读取的本地键**）。
//
// 键名与同步引擎实际读取的组织实体字段一一对应——界面上不能出现"配了但不生效"的字段
// （AGENTS.md §1「配置即生效」）。
// Display 仅用于界面展示（对齐参考产品的「IDaaS 字段 + 技术名」列）。
func targetFields() []TargetField {
	return []TargetField{
		{Key: "name", Display: "user.name", Label: "账号名", Section: "account", Required: true,
			Hint: "登录用的账号名，必填"},
		{Key: "displayName", Display: "user.display_name", Label: "姓名", Section: "account"},
		{Key: "email", Display: "user.email", Label: "邮箱", Section: "account"},
		{Key: "phone", Display: "user.phone", Label: "手机号", Section: "account"},
		{Key: "country_code", Display: "user.phone_region", Label: "手机区号", Section: "account",
			Hint: "手机号的国际区号，如 86"},
		{Key: "title", Display: "user.title", Label: "职位", Section: "account"},
		{Key: "location", Display: "user.location", Label: "所在地", Section: "account"},
		{Key: "id", Display: "user.external_id", Label: "外部唯一标识", Section: "account",
			Hint: "源系统里的稳定标识（留空则用账号名）"},

		// ── Organization group: fields owned by department and job_title entities ──
		// User relationships are synchronized automatically and are not entity mappings.
		{Key: "name", Display: "department.name", Label: "部门名称", Section: "department", Required: true,
			Hint: "Source field used to build the local department name and display name"},
		{Key: "name", Display: "job_title.name", Label: "职务名称", Section: "job_title",
			Hint: "Source field used to build the local job-title name; membership remains automatic"},
	}
}

// ── 各类型声明 ────────────────────────────────────────────────

// syncers 入方向类型（仅声明已注册的连接器实现）
func syncers() []TypeInfo {
	out := []TypeInfo{
		{
			Key: "dingtalk", Direction: DirectionIn, Name: "钉钉", Vendor: "Alibaba",
			Description: "绑定钉钉通讯录，实现组织与账号同步；支持钉钉扫码 / 工作台登录。",
			Icon:        Icon{Key: "dingtalk"}, AuthKind: "app_credentials",
			Fields: append([]Field{
				nameField(), displayNameField(), isEnabledField(),
				{Key: "app_id", Label: "AppKey", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "AppSecret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
			}, syncScopeFields()...),
			Capabilities: Capabilities{Mapping: true, GroupScope: true, SoftDelete: true, Test: true, TargetDepartment: true},
			SourceFields: []SourceField{
				{Key: "userid", Label: "用户 ID", Sample: "u_1001", Section: "account"},
				{Key: "unionid", Label: "UnionID", Sample: "union_xxx", Section: "account"},
				{Key: "name", Label: "员工名称", Sample: "张三", Section: "account"},
				{Key: "mobile", Label: "手机号码", Sample: "13800000000", Section: "account"},
				{Key: "state_code", Label: "国际电话区号", Sample: "86", Section: "account"},
				{Key: "telephone", Label: "分机号", Sample: "8080", Section: "account"},
				{Key: "email", Label: "个人邮箱", Sample: "zhangsan@example.com", Section: "account"},
				{Key: "org_email", Label: "企业邮箱", Sample: "zhangsan@corp.com", Section: "account"},
				{Key: "job_number", Label: "工号", Sample: "E1001", Section: "account"},
				{Key: "title", Label: "职位", Sample: "工程师", Section: "account"},
				{Key: "avatar", Label: "头像", Sample: "https://…/a.png", Section: "account"},
				{Key: "work_place", Label: "工作地", Sample: "杭州", Section: "account"},
				{Key: "dept_id_list", Label: "所属部门 ID 列表", Sample: "1,2", Section: "relationship"},
				{Key: "dept_id", Label: "部门 ID", Sample: "1", Section: "identity"},
				{Key: "name", Label: "部门名称", Sample: "技术部", Section: "department"},
				{Key: "title", Label: "职务名称", Sample: "工程师", Section: "job_title"},
			},
		},
		{
			Key: "wecom", Direction: DirectionIn, Name: "企业微信", Vendor: "Tencent",
			Description: "绑定企业微信，实现组织与账号同步；支持企微扫码登录。",
			Icon:        Icon{Key: "wecom"}, AuthKind: "app_credentials",
			Fields: append([]Field{
				nameField(), displayNameField(), isEnabledField(),
				{Key: "app_id", Label: "CorpID", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "Secret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "corp_id", Label: "CorpID（可选）", Type: FieldText},
			}, syncScopeFields()...),
			Capabilities: Capabilities{Mapping: true, GroupScope: true, SoftDelete: true, Test: true, TargetDepartment: true},
			SourceFields: []SourceField{
				{Key: "userid", Label: "成员账号", Sample: "zhangsan", Section: "account"},
				{Key: "name", Label: "成员名称", Sample: "张三", Section: "account"},
				{Key: "alias", Label: "别名", Sample: "san.zhang", Section: "account"},
				{Key: "mobile", Label: "手机号码", Sample: "13800000000", Section: "account"},
				{Key: "telephone", Label: "座机", Sample: "0571-88888888", Section: "account"},
				{Key: "email", Label: "邮箱", Sample: "zhangsan@example.com", Section: "account"},
				{Key: "title", Label: "职务", Sample: "工程师", Section: "account"},
				{Key: "avatar", Label: "头像", Sample: "https://…/a.png", Section: "account"},
				{Key: "address", Label: "地址", Sample: "杭州市西湖区", Section: "account"},
				{Key: "department", Label: "所属部门 ID 列表", Sample: "1,2", Section: "relationship"},
				{Key: "main_department", Label: "主部门 ID", Sample: "1", Section: "relationship"},
				{Key: "dept_id", Label: "部门 ID", Sample: "1", Section: "identity"},
				{Key: "name", Label: "部门名称", Sample: "技术部", Section: "department"},
				{Key: "title", Label: "职务名称", Sample: "工程师", Section: "job_title"},
			},
		},
		{
			Key: "lark", Direction: DirectionIn, Name: "飞书 / Lark", Vendor: "ByteDance",
			Description: "绑定飞书（含国际版 Lark），实现组织与账号同步；支持飞书扫码登录。",
			Icon:        Icon{Key: "feishu", BySubType: map[string]string{"lark": "lark"}}, AuthKind: "app_credentials",
			Fields: append([]Field{
				nameField(), displayNameField(), isEnabledField(),
				{Key: "app_id", Label: "App ID", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "App Secret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
			}, syncScopeFields()...),
			Variants: []Variant{
				{Key: "feishu", Name: "飞书", Description: "面向国内团队，同步飞书组织与成员。", Icon: "feishu"},
				{Key: "lark", Name: "Lark（飞书国际版）", Description: "面向海外团队，同步 Lark 组织与成员。", Icon: "lark"},
			},
			Capabilities: Capabilities{Mapping: true, GroupScope: true, SoftDelete: true, Test: true, TargetDepartment: true},
			// 账号侧源字段 = 飞书用户对象文档的**全集**（键名统一用官方字段名）。
			// 飞书是否真的返回值取决于应用的字段权限（如邮箱要「获取用户邮箱信息」、手机号要
			// 「获取用户手机号」、工号要「查看成员工号」、雇佣信息要看「获取用户受雇信息」），
			// 权限没开时同步为空值——候选照文档列全，要用哪个就去飞书补哪个权限。
			// 键集合与连接器同源，由 TestInboundSourceFieldsMatchConnectors 双向锁定。
			SourceFields: []SourceField{
				{Key: "user_id", Label: "用户 ID", Sample: "3e3cf96b", Section: "account"},
				{Key: "open_id", Label: "Open ID", Sample: "ou_xxx", Section: "account"},
				{Key: "union_id", Label: "Union ID", Sample: "on_xxx", Section: "account"},
				{Key: "name", Label: "姓名", Sample: "张三", Section: "account"},
				{Key: "en_name", Label: "英文名", Sample: "San Zhang", Section: "account"},
				{Key: "nickname", Label: "别名", Sample: "Alex Zhang", Section: "account"},
				{Key: "email", Label: "个人邮箱", Sample: "zhangsan@gmail.com", Section: "account"},
				// 企业成员的个人邮箱常为空，企业邮箱才是真正在用的那个（需管理员启用飞书邮箱服务）
				{Key: "enterprise_email", Label: "企业邮箱", Sample: "zhangsan@corp.com", Section: "account"},
				{Key: "mobile", Label: "手机号", Sample: "13011111111", Section: "account"},
				{Key: "mobile_visible", Label: "手机号是否可见", Sample: "true", Section: "account"},
				{Key: "gender", Label: "性别（0 保密 / 1 男 / 2 女）", Sample: "1", Section: "account"},
				{Key: "avatar", Label: "头像", Sample: "https://…/a.png", Section: "account"},
				{Key: "department_ids", Label: "所属部门 ID", Sample: "od_xxx,od_yyy", Section: "relationship"},
				{Key: "leader_user_id", Label: "直属上级", Sample: "ou_xxx", Section: "account"},
				{Key: "dotted_line_leader_user_ids", Label: "虚线上级 ID", Sample: "ou_xxx", Section: "account"},
				{Key: "city", Label: "工作城市", Sample: "杭州", Section: "account"},
				{Key: "country", Label: "国家/地区 Code", Sample: "CN", Section: "account"},
				{Key: "work_station", Label: "工位", Sample: "3F-08", Section: "account"},
				{Key: "join_time", Label: "入职时间（秒级时间戳）", Sample: "1600000000", Section: "account"},
				{Key: "is_tenant_manager", Label: "是否租户管理员", Sample: "true", Section: "account"},
				{Key: "employee_no", Label: "工号", Sample: "E1001", Section: "account"},
				{Key: "employee_type", Label: "员工类型（1 正式 / 2 实习 / 3 外包 / 4 劳务 / 5 顾问）", Sample: "1", Section: "account"},
				{Key: "job_title", Label: "职务", Sample: "工程师", Section: "account"},
				{Key: "job_level_id", Label: "职级 ID", Sample: "job_level_1", Section: "account"},
				{Key: "job_family_id", Label: "工作序列 ID", Sample: "job_family_1", Section: "account"},
				{Key: "description", Label: "描述", Sample: "负责…", Section: "account"},
				{Key: "is_frozen", Label: "是否冻结", Sample: "true", Section: "account"},
				{Key: "is_resigned", Label: "是否离职", Sample: "true", Section: "account"},
				{Key: "is_activated", Label: "是否激活", Sample: "true", Section: "account"},
				{Key: "is_exited", Label: "是否退出", Sample: "true", Section: "account"},
				{Key: "open_department_id", Label: "部门标识", Sample: "od_xxx", Section: "identity"},
				{Key: "name", Label: "部门名称", Sample: "技术部", Section: "department"},
				{Key: "job_title", Label: "职务名称", Sample: "工程师", Section: "job_title"},
			},
		},
		{
			Key: "ldap", Direction: DirectionIn, Name: "目录服务（LDAP / AD）", Vendor: "Generic",
			Description: "连接 Active Directory 或 OpenLDAP，导入组织与账号。",
			Icon:        Icon{Key: "ldap"}, AuthKind: "bind",
			Fields: append([]Field{
				nameField(), displayNameField(), isEnabledField(),
				{Key: "host", Label: "服务地址", Type: FieldText, Required: true, Placeholder: "ldap.example.com"},
				{Key: "port", Label: "端口", Type: FieldNumber, Default: "389", Hint: "AD 域控常见 389（明文）或 636（TLS）"},
				{Key: "user", Label: "绑定 DN", Type: FieldText, Placeholder: "cn=admin,dc=example,dc=com"},
				{Key: "password", Label: "绑定口令", Type: FieldPassword, Secret: true, HasValue: true},
				{Key: "database", Label: "Base DN", Type: FieldText, Required: true, Placeholder: "dc=example,dc=com"},
				{Key: "table", Label: "用户过滤条件", Type: FieldText, Placeholder: "(objectClass=person)"},
				{Key: "ssl_mode", Label: "连接加密", Type: FieldSelect, Default: "plain",
					Hint: "明文仅建议在内网使用；StartTLS 与 LDAPS 可避免口令走明文",
					Options: []FieldOption{
						{Value: "plain", Label: "明文（ldap://）"},
						{Value: "starttls", Label: "StartTLS（ldap:// + 升级）"},
						{Value: "ldaps", Label: "LDAPS（ldaps://，默认 636）"},
					}},
			}, syncScopeFields()...),
			Variants: []Variant{
				{Key: "ad", Name: "AD 域控", Description: "以 Active Directory 为源，导入域账号与组织。", Icon: "ad"},
				{Key: "ldap", Name: "OpenLDAP", Description: "连接自建目录服务，导入组织与账号。", Icon: "ldap"},
			},
			Capabilities: Capabilities{Mapping: true, Incremental: false, GroupScope: true, SoftDelete: true, Test: true, TargetDepartment: true},
			SourceFields: []SourceField{
				{Key: "sAMAccountName", Label: "sAMAccountName", Sample: "zhangsan", Section: "account"},
				{Key: "uid", Label: "uid", Sample: "zhangsan", Section: "account"},
				{Key: "cn", Label: "cn", Sample: "张三", Section: "account"},
				{Key: "displayName", Label: "displayName", Sample: "张三", Section: "account"},
				{Key: "mail", Label: "mail", Sample: "zhangsan@example.com", Section: "account"},
				{Key: "mobile", Label: "mobile", Sample: "13800000000", Section: "account"},
				{Key: "telephoneNumber", Label: "telephoneNumber", Sample: "0571-88888888", Section: "account"},
				{Key: "title", Label: "title（职务）", Sample: "工程师", Section: "account"},
				{Key: "l", Label: "l（所在地）", Sample: "杭州", Section: "account"},
				{Key: "memberOf", Label: "memberOf（所属组/部门）", Sample: "CN=技术部,DC=example,DC=com", Section: "relationship"},
				{Key: "cn", Label: "cn（部门名称）", Sample: "技术部", Section: "department"},
				{Key: "distinguishedName", Label: "DN", Sample: "CN=技术部,DC=example,DC=com", Section: "identity"},
				{Key: "title", Label: "title（职务名称）", Sample: "工程师", Section: "job_title"},
			},
		},
		{
			Key: "database", Direction: DirectionIn, Name: "数据库", Vendor: "Generic",
			Description: "从业务库按字段映射导入账号（只读查询）。",
			Icon:        Icon{Key: "database", BySubType: map[string]string{"mysql": "mysql", "postgres": "postgres"}}, AuthKind: "dsn",
			Fields: append([]Field{
				nameField(), displayNameField(), isEnabledField(),
				{Key: "sub_type", Label: "数据库类型", Type: FieldSelect, Required: true, Default: "postgres",
					Options: []FieldOption{{Value: "postgres", Label: "PostgreSQL"}, {Value: "mysql", Label: "MySQL"}}},
				{Key: "host", Label: "地址", Type: FieldText, Required: true},
				{Key: "port", Label: "端口", Type: FieldNumber, Default: "5432"},
				{Key: "database", Label: "库名", Type: FieldText, Required: true},
				{Key: "user", Label: "用户名", Type: FieldText, Required: true},
				{Key: "password", Label: "口令", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "table", Label: "表名", Type: FieldText, Required: true},
				{Key: "ssl_mode", Label: "SSL 模式", Type: FieldSelect, Default: "disable",
					Options:  []FieldOption{{Value: "disable", Label: "disable"}, {Value: "require", Label: "require"}, {Value: "verify-full", Label: "verify-full"}},
					ShowWhen: &Condition{Field: "sub_type", Equals: "postgres"}},
			}, syncScopeFields()...),
			Capabilities: Capabilities{Mapping: true, Incremental: true, SoftDelete: true, Test: true, TargetDepartment: true},
			SourceFields: []SourceField{
				{Key: "username", Label: "账号名列", Sample: "username", Section: "account"},
				{Key: "display_name", Label: "姓名列", Sample: "nickname", Section: "account"},
				{Key: "email", Label: "邮箱列", Sample: "email", Section: "account"},
				{Key: "phone", Label: "手机号列", Sample: "mobile", Section: "account"},
				{Key: "title", Label: "职务列", Sample: "title", Section: "account"},
				{Key: "location", Label: "所在地列", Sample: "city", Section: "account"},
				{Key: "dept_name", Label: "部门名列", Sample: "dept_name", Section: "relationship"},
				{Key: "dept_id", Label: "部门标识列", Sample: "dept_id", Section: "relationship"},
				{Key: "title", Label: "职务名列", Sample: "title", Section: "job_title"},
			},
		},
	}
	return out
}

// provisioners 出方向类型（仅声明已注册的连接器实现；随 3.4 批次逐个补齐）
func provisioners() []TypeInfo {
	out := provisionersBase()
	// 「定时对齐检查」依赖下游的存在性查询（provisioner.ExistsReporter）：
	// 不支持的类型配了计划也跑不了，因此按**实现能力**决定是否暴露计划字段（配置即生效）。
	for i := range out {
		prov, err := provisioner.Get(out[i].Key)
		if err != nil {
			continue
		}
		if _, ok := prov.(provisioner.ExistsReporter); !ok {
			continue
		}
		out[i].Fields = append(out[i].Fields, syncScopeFields()...)
	}
	return out
}

func provisionersBase() []TypeInfo {
	return []TypeInfo{
		{
			Key: "dingtalk", Direction: DirectionOut, Name: "钉钉", Vendor: "Alibaba",
			Description: "把本地账号推送到钉钉通讯录（企业内部应用，复用与入方向相同的 AppKey/AppSecret）。",
			Icon:        Icon{Key: "dingtalk"}, AuthKind: "app_credentials",
			Fields: []Field{
				nameField(), displayNameField(),
				{Key: "application_id", Label: "关联应用", Type: FieldSelect, Required: true, OptionsSource: "applications",
					Hint: "选择应用；一个应用只能绑定一个连接器"},
				{Key: "app_id", Label: "AppKey", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "AppSecret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "dept_id", Label: "默认部门 ID", Type: FieldText,
					Hint: "新建账号归入的钉钉部门 ID；留空则由钉钉归入默认部门"},
				{Key: "state", Label: "状态", Type: FieldSelect, Default: "active",
					Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}},
			},
			// 能力与实现一致：钉钉为 SSO 场景（无口令）、通讯录没有"停用态"（只能删除）
			// 出方向的字段映射归「账号策略」（应用详情 › 账号供给 › 账号策略）：
			// 连接器不重复声明，否则同一件事两处配置、且连接器级那份没人读。
			Capabilities: Capabilities{Test: true},
		},
		{
			Key: "lark", Direction: DirectionOut, Name: "飞书 / Lark", Vendor: "ByteDance",
			Description: "把本地账号推送到飞书（含国际版 Lark）通讯录。",
			Icon:        Icon{Key: "feishu"}, AuthKind: "app_credentials",
			Fields: []Field{
				nameField(), displayNameField(),
				{Key: "application_id", Label: "关联应用", Type: FieldSelect, Required: true, OptionsSource: "applications",
					Hint: "选择应用；一个应用只能绑定一个连接器"},
				{Key: "app_id", Label: "App ID", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "App Secret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "dept_id", Label: "默认部门 ID", Type: FieldText,
					Hint: "新建账号归入的部门 open_department_id；留空则由飞书决定"},
				{Key: "base_url", Label: "开放平台地址", Type: FieldText,
					Placeholder: "https://open.feishu.cn",
					Hint:        "默认飞书；国际版 Lark 填 https://open.larksuite.com"},
				{Key: "state", Label: "状态", Type: FieldSelect, Default: "active",
					Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}},
			},
			// 与实现一致：飞书账号无独立口令；通讯录没有停用态（只能删除）
			// 出方向的字段映射归「账号策略」（应用详情 › 账号供给 › 账号策略）：
			// 连接器不重复声明，否则同一件事两处配置、且连接器级那份没人读。
			Capabilities: Capabilities{Test: true},
		},
		{
			Key: "wecom", Direction: DirectionOut, Name: "企业微信", Vendor: "Tencent",
			Description: "把本地账号推送到企业微信通讯录（复用与入方向相同的 CorpID/Secret）。",
			Icon:        Icon{Key: "wecom"}, AuthKind: "app_credentials",
			Fields: []Field{
				nameField(), displayNameField(),
				{Key: "application_id", Label: "关联应用", Type: FieldSelect, Required: true, OptionsSource: "applications",
					Hint: "选择应用；一个应用只能绑定一个连接器"},
				{Key: "app_id", Label: "CorpID", Type: FieldText, Required: true},
				{Key: "app_secret", Label: "通讯录 Secret", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "dept_id", Label: "默认部门 ID", Type: FieldText, Placeholder: "1",
					Hint: "建号必须归属部门；留空归入根部门（1）"},
				{Key: "state", Label: "状态", Type: FieldSelect, Default: "active",
					Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}},
			},
			// 与实现一致：企业微信无独立口令；开放接口没有"禁用成员"（仅管理后台可操作）
			// 出方向的字段映射归「账号策略」（应用详情 › 账号供给 › 账号策略）：
			// 连接器不重复声明，否则同一件事两处配置、且连接器级那份没人读。
			Capabilities: Capabilities{Test: true},
		},
		{
			Key: "ldap", Direction: DirectionOut, Name: "目录服务（AD / OpenLDAP）", Vendor: "Generic",
			Description: "在 Active Directory 或 OpenLDAP 中创建与维护账号（支持口令重置）。",
			Icon:        Icon{Key: "ldap"}, AuthKind: "bind",
			Fields: []Field{
				nameField(), displayNameField(),
				{Key: "application_id", Label: "关联应用", Type: FieldSelect, Required: true, OptionsSource: "applications",
					Hint: "选择应用；一个应用只能绑定一个连接器"},
				{Key: "directory_type", Label: "目录类型", Type: FieldSelect, Default: "openldap",
					Hint:    "AD 支持禁用/启用账号；OpenLDAP 没有等价语义",
					Options: []FieldOption{{Value: "openldap", Label: "OpenLDAP"}, {Value: "ad", Label: "Active Directory"}}},
				{Key: "host", Label: "服务地址", Type: FieldText, Required: true},
				{Key: "port", Label: "端口", Type: FieldNumber, Default: "389", Hint: "LDAPS 通常为 636"},
				{Key: "ssl_mode", Label: "连接加密", Type: FieldSelect, Default: "plain",
					Hint: "明文仅建议内网；AD 设置口令必须使用 StartTLS 或 LDAPS",
					Options: []FieldOption{
						{Value: "plain", Label: "明文（ldap://）"},
						{Value: "starttls", Label: "StartTLS（ldap:// + 升级）"},
						{Value: "ldaps", Label: "LDAPS（ldaps://）"},
					}},
				{Key: "user", Label: "绑定 DN", Type: FieldText, Required: true, Placeholder: "cn=admin,dc=example,dc=com"},
				{Key: "password", Label: "绑定口令", Type: FieldPassword, Required: true, Secret: true, HasValue: true},
				{Key: "base_dn", Label: "账号位置", Type: FieldText, Required: true,
					Placeholder: "OU=Users,DC=example,DC=com", Hint: "新账号创建在该节点下"},
				{Key: "rdn_attr", Label: "账号 RDN 属性", Type: FieldText, Placeholder: "uid",
					Hint: "OpenLDAP 常用 uid；AD 常用 cn"},
				{Key: "state", Label: "状态", Type: FieldSelect, Default: "active",
					Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}},
			},
			// 目录是凭证权威：支持设置口令；AD 额外支持禁用/启用（由能力在运行时按 directory_type 判定）
			// 出方向的字段映射归「账号策略」（应用详情 › 账号供给 › 账号策略）：
			// 连接器不重复声明，否则同一件事两处配置、且连接器级那份没人读。
			Capabilities: Capabilities{Test: true},
		},
		{
			Key: "http", Direction: DirectionOut, Name: "通用 HTTP", Vendor: "Generic",
			Description: "按 RESTful 契约对接自有应用：建号 / 改密 / 启停 / 删除 / 自检。",
			Icon:        Icon{Key: "http"}, AuthKind: "basic",
			Fields: []Field{
				nameField(), displayNameField(),
				{Key: "application_id", Label: "关联应用", Type: FieldSelect, Required: true, OptionsSource: "applications",
					Hint: "选择应用；一个应用只能绑定一个连接器"},
				{Key: "provider_url", Label: "接口地址", Type: FieldText, Required: true, Placeholder: "https://app.example.com/api/scim"},
				{Key: "principal", Label: "调用主体", Type: FieldText},
				{Key: "credentials", Label: "凭证", Type: FieldPassword, Secret: true, HasValue: true},
				{Key: "state", Label: "状态", Type: FieldSelect, Default: "active",
					Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}},
			},
			// 出方向的字段映射归「账号策略」（应用详情 › 账号供给 › 账号策略）：
			// 连接器不重复声明，否则同一件事两处配置、且连接器级那份没人读。
			Capabilities: Capabilities{Test: true},
		},
	}
}

// Catalog 返回完整目录（前端按方向渲染）。
// 只暴露**已实现**的类型：入方向经 syncer 注册表校验、出方向经 provisioner 注册表校验。
func Build() Catalog {
	in := groupFields(filterImplemented(syncers(), func(key string) bool {
		_, err := syncer.Get(key)
		return err == nil
	}))
	out := groupFields(filterImplemented(provisioners(), func(key string) bool {
		_, err := provisioner.Get(key)
		return err == nil
	}))
	return Catalog{
		Directions: []DirectionCatalog{
			{Direction: DirectionIn, Types: in},
			{Direction: DirectionOut, Types: out},
		},
		TargetFields: targetFields(),
	}
}

// groupFields 按字段语义归组：通用信息 → basic，同步范围 → scope，其余 → connection。
// 放在一处声明，前端据此分步渲染，杜绝"同一字段在多个步骤里重复出现"。
func groupFields(types []TypeInfo) []TypeInfo {
	out := make([]TypeInfo, 0, len(types))
	for _, info := range types {
		fields := make([]Field, 0, len(info.Fields))
		for _, field := range info.Fields {
			switch {
			case field.Key == "name" || field.Key == "display_name" || field.Key == "is_enabled":
				field.Group = FieldGroupBasic
			case field.Group == FieldGroupScope: // 显式声明过的保持不动
			case field.Key == "sync_users" || field.Key == "sync_groups" || field.Key == "soft_delete" ||
				field.Key == "application" || field.Key == "state" ||
				field.Key == "target_department_id" ||
				strings.HasPrefix(field.Key, "schedule_") || strings.HasPrefix(field.Key, "guard_"):
				field.Group = FieldGroupScope
			default:
				field.Group = FieldGroupConnection
			}
			fields = append(fields, field)
		}
		info.Fields = fields
		out = append(out, info)
	}
	return out
}

// SyncerSourceFieldKeys 返回某入方向类型可提供的源字段 key 集合。
// 供提供商侧校验 match_syncer_field 是否落在该连接的源字段白名单内（配置即生效）。
func SyncerSourceFieldKeys(syncerType string) []string {
	for _, group := range Build().Directions {
		if group.Direction != DirectionIn {
			continue
		}
		for _, t := range group.Types {
			if t.Key != syncerType {
				continue
			}
			keys := make([]string, 0, len(t.SourceFields))
			for _, f := range t.SourceFields {
				keys = append(keys, f.Key)
			}
			return keys
		}
	}
	return nil
}

// KnownTypeKeys 返回目录中某方向的类型 key 集合（供服务层做"未知类型拒绝"校验）。
func KnownTypeKeys(direction Direction) []string {
	keys := make([]string, 0, 8)
	for _, group := range Build().Directions {
		if group.Direction != direction {
			continue
		}
		for _, t := range group.Types {
			keys = append(keys, t.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// Find 按方向与 key 查类型
func Find(direction Direction, key string) (TypeInfo, bool) {
	for _, group := range Build().Directions {
		if group.Direction != direction {
			continue
		}
		for _, t := range group.Types {
			if t.Key == key {
				return t, true
			}
		}
	}
	return TypeInfo{}, false
}

func filterImplemented(items []TypeInfo, ok func(string) bool) []TypeInfo {
	out := make([]TypeInfo, 0, len(items))
	for _, item := range items {
		if !ok(item.Key) {
			continue
		}
		out = append(out, item)
	}
	return out
}
