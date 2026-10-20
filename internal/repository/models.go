// Package repository 定义存储层实体与接口。
//
// 领域模型：以租户（institution）为多租户隔离边界与资源归属维度；
// 租户下可挂部门树（department）；用户、应用、提供方、角色、权限、用户组、
// 证书、令牌、审计、同步器均归属到租户。多值关系使用关联表承载。
package repository

import (
	"encoding/json"
	"strings"
	"time"
)

// ThemeData 主题配置（应用级品牌）
type ThemeData struct {
	ThemeType    string `json:"themeType"`
	ColorPrimary string `json:"colorPrimary"`
	BorderRadius int    `json:"borderRadius"`
	IsCompact    bool   `json:"isCompact"`
	IsEnabled    bool   `json:"isEnabled"`
}

// AdapterConfig 应用适配器配置（P4；仅 adapter = "custom" 时生效）。
// ClaimMap 把"输出的 claim 名"映射到"用户属性来源"：{"<claim>": "<userAttr>"}。
type AdapterConfig struct {
	ClaimMap map[string]string `json:"claimMap"`
}

// TokenAttribute 自定义 access_token claim 的一条映射（对齐 Casdoor Token 属性）。
// Category：existing（值取自用户字段）/ static（静态值）。
// Type：string / array；existing 字段沿用其原生类型，static 才按 Type 转换。
type TokenAttribute struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Value    string `json:"value"`
	Type     string `json:"type"`
}

