// Package importer 用户导入的文件解析（xlsx / csv）。
//
// 列清单是模板生成、表头校验与文档的唯一来源：新增可导入字段只改 columns 一处，
// 并由 schema_test.go 锁定「字段 key 与行结构一致」。
package importer

// Column 一列的可导入字段声明。
type Column struct {
	// Key 字段标识（与 Row 结构字段一一对应，也是逐行错误的 column 值）
	Key string
	// HeaderZH / HeaderEN 模板表头（中英各一份模板；上传时按所选语言逐字校验）
	HeaderZH string
	HeaderEN string
	// Required 必填列
	Required bool
	// Multi 多值列（单元格内按分隔符拆分）
	Multi bool
	// Description 模板说明页的列含义
	Description string
	// Example 模板示例行的值
	Example string
}

// columns 用户导入的固定列清单（顺序即模板列顺序）。
var columns = []Column{
	{Key: "name", HeaderZH: "账号名", HeaderEN: "username", Required: true,
		Description: "登录账号，组织内唯一；只允许字母、数字、点、下划线、连字符", Example: "zhangsan"},
	{Key: "display_name", HeaderZH: "显示姓名", HeaderEN: "display_name", Required: true,
		Description: "用户在界面上的显示名称", Example: "张三"},
	{Key: "email", HeaderZH: "邮箱", HeaderEN: "email",
		Description: "邮箱地址；已被其他用户占用时该行跳过", Example: "zhangsan@example.com"},
	{Key: "phone", HeaderZH: "手机号", HeaderEN: "phone",
		Description: "不带国家码的手机号（与“国家码”列配合）", Example: "13800000000"},
	{Key: "country_code", HeaderZH: "国家码", HeaderEN: "country_code",
		Description: "手机号的国家码，如 +86", Example: "+86"},
	{Key: "first_name", HeaderZH: "名", HeaderEN: "first_name",
		Description: "名（可选）", Example: "三"},
	{Key: "last_name", HeaderZH: "姓", HeaderEN: "last_name",
		Description: "姓（可选）", Example: "张"},
	{Key: "title", HeaderZH: "职称", HeaderEN: "title",
		Description: "职称 / 头衔（可选，自由文本）", Example: "工程师"},
	{Key: "department", HeaderZH: "部门路径", HeaderEN: "department",
		Description: "部门在组织内的完整路径，用 / 分隔，如 研发部/平台组；不存在时该行失败（开启“自动创建部门”时自动补建）", Example: "研发部/平台组"},
	{Key: "job_level", HeaderZH: "职级", HeaderEN: "job_level",
		Description: "职级名称，须已在组织内存在", Example: "P6"},
	{Key: "job_titles", HeaderZH: "职务", HeaderEN: "job_titles", Multi: true,
		Description: "职务名称，多个用分隔符分隔；须已在组织内存在", Example: "前端负责人"},
	{Key: "roles", HeaderZH: "角色", HeaderEN: "roles", Multi: true,
		Description: "角色名称，多个用分隔符分隔；须已在组织内存在。导入角色需要额外的角色管理权限", Example: "开发者"},
	{Key: "password", HeaderZH: "初始密码", HeaderEN: "password", Required: true,
		Description: "逐行初始密码，导入时按组织密码策略校验；不会出现在任何导出或记录中", Example: "Passw0rd!2026"},
}

// Columns 返回列清单的副本（调用方不得修改内置声明）。
func Columns() []Column {
	out := make([]Column, len(columns))
	copy(out, columns)
	return out
}

// ColumnByKey 按字段标识取列声明。
func ColumnByKey(key string) (Column, bool) {
	for _, c := range columns {
		if c.Key == key {
			return c, true
		}
	}
	return Column{}, false
}
