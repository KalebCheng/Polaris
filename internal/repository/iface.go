package repository

import (
	"context"
	"time"
)

// ListQuery 通用分页/过滤参数。
type ListQuery struct {
	// InstitutionID 租户过滤（0 = 不限租户；平台级资源用 0）
	InstitutionID int64
	Keyword       string
	Field         string
	Value         string
	Offset        int
	Limit         int
	// DepartmentID 非零时只返回「多值归属包含该部门」的用户。
	DepartmentID int64
	// Filters 附加的等值过滤（AND 语义）。
	// Value 必须是**与列类型匹配**的 Go 类型（布尔列传 bool、数字列传 int64、文本列传 string），
	// 否则 PostgreSQL 会因 "boolean = text" 这类类型冲突直接报错（应用层不做隐式转换）。
	// 说明：users / account_strategies / application_accounts / provision_history 仓库读取该字段；
	// 其它仓库保持既有 Field/Value 语义不变。
	Filters []ColumnFilter
}

// ColumnFilter 单个等值过滤条件。
type ColumnFilter struct {
	Field string
	Value interface{}
}

// PermissionBinding 用户有效权限的来源标注（权限名 + 来源标识，如 user / role:admin / group:ops）。
type PermissionBinding struct {
	Permission string
	Source     string
}

// InstitutionRepository 租户仓库接口
// 存储: PostgreSQL（institutions）
type InstitutionRepository interface {
	Create(ctx context.Context, inst *Institution) (int64, error)
	GetByID(ctx context.Context, id int64) (*Institution, error)
	GetByName(ctx context.Context, name string) (*Institution, error)
	// GetByDomain 按域名查询（租户解析；空域名不匹配）
	GetByDomain(ctx context.Context, domain string) (*Institution, error)
	Update(ctx context.Context, inst *Institution) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Institution, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
}

// StorageSettingsRepository 系统级默认存储配置仓库接口（单行）
// 存储: PostgreSQL（storage_settings）
type StorageSettingsRepository interface {
	// Get 读取单行配置（不存在时返回零值实体，不报错）
	Get(ctx context.Context) (*StorageSettings, error)
	Update(ctx context.Context, s *StorageSettings) error
}

// DepartmentRepository 部门仓库接口（租户内的部门树节点）
// 存储: PostgreSQL（departments）
type DepartmentRepository interface {
	Create(ctx context.Context, d *Department) (int64, error)
	GetByID(ctx context.Context, id int64) (*Department, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Department, error)
	Update(ctx context.Context, d *Department) error
	Delete(ctx context.Context, institutionID int64, name string) error
	List(ctx context.Context, q ListQuery) ([]*Department, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// GetRoot 返回组织根部门（Spec Q4.5：每个组织恰好一行 is_root = TRUE）
	GetRoot(ctx context.Context, institutionID int64) (*Department, error)
	// SetSortIndex 仅更新排序序号（同层内拖拽/上下移动用）
	SetSortIndex(ctx context.Context, institutionID int64, name string, sortIndex int) error
	// RecomputeParentIDChains 按当前 parent_id 重算某组织的全部祖先链（同步器批量改层级后调用）
	RecomputeParentIDChains(ctx context.Context, institutionID int64) error

	// ExistingIDs returns department IDs that belong to the institution.
	ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error)
	// Children 返回某节点的直接子部门（parentID 为 nil 时取顶层）与该层总数（应用授权弹窗的部门树按层加载）
	Children(ctx context.Context, institutionID int64, parentID *int64, offset, limit int) ([]*Department, int64, error)
}

