package repository

// 内置（种子）资源标识：由 app/seed.go 创建，属于系统自身，默认不可删除。
const (
	// BuiltInInstitution 内置组织（多租户的兜底组织）
	BuiltInInstitution = "built-in"
	// BuiltInAdminUser 内置组织下的系统管理员账号
	BuiltInAdminUser = "admin"
)

// IsBuiltInInstitution 判断是否为内置组织。
func IsBuiltInInstitution(name string) bool { return name == BuiltInInstitution }

// IsBuiltInAdmin 判断是否为内置系统管理员账号（内置组织下的 admin）。
func IsBuiltInAdmin(institution, name string) bool {
	return institution == BuiltInInstitution && name == BuiltInAdminUser
}
