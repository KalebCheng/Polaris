package repository

import "time"

// Role 角色（组织内唯一；可嵌套、可包含用户与用户组）
type Role struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Description   string `db:"description" json:"description"`
	IsEnabled     bool   `db:"is_enabled" json:"is_enabled"`
	IsSystem      bool   `db:"is_system" json:"is_system"`

	Users       []RelationRef `db:"-" json:"users,omitempty"`
	JobTitles   []RelationRef `db:"-" json:"job_titles,omitempty"`
	Roles       []RelationRef `db:"-" json:"roles,omitempty"`
	Permissions []RelationRef `db:"-" json:"permissions,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// RelationRef is a display-safe reference to another resource.
type RelationRef struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

// Permission 权限（组织内唯一；绑定主体集合与资源/动作）
type Permission struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Description   string `db:"description" json:"description"`

	Users     []RelationRef `db:"-" json:"users,omitempty"`
	JobTitles []RelationRef `db:"-" json:"job_titles,omitempty"`
	Roles     []RelationRef `db:"-" json:"roles,omitempty"`

	ResourceType string   `db:"resource_type" json:"resource_type"`
	Resources    []string `db:"resources" json:"resources"`
	Actions      []string `db:"actions" json:"actions"`
	Effect       string   `db:"effect" json:"effect"` // Allow | Deny
	IsEnabled    bool     `db:"is_enabled" json:"is_enabled"`

	ExpireTime  *time.Time `db:"expire_time" json:"expire_time,omitempty"`
	Submitter   string     `db:"submitter" json:"submitter,omitempty"`
	Approver    string     `db:"approver" json:"approver,omitempty"`
	ApproveTime *time.Time `db:"approve_time" json:"approve_time,omitempty"`
	State       string     `db:"state" json:"state"` // Approved | Pending | Rejected

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// JobTitle 职务（组织内唯一；**扁平**，无层级）。
//
// 定位（Spec 第六阶段 Q3 / Q4）：组织架构里的「人 × 职务」关系，是批量授权的载体——
// 可挂角色（role_job_titles）、可挂权限（permission_job_titles）、可作应用访问主体
// （application_subjects.subject_type = 'job_title'）。
//
// 为什么没有层级：飞书职务字典没有层级、钉钉 title / 企微 position 是自由文本（无源数据可填）；
// 且旧用户组层级在前端根本无从设置（实测 parent_id 全为 NULL），属"接了线但永不触发"的半死能力，
// 按 §3.1-10 与 §1「配置即生效」拆除（连带 type / properties / is_top_group）。
type JobTitle struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	// Manager / ContactEmail 仅为展示信息（无任何运行时读取点），不参与授权判定。
	Manager      string `db:"manager" json:"manager,omitempty"`
	ContactEmail string `db:"contact_email" json:"contact_email,omitempty"`
	IsEnabled    bool   `db:"is_enabled" json:"is_enabled"`

	Users   []string `db:"-" json:"users,omitempty"`
	UserIDs []int64  `db:"-" json:"user_ids,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// JobLevel 职级（组织内唯一；按 SortOrder 升序展示）。
//
// 定位（Spec 第六阶段 Q4.1 = A）：**仅作为用户属性**，不参与授权（不建 role_job_levels /
// permission_job_levels）。用途只有三处，且必须全部接线：用户表单/详情下拉、用户列表筛选与排序、
// 组织概览分布统计。维护方式与职务同构：本地 CRUD + 身份源同步（飞书 job_level，只拉不推）。
type JobLevel struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	Description   string `db:"description" json:"description"`
	// SortOrder 对齐飞书 order 语义：数值越小越靠前
	SortOrder int       `db:"sort_order" json:"sort_order"`
	IsEnabled bool      `db:"is_enabled" json:"is_enabled"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// 外部身份映射（Spec 第六阶段 Q1 = A）：把"外部系统里的对象"锚定到"本地对象"，
// 键为 **连接 + 源端唯一标识**。四张表结构一致，故共用一个实体；Repository 按连接类型分方法。
type SyncerUserLink struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	LocalID       int64     `db:"user_id" json:"-"`
	ExternalID    string    `db:"external_id" json:"external_id"`
	Raw           []byte    `db:"raw" json:"raw,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

type SyncerDepartmentLink struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	LocalID       int64     `db:"department_id" json:"-"`
	ExternalID    string    `db:"external_id" json:"external_id"`
	Raw           []byte    `db:"raw" json:"raw,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

type SyncerJobTitleLink struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	LocalID       int64     `db:"job_title_id" json:"-"`
	ExternalID    string    `db:"external_id" json:"external_id"`
	Raw           []byte    `db:"raw" json:"raw,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

type SyncerJobLevelLink struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	LocalID       int64     `db:"job_level_id" json:"-"`
	ExternalID    string    `db:"external_id" json:"external_id"`
	Raw           []byte    `db:"raw" json:"raw,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// ExternalIdentityRef 某本地对象在某个身份连接下的外部身份（详情页展示 + 排障用）。
// Raw 是源端报文，存它是为了"同步结果对不上时能直接看到源端给了什么"，故必须有展示点（§1 同源约束）。
type ExternalIdentityRef struct {
	SyncerID   int64     `json:"syncer_id"`
	SyncerName string    `json:"syncer_name"`
	SyncerType string    `json:"syncer_type"`
	ExternalID string    `json:"external_id"`
	Raw        []byte    `json:"raw,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// UserDepartment 用户 ↔ 部门的多值归属（Spec 第六阶段 Q2 = A）。