// UserRepository 用户仓库接口
// 存储: PostgreSQL（users / user_identities / password_history）
type UserRepository interface {
	Create(ctx context.Context, u *User) (int64, error)
	// CreateWithDepartments 事务内创建用户并写入多值部门归属（主部门必须包含在 departmentIDs 中）。
	// user_departments 是部门归属与主部门的唯一真相。
	CreateWithDepartments(ctx context.Context, u *User, departmentIDs []int64, primaryID int64) (int64, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	// GetByUID 按全局唯一 UID 查询（标准 token 的 sub 即 UID，userinfo 不依赖业务 claim）
	GetByUID(ctx context.Context, uid string) (*User, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*User, error)
	GetByEmail(ctx context.Context, institutionID int64, email string) (*User, error)
	// FindMatchCandidates 按提供商映射的本地字段查候选用户（最多 limit 条，用于唯一性判定）。
	// field supports username | email | phone for local user columns.
	FindMatchCandidates(ctx context.Context, institutionID int64, field, value string, limit int) ([]*User, error)
	// FindIdentityLinkMatchCandidates matches only links imported by the selected inbound syncer.
	// field 为连接源字段名（external_id 或连接器 source_fields 之一）。
	FindIdentityLinkMatchCandidates(
		ctx context.Context, institutionID, syncerID int64, field, value string, limit int,
	) ([]*User, error)
	// FindUsersByIdentityAttribute 按提供商的原始身份字段（如 unionid）反向认领本地用户。
	FindUsersByIdentityAttribute(
		ctx context.Context, institutionID, providerID int64, attribute, value string, limit int,
	) ([]*User, error)
	// FindByNameAcrossInstitutions 跨组织按账号名查找（登录时自动解析组织；limit 用于识别歧义）
	FindByNameAcrossInstitutions(ctx context.Context, name string, limit int) ([]*User, error)
	// FindByEmailAcrossInstitutions 跨组织按邮箱查找
	FindByEmailAcrossInstitutions(ctx context.Context, email string, limit int) ([]*User, error)
	// FindByPhoneAcrossInstitutions 跨组织按手机号查找
	FindByPhoneAcrossInstitutions(ctx context.Context, phone string, limit int) ([]*User, error)
	Update(ctx context.Context, u *User) error
	// UpdatePassword 更新密码哈希并写入密码历史（红线 §3.1-6：多表写入使用事务）
	UpdatePassword(ctx context.Context, u *User, historyLimit int) error
	// UpdateInitialPassword 仅在用户当前无密码时写入初始密码（条件更新，避免并发下绕过旧密码校验）
	UpdateInitialPassword(ctx context.Context, u *User, historyLimit int) error
	SoftDelete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*User, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// ListByNames 按账号名批量取用户（用户组详情、部门成员校验用；只返回本组织内的）
	ListByNames(ctx context.Context, institutionID int64, names []string) ([]*User, error)
	// ListByIDs returns users by primary key within the institution.
	ListByIDs(ctx context.Context, institutionID int64, ids []int64) ([]*User, error)
	// ExistingIDs returns active user IDs that belong to the institution.
	ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error)
	UpdateLastSignin(ctx context.Context, id int64, at time.Time, ip string) error
	SetOnline(ctx context.Context, id int64, online bool) error
	// PasswordHistory 读取最近 N 条密码哈希（用于组织密码策略校验）
	PasswordHistory(ctx context.Context, institutionID int64, user string, limit int) ([]string, error)
	// CountAdmins 统计组织内**仍可登录**的管理员数量（"最后一个管理员"防呆用：排除已删除/已禁用）
	CountAdmins(ctx context.Context, institutionID int64) (int64, error)

	// 外部身份绑定
	GetIdentity(ctx context.Context, institutionID int64, provider, providerUserID string) (*UserIdentity, error)
	GetIdentitiesByUser(ctx context.Context, institutionID int64, user string) ([]*UserIdentity, error)
	// CreateWithIdentity 事务内创建用户并绑定外部身份（JIT 供给；红线 §3.1-6）
	CreateWithIdentity(ctx context.Context, u *User, identity *UserIdentity) (int64, error)
	BindIdentity(ctx context.Context, identity *UserIdentity) (int64, error)
	UnbindIdentity(ctx context.Context, id int64) error
}

// ApplicationRepository 应用仓库接口
// 存储: PostgreSQL（applications）
type ApplicationRepository interface {
	Create(ctx context.Context, app *Application) (int64, error)
	GetByID(ctx context.Context, id int64) (*Application, error)
	GetByCode(ctx context.Context, institutionID int64, code string) (*Application, error)
	GetByClientID(ctx context.Context, clientID string) (*Application, error)
	Update(ctx context.Context, app *Application) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Application, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
}

// ApplicationSubjectRepository 应用授权主体仓库接口（应用准入名单）
// 存储: PostgreSQL（application_subjects）
type ApplicationSubjectRepository interface {
	// Replace 整体替换某应用的访问授权配置（单事务：applications.access_mode + application_subjects）
	// mode 为 all | restricted（调用方已校验）；空名单合法（= 暂时无人可访问）
	Replace(ctx context.Context, app *Application, mode string, subjects []ApplicationSubject) error
	// List 列出某应用的授权主体（按类型 + 标识名排序）
	List(ctx context.Context, applicationID int64) ([]ApplicationSubject, error)
	// CountByApplications 批量统计各应用的授权主体数（应用列表「访问范围」列，避免 N 次查询）
	CountByApplications(ctx context.Context, applicationIDs []int64) (map[int64]int, error)
}