// Institution 租户（多租户隔离边界与资源归属维度）。
// 中文 UI 文案称「组织」；标识层统一为 institution（见 Spec ADR D1）。
type Institution struct {
	ID          int64  `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	DisplayName string `db:"display_name" json:"display_name"`
	Domain      string `db:"domain" json:"domain"` // 唯一域名（按 Host 解析租户；空则忽略）
	WebsiteURL  string `db:"website_url" json:"website_url"`
	Logo        string `db:"logo" json:"logo"`
	Favicon     string `db:"favicon" json:"favicon"`
	Description string `db:"description" json:"description"`
	Captcha     string `db:"captcha" json:"captcha"` // 人机校验策略（如 NONE,TEXT,ARITHMETIC）

	// PasswordType 密码类型（见 internal/auth/cred：bcrypt|<其他>，默认 bcrypt）
	PasswordType string `db:"password_type" json:"password_type"`
	// PasswordSalt 组织级密码盐/胡椒（write-only，响应不下发；参与固定盐算法）
	PasswordSalt         string   `db:"password_salt" json:"-"`
	PasswordOptions      []string `db:"password_options" json:"password_options"`
	PasswordExpireDays   int      `db:"password_expire_days" json:"password_expire_days"`
	PasswordHistoryCount int      `db:"password_history_count" json:"password_history_count"`
	TokenRetentionDays   int      `db:"token_retention_days" json:"token_retention_days"`
	RecordRetentionDays  int      `db:"record_retention_days" json:"record_retention_days"`
	DefaultApplication   string   `db:"default_application" json:"default_application"`
	DefaultPasswordEnc   string   `db:"default_password_enc" json:"-"`
	// DefaultPasswordPlain is input-only and never persisted or returned.
	DefaultPasswordPlain string `db:"-" json:"-"`
	UseEmailAsUsername   bool   `db:"use_email_as_username" json:"use_email_as_username"`
	DisableSignin        bool   `db:"disable_signin" json:"disable_signin"`
	Status               string `db:"status" json:"status"` // active | disabled

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// StorageSettings 系统级默认对象存储配置（单行；密钥经 SecretCipher 加密存储，接口不回传明文）。
type StorageSettings struct {
	ID   int16  `db:"id" json:"id"`
	Type string `db:"type" json:"type"` // '' 未配置 | local | s3 | minio

	Endpoint  string `db:"endpoint" json:"endpoint"`
	Region    string `db:"region" json:"region"`
	Bucket    string `db:"bucket" json:"bucket"`
	AccessKey string `db:"access_key" json:"access_key"`
	// SecretKeyEnc 加密后的 Secret Key（write-only，绝不下发）
	SecretKeyEnc string `db:"secret_key_enc" json:"-"`
	PathPrefix   string `db:"path_prefix" json:"path_prefix"`
	Domain       string `db:"domain" json:"domain"`

	RootPath   string `db:"root_path" json:"root_path"`     // local 模式根目录
	PublicBase string `db:"public_base" json:"public_base"` // local 模式公开访问前缀
	PathStyle  bool   `db:"path_style" json:"path_style"`   // S3 兼容：是否使用 path-style

	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Department 部门（租户内的层级节点；仅作用户归属与筛选，不参与权限，见 Spec ADR D2）。
type Department struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Description   string `db:"description" json:"description"`
	ParentID      *int64 `db:"parent_id" json:"parent_id"`
	// ParentIDChain 祖先主键链（根→父，不含自身）；用于子树判断，不存字符串路径。
	ParentIDChain []int64 `db:"parent_id_chain" json:"parent_id_chain"`
	// CodePath / NamePath / Level 读时由递归 CTE 计算，不落库。
	CodePath  string `db:"-" json:"code_path"`
	NamePath  string `db:"-" json:"name_path"`
	Level     int    `db:"-" json:"level"`
	HasChild  bool   `db:"has_child" json:"has_child"`
	SortIndex int    `db:"sort_index" json:"sort_index"`
	// IsRoot 组织根节点（每个组织恰好一行；名字随组织，禁止改名/移动/删除，见 Spec Q4.5）
	IsRoot bool `db:"is_root" json:"is_root"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// User 用户账号（租户内唯一：institution + name）
type User struct {
	ID            int64  `db:"id" json:"id"`
	UID           string `db:"uid" json:"uid"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	Type          string `db:"type" json:"type"` // normal | admin

	// DepartmentID 主部门（API 派生字段，不落库；唯一真相是 user_departments.is_primary）
	DepartmentID *int64 `db:"-" json:"department_id"`
	// JobLevelID 职级（Spec Q4.1 = A：仅作属性用于展示 / 筛选 / 统计，不参与授权判定）
	JobLevelID *int64 `db:"job_level_id" json:"job_level_id,omitempty"`

	PasswordHash string `db:"password_hash" json:"-"`
	PasswordSalt string `db:"password_salt" json:"-"`
	PasswordType string `db:"password_type" json:"password_type"`

	DisplayName   string `db:"display_name" json:"display_name"`
	FirstName     string `db:"first_name" json:"first_name"`
	LastName      string `db:"last_name" json:"last_name"`
	Avatar        string `db:"avatar" json:"avatar"`
	Email         string `db:"email" json:"email"`
	EmailVerified bool   `db:"email_verified" json:"email_verified"`
	Phone         string `db:"phone" json:"phone"`
	PhoneVerified bool   `db:"phone_verified" json:"phone_verified"`
	CountryCode   string `db:"country_code" json:"country_code"`
	Location      string `db:"location" json:"location"`
	Address       string `db:"address" json:"address"`
	Affiliation   string `db:"affiliation" json:"affiliation"`
	Title         string `db:"title" json:"title"`
	Homepage      string `db:"homepage" json:"homepage"`
	Bio           string `db:"bio" json:"bio"`
	Language      string `db:"language" json:"language"`
	Gender        string `db:"gender" json:"gender"`
	Birthday      string `db:"birthday" json:"birthday"`
	Education     string `db:"education" json:"education"`
	Score         int    `db:"score" json:"score"`

	IsAdmin     bool `db:"is_admin" json:"is_admin"`
	IsForbidden bool `db:"is_forbidden" json:"is_forbidden"`
	IsDeleted   bool `db:"is_deleted" json:"is_deleted"`
	IsOnline    bool `db:"is_online" json:"is_online"`

	SignupApplication string `db:"signup_application" json:"signup_application"`
	RegisterType      string `db:"register_type" json:"register_type"`
	RegisterSource    string `db:"register_source" json:"register_source"`

	CreatedIP      string     `db:"created_ip" json:"created_ip"`
	LastSigninTime *time.Time `db:"last_signin_time" json:"last_signin_time"`
	LastSigninIP   string     `db:"last_signin_ip" json:"last_signin_ip"`

	PasswordUpdatedAt *time.Time `db:"password_updated_at" json:"password_updated_at"`

	MfaEnabled       bool     `db:"mfa_enabled" json:"mfa_enabled"`
	MfaType          string   `db:"mfa_type" json:"mfa_type"` // totp
	MfaSecret        string   `db:"mfa_secret" json:"-"`
	MfaRecoveryCodes []string `db:"mfa_recovery_codes" json:"-"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ProviderItem 应用启用的身份提供方条目
type ProviderItem struct {
	Name      string `json:"name"` // providers.name
	CanSignUp bool   `json:"canSignUp"`
	CanSignIn bool   `json:"canSignIn"`
	CanUnlink bool   `json:"canUnlink"`
	Prompted  bool   `json:"prompted"`
	AlertType string `json:"alertType"`
	Rule      string `json:"rule"`
}

// 登录方式名称：signin_methods[].name 的合法取值（后端判定与前端渲染共用同一组字面量）。
const (
	SigninMethodPassword  = "Password"  // 密码
	SigninMethodEmailCode = "EmailCode" // 邮箱验证码
	SigninMethodSMSCode   = "SMSCode"   // 短信验证码
	SigninMethodMagicLink = "MagicLink" // 邮件魔法链接（免密）
	SigninMethodProvider  = "Provider"  // 第三方 / SSO 提供方
	SigninMethodGuest     = "Guest"     // 访客
)

// 密码登录标识类型（signin_methods[Password].identifiers 的合法取值）。
const (
	PasswordIdentifierUsername = "username"
	PasswordIdentifierEmail    = "email"
	PasswordIdentifierPhone    = "phone"
)

// SigninMethod 应用允许的登录方式。
//
// 渠道类方式（EmailCode / SMSCode / MagicLink）显式引用 provider + template；
// 所有方式都可以独立启用 MFA：
//   - MfaEnabled=false：完全不走 MFA；
//   - MfaEnabled=true, MfaRequired=false：未绑定用户每次登录提示绑定但可跳过；已绑定用户仍需验证；
//   - MfaEnabled=true, MfaRequired=true：必须完成绑定与验证。
type SigninMethod struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Rule        string `json:"rule"`

	// Identifiers 仅 Password 使用：username | email | phone；至少一项。
	Identifiers []string `json:"identifiers"`

	// ProviderID / TemplateID 仅通知类方式使用（同组织 provider 主键 + 场景匹配的模板主键）。
	ProviderID int64 `json:"providerId,omitempty"`
	TemplateID int64 `json:"templateId,omitempty"`

	MfaEnabled     bool    `json:"mfaEnabled"`
	MfaRequired    bool    `json:"mfaRequired"`
	MfaProviderIDs []int64 `json:"mfaProviderIds"`

	// 反序列化时记录字段是否真实存在：历史 Password 方法没有 mfaEnabled 时按 true 解释，
	// 但不能把管理员显式关闭（mfaEnabled=false）误判成"缺失"。
	mfaEnabledSet  bool
	mfaRequiredSet bool
}

// UnmarshalJSON 记录 MFA 字段是否存在，用于兼容旧 signin_methods JSON。
func (m *SigninMethod) UnmarshalJSON(data []byte) error {
	type alias SigninMethod
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = SigninMethod(decoded)
	_, m.mfaEnabledSet = raw["mfaEnabled"]
	_, m.mfaRequiredSet = raw["mfaRequired"]
	return nil
}

// MFAEnabled 返回按兼容默认解释后的启用状态。
func (m SigninMethod) MFAEnabled() bool {
	if m.mfaEnabledSet {
		return m.MfaEnabled
	}
	// 旧行为：Password 用户只要绑定了 MFA 就会被挑战。v1.1 把它解释为"启用但默认不强制绑定"。
	return m.Name == SigninMethodPassword
}

// MFARequired 返回是否强制完成绑定/校验（旧数据一律 false，由迁移按组织策略回填）。
func (m SigninMethod) MFARequired() bool { return m.mfaRequiredSet && m.MfaRequired }

// ResolvePasswordIdentifiers 返回密码标识集合：历史数据缺少字段时兼容为用户名 + 邮箱。
func (m SigninMethod) ResolvePasswordIdentifiers() []string {
	if len(m.Identifiers) == 0 {
		return []string{PasswordIdentifierUsername, PasswordIdentifierEmail}
	}
	out := make([]string, 0, len(m.Identifiers))
	seen := map[string]bool{}
	for _, item := range m.Identifiers {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

// IsPasswordIdentifier 判断标识类型是否受支持。
func IsPasswordIdentifier(value string) bool {
	switch value {
	case PasswordIdentifierUsername, PasswordIdentifierEmail, PasswordIdentifierPhone:
		return true
	default:
		return false
	}
}

// FindSigninMethod 返回应用启用的登录方式；未配置 signin_methods 的历史应用回退为 Password。
func (a *Application) FindSigninMethod(name string) (SigninMethod, bool) {
	if a == nil {
		return SigninMethod{}, false
	}
	if len(a.SigninMethods) == 0 {
		return SigninMethod{Name: SigninMethodPassword, DisplayName: SigninMethodPassword}, name == SigninMethodPassword
	}
	for _, method := range a.SigninMethods {
		if method.Name == name {
			return method, true
		}
	}
	return SigninMethod{}, false
}

// HasSigninMethod 判定应用是否启用了某种登录方式（"配置即生效"的唯一判定入口）。
// 兜底：signin_methods 为空时视为仅密码（兼容迁移前创建、且未回填的历史数据）。
func (a *Application) HasSigninMethod(name string) bool {
	if a == nil {
		return false
	}
	if len(a.SigninMethods) == 0 {
		return name == SigninMethodPassword
	}
	for _, m := range a.SigninMethods {
		if m.Name == name {
			return true
		}
	}
	return false
}

// SignupItem 注册表单字段编排
type SignupItem struct {
	Name        string   `json:"name"`
	Visible     bool     `json:"visible"`
	Required    bool     `json:"required"`
	Prompted    bool     `json:"prompted"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Placeholder string   `json:"placeholder"`
	Options     []string `json:"options"`
	Regex       string   `json:"regex"`
	Rule        string   `json:"rule"`
}

// ScopeItem 应用可申请的 OAuth scope
type ScopeItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// Application 应用（OAuth2/OIDC 客户端）
// 命名约定（第二阶段改名）：Code 是应用标识（URL/API/外部引用用，创建后不可修改），
// Name 是人类可读名称；二者与列 code / name 一一对应。
type Application struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Code          string `db:"code" json:"code"`
	Name          string `db:"name" json:"name"`
	Category      string `db:"category" json:"category"` // default | web | native | desktop
	Type          string `db:"type" json:"type"`         // oauth | oidc
	Logo          string `db:"logo" json:"logo"`
	Title         string `db:"title" json:"title"`
	Description   string `db:"description" json:"description"`
	HomepageURL   string `db:"homepage_url" json:"homepage_url"`
	Order         int    `db:"sort_order" json:"order"`

	LogoDark string   `db:"logo_dark" json:"logo_dark"`
	Favicon  string   `db:"favicon" json:"favicon"`
	Tags     []string `db:"tags" json:"tags"`

	CertID            *int64 `db:"cert_id" json:"cert_id,omitempty"`
	DefaultJobTitleID *int64 `db:"default_job_title_id" json:"default_job_title_id,omitempty"`

	// 登录页行为与访问限制
	OrgChoiceMode string `db:"org_choice_mode" json:"org_choice_mode"`
	IPRestriction string `db:"ip_restriction" json:"ip_restriction"`
	IPWhitelist   string `db:"ip_whitelist" json:"ip_whitelist"`

	// 访问模式（谁能使用该应用，见 Spec 第六阶段）：
	//   all        → 全员可访问（默认；与引入该能力之前的行为完全一致）
	//   restricted → 仅 application_subjects 名单内主体（含其子部门 / 子用户组）可访问
	// 运行时判定唯一入口：ApplicationAccessService.Allowed（禁止在别处复制判定）。
	AccessMode string `db:"access_mode" json:"access_mode"`

	// 跳转地址（仅保留有运行时读取点的两项：注册入口与用户协议）
	SignupURL  string `db:"signup_url" json:"signup_url"`
	TermsOfUse string `db:"terms_of_use" json:"terms_of_use"`

	// 登录方式（唯一数据源，驱动登录页渲染与后端放行判定）
	SigninMethods []SigninMethod `db:"signin_methods" json:"signin_methods"`
	// 注册字段编排（驱动托管注册页 /signup 的字段与校验）
	SignupItems []SignupItem `db:"signup_items" json:"signup_items"`

	// 自助注册（本阶段收敛的唯一注册开关，取代相悖的 enable_sign_up / disable_self_signup）
	SelfSignupEnabled bool `db:"self_signup_enabled" json:"self_signup_enabled"`
	SignupAutoSignin  bool `db:"signup_auto_signin" json:"signup_auto_signin"`

	// 各登录方式的一次性凭据有效期（秒）
	EmailCodeTTLSeconds int `db:"email_code_ttl_seconds" json:"email_code_ttl_seconds"`
	SMSCodeTTLSeconds   int `db:"sms_code_ttl_seconds" json:"sms_code_ttl_seconds"`
	MagicLinkTTLSeconds int `db:"magic_link_ttl_seconds" json:"magic_link_ttl_seconds"`

	// 应用级品牌主题（覆盖组织默认）
	ThemeData *ThemeData `db:"theme_data" json:"theme_data"`

	// 应用适配器（D6）：default 为标准 OIDC claims；custom 时以 AdapterConfig 驱动 claim 映射
	Adapter       string         `db:"adapter" json:"adapter"`
	AdapterConfig *AdapterConfig `db:"adapter_config" json:"adapter_config"`

	// 会话策略（运行时生效：application_policy.enforceSessionPolicy）
	// MaxSessions：0 = 不限制；1 = 独占（新登录踢掉该用户在本应用下的其他会话）；N = 最多 N 个。
	MaxSessions int `db:"max_sessions" json:"max_sessions"`

	// 登录失败限制（运行时生效：auth_service.signinFailurePolicy / countFailure）
	// SigninErrorLimit：0 = 沿用平台默认（auth.rate_limit.login_max_attempts）；N = 该应用的失败次数上限。
	// SigninRetryWaitMinutes：0 = 沿用平台默认（auth.rate_limit.login_window_seconds）；
	//   N = 达到次数上限后自此刻起拒绝继续尝试的分钟数。
	SigninErrorLimit       int `db:"signin_error_limit" json:"signin_error_limit"`
	SigninRetryWaitMinutes int `db:"signin_retry_wait_minutes" json:"signin_retry_wait_minutes"`

	GrantTypes   []string       `db:"grant_types" json:"grant_types"`
	RedirectURIs []string       `db:"redirect_uris" json:"redirect_uris"`
	Providers    []ProviderItem `db:"providers" json:"providers"`
	Scopes       []ScopeItem    `db:"scopes" json:"scopes"`

	ClientID         string `db:"client_id" json:"client_id"`
	ClientSecretHash string `db:"client_secret_hash" json:"-"`
	// ClientSecretPlain is populated only for one-time create/reset responses.
	ClientSecretPlain string `db:"-" json:"-"`

	// TokenFormat 声明模式：standard（仅 OIDC 标准 claims）/ custom（按 token_attributes 与适配器下发业务 claims）
	TokenFormat            string           `db:"token_format" json:"token_format"`
	TokenSigningMethod     string           `db:"token_signing_method" json:"token_signing_method"`
	TokenAttributes        []TokenAttribute `db:"token_attributes" json:"token_attributes"` // access_token claim 映射；空 = 默认 username/institution
	ExpireInHours          float64          `db:"expire_in_hours" json:"expire_in_hours"`
	RefreshExpireInHours   float64          `db:"refresh_expire_in_hours" json:"refresh_expire_in_hours"`
	DisableRefreshRotation bool             `db:"disable_refresh_rotation" json:"disable_refresh_rotation"`
	CookieExpireInHours    int64            `db:"cookie_expire_in_hours" json:"cookie_expire_in_hours"`

	ForcedRedirectOrigin string `db:"forced_redirect_origin" json:"forced_redirect_origin"`
	BackchannelLogoutURI string `db:"backchannel_logout_uri" json:"backchannel_logout_uri"`

	State string `db:"state" json:"state"` // active | disabled

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// 应用访问模式取值（applications.access_mode）。
const (
	AccessModeAll        = "all"        // 全员可访问（默认）
	AccessModeRestricted = "restricted" // 受限：仅授权名单内主体可访问
)

// IsAccessMode 判定访问模式取值是否受支持。
func IsAccessMode(mode string) bool {
	return mode == AccessModeAll || mode == AccessModeRestricted
}

// 应用授权主体类型（application_subjects.subject_type）。
const (
	SubjectTypeUser = "user"
	// SubjectTypeJobTitle 职务（原用户组；Spec 第六阶段 Q3 = A 改名，取值 group → job_title）
	SubjectTypeJobTitle   = "job_title"
	SubjectTypeDepartment = "department"
)

// IsSubjectType 判定主体类型取值是否受支持。
func IsSubjectType(t string) bool {
	return t == SubjectTypeUser || t == SubjectTypeJobTitle || t == SubjectTypeDepartment
}

// ApplicationSubject is one entry in an application's access allowlist.
// Exactly one subject ID column is populated, matching SubjectType.
type ApplicationSubject struct {
	ID                 int64     `db:"id" json:"id"`
	InstitutionID      int64     `db:"institution_id" json:"institution_id"`
	ApplicationID      int64     `db:"application_id" json:"application_id"`
	SubjectType        string    `db:"subject_type" json:"subject_type"`
	UserID             *int64    `db:"user_id" json:"user_id,omitempty"`
	JobTitleID         *int64    `db:"job_title_id" json:"job_title_id,omitempty"`
	DepartmentID       *int64    `db:"department_id" json:"department_id,omitempty"`
	SubjectName        string    `db:"-" json:"name"`
	SubjectDisplayName string    `db:"-" json:"display_name"`
	SubjectPath        string    `db:"-" json:"path,omitempty"`
	SubjectEmail       string    `db:"-" json:"email,omitempty"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
}