//
// 真相来源：多值关系与主部门（is_primary = TRUE）都只存这里。
// 让既有 DTO / 列表筛选 / 应用访问判定继续可用。
type UserDepartment struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	UserID        int64     `db:"user_id" json:"user_id"`
	DepartmentID  int64     `db:"department_id" json:"department_id"`
	IsPrimary     bool      `db:"is_primary" json:"is_primary"`
	SyncerID      *int64    `db:"syncer_id" json:"syncer_id,omitempty"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// Cert 证书（用于 id_token 签名 / SAML 等）
type Cert struct {
	ID              int64  `db:"id" json:"id"`
	InstitutionID   int64  `db:"institution_id" json:"institution_id"`
	Name            string `db:"name" json:"name"`
	DisplayName     string `db:"display_name" json:"display_name"`
	Scope           string `db:"scope" json:"scope"`                       // JWT | SAML
	Type            string `db:"type" json:"type"`                         // x509
	CryptoAlgorithm string `db:"crypto_algorithm" json:"crypto_algorithm"` // RSA | ECDSA
	BitSize         int    `db:"bit_size" json:"bit_size"`
	ExpireInYears   int    `db:"expire_in_years" json:"expire_in_years"`

	Certificate string `db:"certificate" json:"certificate"`
	PrivateKey  string `db:"private_key" json:"-"`

	ExpireTime *time.Time `db:"expire_time" json:"expire_time"`
	IsSystem   bool       `db:"is_system" json:"is_system"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Token 令牌（授权码 / access / refresh / id token 记录，用于内省与吊销）
type Token struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID / UserID 主键关联（Spec Q5 = 全量主键化；见迁移 041）
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	UserID        *int64 `db:"user_id" json:"user_id,omitempty"`
	// Application / User 只读 JOIN 产物（应用 code / 用户账号名），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	User        string `db:"-" json:"user,omitempty"`
	Name        string `db:"name" json:"name"`

	CodeHash            string `db:"code_hash" json:"-"`
	CodeChallenge       string `db:"code_challenge" json:"code_challenge"`
	CodeChallengeMethod string `db:"code_challenge_method" json:"code_challenge_method"`
	CodeIsUsed          bool   `db:"code_is_used" json:"code_is_used"`
	CodeExpireIn        int64  `db:"code_expire_in" json:"code_expire_in"`
	CodeScope           string `db:"code_scope" json:"code_scope"`
	CodeRedirectURI     string `db:"code_redirect_uri" json:"code_redirect_uri"`
	// CodeNonce 授权请求的 nonce，换码时必须原样写入 id_token（OIDC Core 3.1.2.1）。
	CodeNonce string `db:"code_nonce" json:"code_nonce"`

	AccessTokenHash  string `db:"access_token_hash" json:"-"`
	RefreshTokenHash string `db:"refresh_token_hash" json:"-"`
	IDTokenHash      string `db:"id_token_hash" json:"-"`

	ExpiresIn  int        `db:"expires_in" json:"expires_in"`
	Scope      string     `db:"scope" json:"scope"`
	TokenType  string     `db:"token_type" json:"token_type"`
	GrantType  string     `db:"grant_type" json:"grant_type"`
	Resource   string     `db:"resource" json:"resource"`
	SessionID  string     `db:"session_id" json:"session_id"`
	Revoked    bool       `db:"revoked" json:"revoked"`
	ExpireTime *time.Time `db:"expire_time" json:"expire_time"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Record 审计记录（对应 Casdoor Record）
type Record struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name,omitempty"`
	ClientIP      string `db:"client_ip" json:"client_ip,omitempty"`
	// User 事件发生时的操作者名称快照（审计不可变：对象改名/删除后仍可读）
	User string `db:"user" json:"user,omitempty"`
	// UserID 可空主键关联（Spec Q5 第 4 批：按人精确检索；用户删除时置空，不连带删审计行）
	UserID      *int64    `db:"user_id" json:"user_id,omitempty"`
	Method      string    `db:"method" json:"method,omitempty"`
	RequestURI  string    `db:"request_uri" json:"request_uri,omitempty"`
	Action      string    `db:"action" json:"action,omitempty"`
	Language    string    `db:"language" json:"language,omitempty"`
	Object      string    `db:"object" json:"object,omitempty"`
	Response    string    `db:"response" json:"response,omitempty"`
	StatusCode  int       `db:"status_code" json:"status_code"`
	Detail      string    `db:"detail" json:"detail,omitempty"`
	IsTriggered bool      `db:"is_triggered" json:"is_triggered"`
	TraceID     string    `db:"trace_id" json:"trace_id,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// UserIdentity 外部身份绑定（provider 账号 ↔ 本地用户）
type UserIdentity struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// UserID / ProviderID 主键关联（Spec Q5 第 5 批；唯一键收敛为 institution + provider + 源端 ID）
	UserID     int64  `db:"user_id" json:"user_id"`
	ProviderID *int64 `db:"provider_id" json:"provider_id,omitempty"`
	// User / Provider 只读 JOIN 产物（账号名 / provider 名），仅为保持对外契约不变
	User           string `db:"-" json:"user,omitempty"`
	Provider       string `db:"-" json:"provider,omitempty"`
	ProviderUserID string `db:"provider_user_id" json:"provider_user_id"`
	Email          string `db:"email" json:"email,omitempty"`
	// Attributes 提供商侧原始身份字段（unionid/openid/sub/email/mobile/…），
	// 供"同源身份连接"按配置的字段对做双向认领（登录 ↔ 同步）。
	Attributes map[string]string `db:"attributes" json:"-"`
	CreatedAt  time.Time         `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time         `db:"updated_at" json:"updated_at"`
}

// PasswordHistory 密码历史（组织密码策略：禁止复用最近 N 次）
type PasswordHistory struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	User          string    `db:"user" json:"user"`
	PasswordHash  string    `db:"password_hash" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}