// ProviderTemplateRepository 邮件/短信提供商的场景模板仓库接口
// 存储: PostgreSQL（provider_email_templates / provider_sms_templates）
type ProviderTemplateRepository interface {
	Create(ctx context.Context, t *ProviderTemplate) (int64, error)
	GetByID(ctx context.Context, channel string, id int64) (*ProviderTemplate, error)
	ListByProvider(ctx context.Context, channel string, institutionID, providerID int64, scene string) ([]*ProviderTemplate, error)
	Update(ctx context.Context, t *ProviderTemplate) error
	Delete(ctx context.Context, channel string, id int64) error
}

// UserMFACredentialRepository 用户 MFA 凭证仓库接口
// 存储: PostgreSQL（user_mfa_credentials）
type UserMFACredentialRepository interface {
	Get(ctx context.Context, institutionID, userID, providerID int64) (*UserMFACredential, error)
	ListByUser(ctx context.Context, institutionID, userID int64) ([]*UserMFACredential, error)
	Save(ctx context.Context, c *UserMFACredential) (int64, error)
	Update(ctx context.Context, c *UserMFACredential) error
	Delete(ctx context.Context, institutionID, userID, providerID int64) error
}

// ProviderRepository 提供方仓库接口
// 存储: PostgreSQL（providers）
type ProviderRepository interface {
	Create(ctx context.Context, p *Provider) (int64, error)
	GetByID(ctx context.Context, id int64) (*Provider, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Provider, error)
	Update(ctx context.Context, p *Provider) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Provider, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// ListIdentityLinkProviders 返回绑定到该入方向连接、启用身份链接匹配的提供商
	// （"同源身份连接"反向认领用；只取启用项）。
	ListIdentityLinkProviders(ctx context.Context, institutionID, syncerID int64) ([]*Provider, error)
}

// RoleRepository 角色仓库接口
// 存储: PostgreSQL（roles / role_users / role_job_titles / role_roles / role_permissions）
type RoleRepository interface {
	Create(ctx context.Context, r *Role) (int64, error)
	GetByID(ctx context.Context, id int64) (*Role, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Role, error)
	Update(ctx context.Context, r *Role) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Role, error)
	Count(ctx context.Context, q ListQuery) (int64, error)

	// 角色成员与授权（整体替换，事务内完成）
	SetMembers(ctx context.Context, roleID int64, relations *RoleRelations) error
	// AffectedUsersForRoles returns direct, job-title, and ancestor-role users for the given roles.
	AffectedUsersForRoles(ctx context.Context, institutionID int64, roleNames []string) ([]string, error)
	// AffectedUsersForPermission returns all users whose effective authorization includes the permission.
	AffectedUsersForPermission(ctx context.Context, institutionID int64, permissionName string) ([]string, error)
	// RolesForUser 返回用户直接归属的角色名（含经用户组间接归属）
	RolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error)
	// PermissionsForUser 返回用户经角色/用户组/直接绑定获得的权限名（有效权限集合）
	PermissionsForUser(ctx context.Context, institutionID int64, user string) ([]*Permission, error)
	// DirectRolesForUser 返回用户**直接**归属的角色名（不含经用户组/嵌套角色继承）
	DirectRolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error)
	// DirectRoleIDsForUser returns the user's direct role primary keys.
	DirectRoleIDsForUser(ctx context.Context, institutionID, userID int64) ([]int64, error)
	// SetDirectRoleIDs replaces the user's direct roles atomically.
	SetDirectRoleIDs(ctx context.Context, institutionID, userID int64, roleIDs []int64) error
	// PermissionBindingsForUser 返回有效权限的"来源"明细（直接 / 角色 X / 职务 Y）
	PermissionBindingsForUser(ctx context.Context, institutionID int64, user string) ([]PermissionBinding, error)
	// RolesForJobTitle 返回职务关联的角色名（role_job_titles）
	RolesForJobTitle(ctx context.Context, institutionID int64, jobTitle string) ([]string, error)
	// PermissionsForJobTitle 返回职务经角色获得的权限（去重）
	PermissionsForJobTitle(ctx context.Context, institutionID int64, jobTitle string) ([]*Permission, error)
}

// RoleRelations 角色的成员/子角色/权限关系
type RoleRelations struct {
	UserIDs       []int64 `json:"users"`
	JobTitleIDs   []int64 `json:"job_titles"`
	RoleIDs       []int64 `json:"roles"`
	PermissionIDs []int64 `json:"permissions"`
}

// PermissionRepository 权限仓库接口
// 存储: PostgreSQL（permissions / permission_users / permission_job_titles / permission_roles）
type PermissionRepository interface {
	Create(ctx context.Context, p *Permission) (int64, error)
	GetByID(ctx context.Context, id int64) (*Permission, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Permission, error)
	Update(ctx context.Context, p *Permission) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Permission, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	SetSubjects(ctx context.Context, permissionID int64, userIDs, jobTitleIDs, roleIDs []int64) error
}

// JobTitleRepository 职务仓库接口（Spec 第六阶段 Q3 = A：原「用户组」，扁平无层级）
// 存储: PostgreSQL（job_titles / job_title_members）
type JobTitleRepository interface {
	Create(ctx context.Context, t *JobTitle) (int64, error)
	GetByID(ctx context.Context, id int64) (*JobTitle, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*JobTitle, error)
	Update(ctx context.Context, t *JobTitle) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*JobTitle, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// SetMemberIDs replaces job-title membership by user primary key.
	SetMemberIDs(ctx context.Context, jobTitleID int64, userIDs []int64) error
	// AddMembers 追加任职人员（幂等；只增不删——同步派生的职务不得清掉管理员手工维护的关系）
	AddMembers(ctx context.Context, jobTitleID int64, users []string) error
	// JobTitlesForUser 返回用户任职的职务标识
	JobTitlesForUser(ctx context.Context, institutionID int64, user string) ([]string, error)
	// JobTitleIDsForUser returns the user's job-title primary keys.
	JobTitleIDsForUser(ctx context.Context, institutionID, userID int64) ([]int64, error)
	// SetJobTitleIDsForUser replaces a user's job-title assignments by primary key.
	SetJobTitleIDsForUser(ctx context.Context, institutionID, userID int64, jobTitleIDs []int64) error
	// MemberCounts 返回组织内各职务的任职人数（列表页展示，避免 N 次查询）
	MemberCounts(ctx context.Context, institutionID int64) (map[string]int, error)
	// MemberIDsByJobTitleIDs returns distinct user IDs assigned to the job titles.
	MemberIDsByJobTitleIDs(ctx context.Context, institutionID int64, jobTitleIDs []int64) ([]int64, error)
	// ExistingIDs returns job-title IDs that belong to the institution.
	ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error)
}

// JobLevelRepository 职级仓库接口（Spec 第六阶段 Q4）
// 存储: PostgreSQL（job_levels；用户侧经 users.job_level_id 单值引用）
type JobLevelRepository interface {
	Create(ctx context.Context, l *JobLevel) (int64, error)
	GetByID(ctx context.Context, id int64) (*JobLevel, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*JobLevel, error)
	Update(ctx context.Context, l *JobLevel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*JobLevel, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// CountByLevel 返回组织内各职级的用户数（组织概览的职级分布）
	CountByLevel(ctx context.Context, institutionID int64) (map[int64]int, error)
}

// UserDepartmentRepository 用户 ↔ 部门多值归属仓库接口（Spec 第六阶段 Q2 = A）
// 存储: PostgreSQL（user_departments）
type UserDepartmentRepository interface {
	// ReplaceForUser 整体替换某用户的部门归属；syncerID 非空时只替换"该连接写入的归属"
	// （管理员手工维护的归属不被同步覆盖）。
	ReplaceForUser(ctx context.Context, institutionID, userID int64, departmentIDs []int64, primaryID int64, syncerID *int64) error
	// DepartmentIDsForUser 返回用户的全部部门归属（主部门优先）
	DepartmentIDsForUser(ctx context.Context, userID int64) ([]int64, error)
	// DepartmentIDsForUsers 批量返回多个用户的部门归属（user_id → department_id；主部门优先），
	// 供用户列表一次补全多部门展示，避免逐行查询。
	DepartmentIDsForUsers(ctx context.Context, institutionID int64, userIDs []int64) (map[int64][]int64, error)
	// ListForUser 返回用户的部门归属明细（含主部门标记与来源连接）
	ListForUser(ctx context.Context, userID int64) ([]*UserDepartment, error)
	// ListBySyncer 返回"某连接写入的全部归属"（user_id → department_id 列表），
	// 供同步引擎一次读入、避免逐账号查询
	ListBySyncer(ctx context.Context, syncerID int64) (map[int64][]int64, error)
	// UserIDsOfDepartments 返回"这些部门 + 其全部子部门"内的用户 id（部门保留层级语义）
	UserIDsOfDepartments(ctx context.Context, institutionID int64, departmentIDs []int64) ([]int64, error)
}

// SyncerLinkRepository 外部身份映射仓库接口（Spec 第六阶段 FR-1）
// 存储: PostgreSQL（syncer_user_links / syncer_department_links / syncer_job_title_links / syncer_job_level_links）
// kind 取值：user | department | job_title | job_level
type SyncerLinkRepository interface {
	// LoadExternalIndex 返回「源端标识 → 本地对象 ID」索引（单连接内）
	LoadExternalIndex(ctx context.Context, kind string, syncerID int64) (map[string]int64, error)
	// Upsert 写入/更新一条映射
	Upsert(ctx context.Context, kind string, institutionID, syncerID, localID int64, externalID string, raw []byte) error
	// Prune 清理本连接中"源端已不存在"的映射行，返回被清理的本地对象 ID
	Prune(ctx context.Context, kind string, syncerID int64, externalIDs []string) ([]int64, error)
	// ListForLocal 返回某本地对象的外部身份明细（详情页展示源端报文）
	ListForLocal(ctx context.Context, kind string, localID int64) ([]*ExternalIdentityRef, error)
}

// CertRepository 证书仓库接口
// 存储: PostgreSQL（certs）
type CertRepository interface {
	Create(ctx context.Context, c *Cert) (int64, error)
	GetByID(ctx context.Context, id int64) (*Cert, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Cert, error)
	Update(ctx context.Context, c *Cert) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Cert, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
}

// TokenRepository 令牌仓库接口
// 存储: PostgreSQL（tokens）
type TokenRepository interface {
	Create(ctx context.Context, t *Token) (int64, error)
	GetByID(ctx context.Context, id int64) (*Token, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Token, error)
	GetByCode(ctx context.Context, applicationID int64, code string) (*Token, error)
	GetByAccessToken(ctx context.Context, accessToken string) (*Token, error)
	GetByRefreshToken(ctx context.Context, applicationID int64, refreshToken string) (*Token, error)
	// ConsumeCode 原子标记授权码已使用（返回 false 表示已被消费/不存在）
	ConsumeCode(ctx context.Context, applicationID int64, code string) (bool, error)
	Update(ctx context.Context, t *Token) error
	// RotateRefreshToken revokes oldID and stores replacement in one transaction.
	RotateRefreshToken(ctx context.Context, oldID int64, replacement *Token) error
	// RevokeBySession revokes every active token bound to a Polaris session.
	RevokeBySession(ctx context.Context, sessionID string) (int64, error)
	Revoke(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Token, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*Token, error)
	DeleteBatch(ctx context.Context, ids []int64) error
}

// RecordRepository 审计记录仓库接口
// 存储: PostgreSQL（records）
type RecordRepository interface {
	Create(ctx context.Context, r *Record) (int64, error)
	GetByID(ctx context.Context, id int64) (*Record, error)
	List(ctx context.Context, q ListQuery) ([]*Record, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
}

// SessionRepository 会话仓库接口
// 存储: Redis
type SessionRepository interface {
	Create(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) ([]byte, error)
	Update(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error
	Delete(ctx context.Context, sessionID string) error
	Exists(ctx context.Context, sessionID string) (bool, error)
	TTL(ctx context.Context, sessionID string) (time.Duration, error)
	AddUserSession(ctx context.Context, institutionID int64, user, sessionID string) error
	RemoveUserSession(ctx context.Context, institutionID int64, user, sessionID string) error
	UserSessions(ctx context.Context, institutionID int64, user string) ([]string, error)
	DeleteUserSessions(ctx context.Context, institutionID int64, user string) ([]string, error)
	// ListSessionIDs 返回全部会话 ID（跨组织，供平台级「会话」视图使用）
	ListSessionIDs(ctx context.Context) ([]string, error)
}

// RateLimitRepository 限流仓库接口
// 存储: Redis
type RateLimitRepository interface {
	// Incr 自增计数；首次写入时设置窗口 TTL。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Get(ctx context.Context, key string) (int64, error)
	Reset(ctx context.Context, key string) error
	// Expire 重置已有键的存活时间（用于"达到上限后自此刻开始冻结"）。
	Expire(ctx context.Context, key string, ttl time.Duration) error
	// TTL 读取键的剩余存活时间（键不存在返回 0）
	TTL(ctx context.Context, key string) (time.Duration, error)
	// Keys 按 glob 模式枚举键（SCAN 实现，用于解除登录失败锁定）。
	// 注意：SCAN 的 MATCH 是 glob，调用方拿到键后必须自行做**精确比对**（见 service/unlock.go）。
	Keys(ctx context.Context, pattern string) ([]string, error)
}

// StateStore OAuth/SSO state 暂存接口
// 存储: Redis
type StateStore interface {
	Save(ctx context.Context, state string, payload []byte, ttl time.Duration) error
	Take(ctx context.Context, state string) ([]byte, error) // 取出并删除（一次性）
}

// MagicLinkStore 魔法链接一次性令牌暂存接口
// 存储: Redis（polaris:magic:*），读取即删除（一次性消费，防重放）
type MagicLinkStore interface {
	Save(ctx context.Context, token string, payload []byte, ttl time.Duration) error
	Take(ctx context.Context, token string) ([]byte, error)
}

// CodeStore 验证码（邮件/短信）暂存接口
// 存储: Redis（一次性消费 + TTL）
type CodeStore interface {
	Save(ctx context.Context, key, value string, ttl time.Duration) error
	// Take 取出并删除（原子），不存在返回空串与 nil
	Take(ctx context.Context, key string) (string, error)
}

// SyncerRepository 外部用户数据源（同步器）仓库接口
// 存储: PostgreSQL（syncers / syncer_runs）
type SyncerRepository interface {
	Create(ctx context.Context, s *Syncer) (int64, error)
	GetByID(ctx context.Context, id int64) (*Syncer, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Syncer, error)
	Update(ctx context.Context, s *Syncer) error
	// SetScheduleJobID 回填本连接持有的 asyncd 任务主键（保存连接后单独一步，由对账兜底）
	SetScheduleJobID(ctx context.Context, id int64, jobID int64) error
	// SetEnabled 只改启用态（不触碰配置列：全量 round-trip 会丢掉目录未声明的 config 键）
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	// ListByScheduleJobIDs 按任务主键批量反查持有它们的连接（任务中心的"来源"展示用）
	ListByScheduleJobIDs(ctx context.Context, jobIDs []int64) ([]*Syncer, error)
	// ListScheduleJobNames 列出本连接持有的任务定义名（启动退役时保护它们不被误伤）
	ListScheduleJobNames(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Syncer, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	ListEnabled(ctx context.Context) ([]*Syncer, error)
	// UpdateRunResult 写回最近一次同步状态（不触碰业务配置字段）
	UpdateRunResult(ctx context.Context, id int64, status, message string, at time.Time, counts SyncCounts) error
	// CreateRun 记录一次同步明细
	CreateRun(ctx context.Context, run *SyncerRun) (int64, error)
	ListRuns(ctx context.Context, syncerID int64, offset, limit int) ([]*SyncerRun, error)
}

// SyncCounts 一次同步的统计
type SyncCounts struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Deleted int `json:"deleted"`
}

// ConnectorRepository 下游应用连接器仓库接口
// 存储: PostgreSQL（connectors）
type ConnectorRepository interface {
	Create(ctx context.Context, c *Connector) (int64, error)
	GetByID(ctx context.Context, id int64) (*Connector, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*Connector, error)
	Update(ctx context.Context, c *Connector) error
	// SetScheduleJobID 回填本连接器持有的 asyncd 任务主键（保存连接器后单独一步，由对账兜底）
	SetScheduleJobID(ctx context.Context, id int64, jobID int64) error
	// ListByScheduleJobIDs 按任务主键批量反查持有它们的连接器（任务中心的"来源"展示用）
	ListByScheduleJobIDs(ctx context.Context, jobIDs []int64) ([]*Connector, error)
	// ListScheduleJobNames 列出本连接器持有的任务定义名（启动退役时保护它们不被误伤）
	ListScheduleJobNames(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*Connector, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// ListEnabled 返回某应用（主键）下启用的连接器（applicationID 为 0 = 租户内全部启用连接器）
	ListEnabled(ctx context.Context, institutionID int64, applicationID int64) ([]*Connector, error)
	// UpdateRunResult 写回最近一次供给状态（不触碰业务配置字段）
	UpdateRunResult(ctx context.Context, id int64, status, message string, at time.Time) error
}

// AccountStrategyRepository 账号策略仓库接口
// 存储: PostgreSQL（account_strategies）
type AccountStrategyRepository interface {
	Create(ctx context.Context, a *AccountStrategy) (int64, error)
	GetByID(ctx context.Context, id int64) (*AccountStrategy, error)
	GetByName(ctx context.Context, institutionID int64, name string) (*AccountStrategy, error)
	Update(ctx context.Context, a *AccountStrategy) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*AccountStrategy, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
	// FindForApplication 返回指定应用（主键）启用的策略（应用专属优先，其次租户默认；applicationID 为 0 = 只看默认）
	FindForApplication(ctx context.Context, institutionID int64, applicationID int64) (*AccountStrategy, error)
}

// ApplicationAccountRepository 应用账号仓库接口
// 存储: PostgreSQL（application_accounts）；幂等键 (institution_id, application_id, user_id)
type ApplicationAccountRepository interface {
	Create(ctx context.Context, a *ApplicationAccount) (int64, error)
	GetByID(ctx context.Context, id int64) (*ApplicationAccount, error)
	// GetByUserApp 按幂等键查询（主键；不存在返回错误）
	GetByUserApp(ctx context.Context, institutionID int64, applicationID, userID int64) (*ApplicationAccount, error)
	Update(ctx context.Context, a *ApplicationAccount) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) ([]*ApplicationAccount, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
}

// ProvisionHistoryRepository 供给历史仓库接口
// 存储: PostgreSQL（provision_history）
type ProvisionHistoryRepository interface {
	Create(ctx context.Context, h *ProvisionHistory) (int64, error)
	List(ctx context.Context, q ListQuery) ([]*ProvisionHistory, error)
	Count(ctx context.Context, q ListQuery) (int64, error)
}

// ── asyncd：任务定义 / 执行实例 / 执行记录 / 执行器实例 ───────────────────────────

// AsyncdJobRunQuery 执行实例查询过滤（任务中心：按归属 + 状态取执行记录）。
type AsyncdJobRunQuery struct {
	// JobID 归属（0 = 全部任务）
	JobID int64
	// Status queued | running | succeeded | dead（空 = 全部）
	Status string
}

// AsyncdJobRunStat 执行统计的一行（按任务 × 状态聚合）。
type AsyncdJobRunStat struct {
	JobID int64
	// Status queued | running | succeeded | dead
	Status string
	Count  int
	// LastQueuedAt 该任务最近一次执行实例的入队时间
	LastQueuedAt time.Time
}

// AsyncdWorkerStat 执行器实例 + 实时在跑任务数（在跑数由 runs 聚合得出，不存列）。
type AsyncdWorkerStat struct {
	Worker AsyncdWorker
	// Running 该槽位当前持租约在执行的任务数
	Running int
}

// AsyncdJobRepository 任务定义仓库接口（存储: PostgreSQL asyncd_jobs）
//
// asyncd 是通用调度模块：本接口**不认识任何业务概念**（syncer / connector / 应用供给……）。
// 定义有两类来源，差别只在写入语义：
//   - 代码声明：启动时 upsert，不覆盖运维在界面上的启停决定；
//   - 业务持有：由持有方（如身份连接）保存/对账时写入，规则与启用态以持有方为准。
//
// "谁持有这个任务"由业务侧自己记录（如 syncers.schedule_job_id），asyncd 不存归属列。
type AsyncdJobRepository interface {
	// Upsert 注册/更新**代码声明**的任务定义（启动时对齐，幂等；next_run_at 与 enabled 仅在新建时初始化）
	Upsert(ctx context.Context, job *AsyncdJob) error
	// UpsertJob 写入**业务持有**的任务定义（覆盖规则/payload/enabled/unique_key，返回任务主键；
	// resetNextRunAt=true 时按新规则重置 next_run_at，避免"改了计划要等旧的下次时间才生效"）
	UpsertJob(ctx context.Context, job *AsyncdJob, resetNextRunAt bool) (int64, error)
	// SetEnabledByName 按任务名启停（业务持有方的计划变化/停用时调用）
	SetEnabledByName(ctx context.Context, name string, enabled bool) error
	// RetireUndeclared 把"本次启动未在有效名单里"的定义标记退役（保留历史，不再被调度）。
	// activeNames 由调用方组合：代码声明 ∪ 业务持有（业务侧提供自己的名单，asyncd 不反查业务表）。
	RetireUndeclared(ctx context.Context, activeNames []string, at time.Time) (int64, error)
	// GetByName 按定义名取（入队时解析处理器与默认策略）
	GetByName(ctx context.Context, name string) (*AsyncdJob, error)
	// GetByID 按主键取（对外路径统一用主键，见 handler/util.go 的约定）
	GetByID(ctx context.Context, id int64) (*AsyncdJob, error)
	// ClaimDue 原子推进到点的定时任务并返回（多实例下同一时刻只有一个实例拿到）
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]*AsyncdJob, error)
	// MarkRun 记录本次调度结果（queued|skipped|failed）并推进 next_run_at
	MarkRun(ctx context.Context, id int64, status string, nextRunAt time.Time, at time.Time) error
	// List 列出任务定义（includeRetired=true 时含已退役的，供执行记录的归属解析）
	List(ctx context.Context, includeRetired bool) ([]*AsyncdJob, error)
	// SetEnabled 启停定时任务（异步任务不可停用；按主键）
	SetEnabled(ctx context.Context, id int64, enabled bool) error
}

// AsyncdJobRunRepository 执行实例仓库接口（存储: PostgreSQL asyncd_job_runs）
//
// 契约：Enqueue 必须能在调用方的业务事务里执行（outbox）；Claim 用 SKIP LOCKED 领取，
// 保证多实例并发下同一任务只会被一个执行器槽位拿到。
type AsyncdJobRunRepository interface {
	// Enqueue 入队；unique_key 非空且已有 queued/running 实例时返回既有实例（existing=true）
	Enqueue(ctx context.Context, run *AsyncdJobRun) (id int64, existing bool, err error)
	// Claim 领取到期实例（多实例安全）；leaseTTL 为租约时长
	Claim(ctx context.Context, workerID string, leaseTTL time.Duration, limit int) ([]*AsyncdJobRun, error)
	// RenewLease 续租（心跳）；返回 false 表示租约已失效（实例已被回收，需停止执行）
	RenewLease(ctx context.Context, id int64, workerID string, leaseTTL time.Duration) (bool, error)
	// Complete 标记成功
	Complete(ctx context.Context, id int64, workerID string) error
	// Retry 记录失败并安排下次重试；attempts >= max_attempts 时移入死信（dead）
	Retry(ctx context.Context, id int64, workerID, cause string, nextAttemptAt time.Time) error
	// Kill 直接入死信（如超期/不可重试错误）
	Kill(ctx context.Context, id int64, workerID, cause string) error
	// ReclaimExpired 回收租约过期的实例（执行器崩溃恢复）；返回被回收的实例 id
	ReclaimExpired(ctx context.Context, limit int) ([]int64, error)
	// Counts 按状态计数（可同时按归属过滤）
	Counts(ctx context.Context, query AsyncdJobRunQuery) (map[string]int, error)
	// List 列出执行实例（执行记录列表）
	List(ctx context.Context, query AsyncdJobRunQuery, offset, limit int) ([]*AsyncdJobRun, error)
	// GetByID 按主键取（人工重试前校验状态、尝试明细按归属鉴权）
	GetByID(ctx context.Context, id int64) (*AsyncdJobRun, error)
	// Stats 按任务 × 状态聚合执行计数（任务目录用）
	Stats(ctx context.Context) ([]AsyncdJobRunStat, error)
	// RetryDead 人工重试死信实例：回到 queued 并清零尝试计数
	RetryDead(ctx context.Context, id int64) error
	// PurgeFinishedBefore 清理终态且完成时间早于 before 的实例（尝试记录随外键级联删除）
	PurgeFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

// AsyncdJobRunAttemptRepository 执行记录仓库接口（存储: PostgreSQL asyncd_job_run_attempts）
type AsyncdJobRunAttemptRepository interface {
	// Start 记录一次尝试的开始（返回尝试记录 id）
	Start(ctx context.Context, attempt *AsyncdJobRunAttempt) (int64, error)
	// Finish 记录一次尝试的结束（成功/失败 + 耗时，耗时由数据库时间计算）
	Finish(ctx context.Context, id int64, status, cause string) error
	// ListByRun 取某次执行的全部尝试（按尝试序号升序）
	ListByRun(ctx context.Context, jobRunID int64) ([]*AsyncdJobRunAttempt, error)
}

// AsyncdWorkerRepository 执行器实例仓库接口（存储: PostgreSQL asyncd_workers）
type AsyncdWorkerRepository interface {
	// Register 登记/复活一个执行器槽位（进程启动时调用）
	Register(ctx context.Context, worker *AsyncdWorker) error
	// Heartbeat 心跳续期（固定间隔调用；返回受影响行数 0 表示行已被清理）
	Heartbeat(ctx context.Context, workerID string) error
	// Stop 标记优雅退出（stopped_at）
	Stop(ctx context.Context, workerID string) error
	// List 列出执行器槽位与实时在跑任务数（按实例、槽位排序）
	List(ctx context.Context) ([]AsyncdWorkerStat, error)
	// CountOnline 在线槽位数（心跳阈值内且未停止；任务目录的头部摘要用）
	CountOnline(ctx context.Context, staleAfter time.Duration) (int, error)
	// PurgeStaleBefore 清理停止/失联早于 before 的执行器行
	PurgeStaleBefore(ctx context.Context, before time.Time) (int64, error)
}
