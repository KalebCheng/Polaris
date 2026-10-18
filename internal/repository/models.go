// Package repository 定义存储层实体与接口。
//
// 领域模型：以租户（institution）为多租户隔离边界与资源归属维度；
// 租户下可挂部门树（department）；用户、应用、提供方、角色、权限、用户组、
// 证书、令牌、审计、同步器均归属到租户。多值关系使用关联表承载。
package repository

import "time"

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
	PasswordSalt         string   `db:"password_salt" json:"password_salt"`
	PasswordOptions      []string `db:"password_options" json:"password_options"`
	PasswordExpireDays   int      `db:"password_expire_days" json:"password_expire_days"`
	PasswordHistoryCount int      `db:"password_history_count" json:"password_history_count"`
	TokenRetentionDays   int      `db:"token_retention_days" json:"token_retention_days"`
	RecordRetentionDays  int      `db:"record_retention_days" json:"record_retention_days"`
	DefaultApplication   string   `db:"default_application" json:"default_application"`
	DefaultPassword      string   `db:"default_password" json:"default_password"`
	UseEmailAsUsername   bool     `db:"use_email_as_username" json:"use_email_as_username"`
	DisableSignin        bool     `db:"disable_signin" json:"disable_signin"`
	// MfaPolicy 二次验证策略：optional（默认，不强制）| required（强制）
	MfaPolicy string `db:"mfa_policy" json:"mfa_policy"`
	// MfaRememberHours 「记住此设备」免二次校验时长（小时）；0 = 不记住
	MfaRememberHours int    `db:"mfa_remember_hours" json:"mfa_remember_hours"`
	Status           string `db:"status" json:"status"` // active | disabled

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// MFA 策略取值（组织级二次验证）
const (
	MfaPolicyOptional = "optional"
	MfaPolicyRequired = "required"
)

// IsMfaPolicy 判断 MFA 策略取值是否受支持。
func IsMfaPolicy(policy string) bool {
	return policy == MfaPolicyOptional || policy == MfaPolicyRequired
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
	CodePath      string `db:"code_path" json:"code_path"`
	NamePath      string `db:"name_path" json:"name_path"`
	Level         int    `db:"level" json:"level"`
	HasChild      bool   `db:"has_child" json:"has_child"`
	SortIndex     int    `db:"sort_index" json:"sort_index"`
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

	// DepartmentID 主部门（部门树节点；多值归属的真相在 user_departments，仅用于归属与筛选，不参与权限）
	DepartmentID *int64 `db:"department_id" json:"department_id"`
	// JobLevelID 职级（Spec Q4.1 = A：仅作属性用于展示 / 筛选 / 统计，不参与授权判定）
	JobLevelID *int64 `db:"job_level_id" json:"job_level_id,omitempty"`

	PasswordHash string `db:"password_hash" json:"password_hash"`
	PasswordSalt string `db:"password_salt" json:"password_salt"`
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
	MfaSecret        string   `db:"mfa_secret" json:"mfa_secret"`
	MfaRecoveryCodes []string `db:"mfa_recovery_codes" json:"mfa_recovery_codes"`

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

// SigninMethod 应用允许的登录方式
type SigninMethod struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Rule        string `json:"rule"`
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

	Cert         string `db:"cert" json:"cert"` // 签名证书名（certs.name）
	DefaultGroup string `db:"default_group" json:"default_group"`

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

	ClientID     string `db:"client_id" json:"client_id"`
	ClientSecret string `db:"client_secret" json:"client_secret"`

	TokenFormat            string   `db:"token_format" json:"token_format"` // jwt | ...（当前实现 jwt）
	TokenSigningMethod     string   `db:"token_signing_method" json:"token_signing_method"`
	TokenFields            []string `db:"token_fields" json:"token_fields"` // access_token 内嵌字段白名单；空 = 默认
	ExpireInHours          float64  `db:"expire_in_hours" json:"expire_in_hours"`
	RefreshExpireInHours   float64  `db:"refresh_expire_in_hours" json:"refresh_expire_in_hours"`
	DisableRefreshRotation bool     `db:"disable_refresh_rotation" json:"disable_refresh_rotation"`
	CookieExpireInHours    int64    `db:"cookie_expire_in_hours" json:"cookie_expire_in_hours"`

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

// ApplicationSubject 应用授权主体：应用准入名单的一条（三类主体统一落一张表）。
//
// SubjectName 是**组织内唯一标识名**（users.name / groups.name / departments.name），
// 因此同一组织内不会歧义；展示名由服务层 JOIN 回实体装配（主体被删除时表现为"已失效"）。
type ApplicationSubject struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	ApplicationID int64     `db:"application_id" json:"application_id"`
	SubjectType   string    `db:"subject_type" json:"subject_type"`
	SubjectName   string    `db:"subject_name" json:"subject_name"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// Provider 提供商：覆盖外部身份提供方（第三方登录 / 标准 OIDC）、邮件、短信、人机校验、存储多类别。
//
// 字段语义说明：
//   - Category 取值 oauth | email | sms | captcha | storage（小写枚举，迁移 027 已归一化）；
//   - Type 为该类别下的类型 key，声明见 internal/service/provider/catalog.go（后端唯一来源）；
//   - ClientSecret 为 AES-GCM 密文（空串表示未配置），出服务层前一律剥离（HasSecret 表达"是否已配置"）；
//   - 末段 6 个字段是外部身份提供方的 SSO 信任策略（原在 configs/app.yaml 的 provisioning/redirect_uri）。
type Provider struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Category      string `db:"category" json:"category"`
	Type          string `db:"type" json:"type"`

	ClientID     string `db:"client_id" json:"client_id"`
	ClientSecret string `db:"client_secret" json:"client_secret"`

	// 外部身份提供方：签发端点（issuer_url 走 discovery；三项 custom_* 齐全时走静态端点）
	CustomAuthURL     string   `db:"custom_auth_url" json:"custom_auth_url"`
	CustomTokenURL    string   `db:"custom_token_url" json:"custom_token_url"`
	CustomUserInfoURL string   `db:"custom_user_info_url" json:"custom_user_info_url"`
	IssuerURL         string   `db:"issuer_url" json:"issuer_url"`
	Scopes            []string `db:"scopes" json:"scopes"`
	EnablePKCE        bool     `db:"enable_pkce" json:"enable_pkce"`

	// Email：SMTP 连接与邮件模板
	Host        string `db:"host" json:"host"`
	Port        int    `db:"port" json:"port"`
	SslMode     string `db:"ssl_mode" json:"ssl_mode"`
	FromAddress string `db:"from_address" json:"from_address"`
	Title       string `db:"title" json:"title"`
	Content     string `db:"content" json:"content"`
	Receiver    string `db:"receiver" json:"receiver"` // 自检收件人/手机号

	// SMS：区域、签名与模板编号
	RegionID     string `db:"region_id" json:"region_id"`
	SignName     string `db:"sign_name" json:"sign_name"`
	TemplateCode string `db:"template_code" json:"template_code"`
	// TemplateParams 模板变量声明：模板内容里用到的 ${变量}（内置变量集的子集）。
	// 发送时按此装配 TemplateParam——阿里云对变量名多一个少一个都会拒绝，因此不能靠猜。
	// 空 = 按历史行为只发 code。
	TemplateParams []string `db:"template_params" json:"template_params"`

	// Storage：端点、自定义域名、桶与路径前缀
	Endpoint   string `db:"endpoint" json:"endpoint"`
	Domain     string `db:"domain" json:"domain"`
	Bucket     string `db:"bucket" json:"bucket"`
	PathPrefix string `db:"path_prefix" json:"path_prefix"`

	// SSO 信任策略（仅 oauth 类别使用；均为运行时真实读取点，见 provider.Runtime/Resolver）
	AutoCreate     bool     `db:"auto_create" json:"auto_create"`
	LinkByEmail    bool     `db:"link_by_email" json:"link_by_email"`
	AllowedDomains []string `db:"allowed_domains" json:"allowed_domains"`
	RedirectURI    string   `db:"redirect_uri" json:"redirect_uri"`

	State string `db:"state" json:"state"` // active | disabled

	// HasSecret 非持久化：密钥出服务层即被剥离，对外只以"是否已配置"表达
	HasSecret bool `db:"-" json:"has_secret"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Role 角色（组织内唯一；可嵌套、可包含用户与用户组）
type Role struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Description   string `db:"description" json:"description"`
	IsEnabled     bool   `db:"is_enabled" json:"is_enabled"`
	IsSystem      bool   `db:"is_system" json:"is_system"`

	Users     []string `db:"-" json:"users,omitempty"`      // 由关联表装配
	JobTitles []string `db:"-" json:"job_titles,omitempty"` // 由关联表装配（原 groups）
	Roles     []string `db:"-" json:"roles,omitempty"`      // 子角色

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Permission 权限（组织内唯一；绑定主体集合与资源/动作）
type Permission struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Description   string `db:"description" json:"description"`

	Users     []string `db:"-" json:"users,omitempty"`
	JobTitles []string `db:"-" json:"job_titles,omitempty"`
	Roles     []string `db:"-" json:"roles,omitempty"`

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

	Users []string `db:"-" json:"users,omitempty"` // 由关联表装配（任职人员）

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
// 真相来源：多值关系只存这里；users.department_id 保留为「主部门」（is_primary = TRUE），
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
	PrivateKey  string `db:"private_key" json:"private_key"`

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

	Code                string `db:"code" json:"code"`
	CodeChallenge       string `db:"code_challenge" json:"code_challenge"`
	CodeChallengeMethod string `db:"code_challenge_method" json:"code_challenge_method"`
	CodeIsUsed          bool   `db:"code_is_used" json:"code_is_used"`
	CodeExpireIn        int64  `db:"code_expire_in" json:"code_expire_in"`
	CodeScope           string `db:"code_scope" json:"code_scope"`
	CodeRedirectURI     string `db:"code_redirect_uri" json:"code_redirect_uri"`

	AccessToken      string `db:"access_token" json:"access_token"`
	RefreshToken     string `db:"refresh_token" json:"refresh_token"`
	IDToken          string `db:"id_token" json:"id_token"`
	AccessTokenHash  string `db:"access_token_hash" json:"access_token_hash"`
	RefreshTokenHash string `db:"refresh_token_hash" json:"refresh_token_hash"`
	IDTokenHash      string `db:"id_token_hash" json:"id_token_hash"`

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
	User           string    `db:"-" json:"user,omitempty"`
	Provider       string    `db:"-" json:"provider,omitempty"`
	ProviderUserID string    `db:"provider_user_id" json:"provider_user_id"`
	Email          string    `db:"email" json:"email,omitempty"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// PasswordHistory 密码历史（组织密码策略：禁止复用最近 N 次）
type PasswordHistory struct {
	ID            int64     `db:"id" json:"id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	User          string    `db:"user" json:"user"`
	PasswordHash  string    `db:"password_hash" json:"password_hash"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// Syncer 外部用户数据源（对应 Casdoor 的 Syncer）。
// 通过连接器从外部系统（钉钉 / 飞书 / 企业微信 / LDAP / 数据库 / SCIM）拉取用户与组织，写入本地组织。
type Syncer struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Type          string `db:"type" json:"type"`         // dingtalk | lark | wecom | ldap | database
	SubType       string `db:"sub_type" json:"sub_type"` // 接入变体：ad | ldap | mysql | postgres | feishu | lark
	IsEnabled     bool   `db:"is_enabled" json:"is_enabled"`
	Schedule      string `db:"schedule" json:"schedule"` // 空 = 不自动同步；否则为间隔秒数

	// ── 类型专属配置（JSONB 落库；下列字段是运行期视图，不直接持久化）──
	//
	// Config 保存非敏感连接配置（host/port/user/database/table/database_type/ssl_mode/app_id/corp_id），
	// Secrets 保存凭据密文（password/app_secret，AES-GCM），FieldMapping 保存字段映射（account → {本地字段: 源字段}）。
	// 仓储层读取时展开到下面的扁平字段、写入时回收，因此对外 API/DTO 契约保持不变（见 Spec §5.4）。
	Config       map[string]string            `db:"config" json:"-"`
	Secrets      map[string]string            `db:"secrets" json:"-"`
	FieldMapping map[string]map[string]string `db:"field_mapping" json:"-"`

	Host         string `db:"-" json:"host"`
	Port         int    `db:"-" json:"port"`
	User         string `db:"-" json:"user"`
	Password     string `db:"-" json:"-"` // 密文；明文只能经 secret 字段提交（§3.1-7）
	Database     string `db:"-" json:"database"`
	Table        string `db:"-" json:"table"`
	DatabaseType string `db:"-" json:"database_type"`
	SSLMode      string `db:"-" json:"ssl_mode"`

	// 第三方开放平台应用凭证（钉钉/飞书/企业微信）
	AppID     string `db:"-" json:"app_id"`
	AppSecret string `db:"-" json:"-"` // 密文；明文只能经 app_secret_plain 提交
	CorpID    string `db:"-" json:"corp_id"`

	// 字段映射：本地字段 → 源字段（仅支持映射的连接器读取）
	UserMapping map[string]string `db:"-" json:"user_mapping"`

	// 同步配置（JSONB）：增量、调度（每天时刻 / Cron + 执行窗口）、同步保护阈值
	SyncConfig SyncConfig `db:"sync_config" json:"sync_config"`
	// 同步目标：外部组织与账号挂到该部门节点之下（nil = 组织根）
	TargetDepartmentID *int64 `db:"target_department_id" json:"target_department_id"`

	// 同步策略
	SyncUsers  bool `db:"sync_users" json:"sync_users"`
	SyncGroups bool `db:"sync_groups" json:"sync_groups"`
	SoftDelete bool `db:"soft_delete" json:"soft_delete"` // 源端缺失的用户是否在本地停用

	LastRunTime *time.Time `db:"last_run_time" json:"last_run_time"`
	LastStatus  string     `db:"last_status" json:"last_status"` // idle | running | success | failed
	LastMessage string     `db:"last_message" json:"last_message"`
	LastCounts  []byte     `db:"last_counts" json:"last_counts"` // JSONB

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// SyncerRun 一次同步执行记录
type SyncerRun struct {
	ID            int64     `db:"id" json:"id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	Status        string    `db:"status" json:"status"`   // success | failed | skipped | aborted
	Trigger       string    `db:"trigger" json:"trigger"` // manual | schedule | retry
	Message       string    `db:"message" json:"message,omitempty"`
	Counts        []byte    `db:"counts" json:"counts,omitempty"` // JSONB：本次统计
	Errors        []byte    `db:"errors" json:"errors,omitempty"` // JSONB：逐条失败原因（最多 200 条）
	DurationMS    int       `db:"duration_ms" json:"duration_ms"` // 本次耗时（毫秒）
	StartedAt     time.Time `db:"started_at" json:"started_at"`
	FinishedAt    time.Time `db:"finished_at" json:"finished_at"`
}

// SyncSchedule 定时校验周期：每天多时刻（上限 24）或 Cron 表达式，二选一
type SyncSchedule struct {
	Mode string `json:"mode"` // daily | cron | ""（不自动）
	// Times 每天时刻（形如 "03:00"）；上限由服务层校验
	Times []string `json:"times,omitempty"`
	// IntervalSeconds 间隔模式下的周期（秒）
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	Cron            string `json:"cron,omitempty"`
	// WindowMinutes 执行窗口：到点后在该窗口内首次触达即执行一次（默认 30）
	WindowMinutes int `json:"window_minutes,omitempty"`
}

// SyncGuard 同步保护：需要删除的条目达到阈值即取消本次任务（0 = 不限制）
type SyncGuard struct {
	MaxUserDeletes int `json:"max_user_deletes"`
	MaxDeptDeletes int `json:"max_dept_deletes"`
}

// SyncConfig 入方向同步配置（syncers.sync_config）
type SyncConfig struct {
	// Incremental 增量同步（仅声明支持的连接器生效）
	Incremental bool         `json:"incremental"`
	Schedule    SyncSchedule `json:"schedule"`
	Guard       SyncGuard    `json:"guard"`
}

// Connector 下游应用连接器（推送型供给，对应 MaxKey 的 mxk_connectors）。
// 与 Syncer（上游目录拉取）相反：Connector 把本地账号推送到下游应用。
// 复用 service/syncer 的 HTTP 客户端基础设施。
type Connector struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID 关联应用主键（Spec Q5：存储只认主键）；nil = 通用连接器
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	// Application 只读 JOIN 产物（应用 code），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	Name        string `db:"name" json:"name"`
	DisplayName string `db:"display_name" json:"display_name"`
	// Type 连接器类型（http | dingtalk | lark | wecom | ldap），决定使用哪个 provisioner 实现
	Type string `db:"type" json:"type"`

	// Config/Secrets：与入方向同构（config = provider_url/principal；secrets = credentials 密文）
	Config  map[string]string `db:"config" json:"-"`
	Secrets map[string]string `db:"secrets" json:"-"`

	// 类型专属配置与凭据的**运行期视图**（存储为 config / secrets 两个 JSONB 列）。
	// 键由类型目录（service/connection）声明，写入时按声明校验，因此新增一种下游连接器
	// 不需要改表、也不需要改实体（避免"宽表 + 请求绑定漏字段导致配置被静默丢弃"）。
	Values map[string]string `db:"-" json:"-"`
	// HasSecrets 各敏感键是否已配置（响应只表达"已配置"，不下发明文）
	HasSecrets map[string]bool `db:"-" json:"-"`

	State string `db:"state" json:"state"` // active | disabled

	LastRunTime *time.Time `db:"last_run_time" json:"last_run_time"`
	LastStatus  string     `db:"last_status" json:"last_status"` // idle | success | failed
	LastMessage string     `db:"last_message" json:"last_message"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// AccountStrategy 账号策略（对应 MaxKey 的 AccountsStrategy）。
// mapping：本地属性 → 下游属性；policy：何时建号 / 何时回收。
type AccountStrategy struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID 关联应用主键；nil = 租户内默认策略
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	// Application 只读 JOIN 产物（应用 code），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	Name        string `db:"name" json:"name"`
	DisplayName string `db:"display_name" json:"display_name"`
	IsEnabled   bool   `db:"is_enabled" json:"is_enabled"`

	Mapping map[string]string `db:"mapping" json:"mapping"` // 本地属性 → 下游属性
	Policy  map[string]string `db:"policy" json:"policy"`   // 建号/回收触发策略

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ApplicationAccount 应用账号（本地用户 × 应用 → 下游账号，对应 MaxKey 的 mxk_accounts）。
// 幂等键：(institution, application, user)。
type ApplicationAccount struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID / UserID 主键关联（Spec Q5 第 3 批；幂等键改为 (institution_id, application_id, user_id)）
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	UserID        *int64 `db:"user_id" json:"user_id,omitempty"`
	// Application / User 只读 JOIN 产物（应用 code / 用户账号名），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	User        string `db:"-" json:"user,omitempty"`

	Username string `db:"username" json:"username"` // 本地用户名快照（= users.name）

	RelatedUsername string `db:"related_username" json:"related_username"` // 下游应用中的账号名
	// HasPassword 建号时是否向下游设置过初始口令。
	// 说明：口令本体**不落库**（既不存密文也不进审计）——本地没有任何需要它的场景，
	// 存下来只会让"一次库泄露"等于"所有下游账号被接管"（Spec FR-11 / Q6）。
	HasPassword bool `db:"has_password" json:"has_password"`

	CreateType string `db:"create_type" json:"create_type"` // automatic | manual
	Status     string `db:"status" json:"status"`           // active | disabled | pending | failed

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ProvisionHistory 供给历史（对应 MaxKey 的 mxk_history_provisions）。
type ProvisionHistory struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Connector     string `db:"connector" json:"connector"`
	// ApplicationID / UserID 主键关联（可空：对象删除后保留历史行）
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	UserID        *int64 `db:"user_id" json:"user_id,omitempty"`
	// Application / User 只读 JOIN 产物，仅为保持对外契约不变
	Application string    `db:"-" json:"application,omitempty"`
	User        string    `db:"-" json:"user,omitempty"`
	Action      string    `db:"action" json:"action"` // create | update | disable | delete | retry
	Status      string    `db:"status" json:"status"` // success | failed | pending
	Message     string    `db:"message" json:"message,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// ── asyncd：任务定义 / 执行实例 / 执行记录 / 执行器实例 ───────────────────────────
// 四维度模型见 docs/specs/261015-asyncd-task-model.md §4。

// 任务定义的类别（asyncd_jobs.kind）
const (
	// AsyncdJobKindAsync 事件驱动：由业务动作按需入队，没有周期
	AsyncdJobKindAsync = "async"
	// AsyncdJobKindScheduled 定时任务：由调度器按周期/时刻入队
	AsyncdJobKindScheduled = "scheduled"
)

// 调度规则（asyncd_jobs.schedule_kind；仅 kind=scheduled 使用）
const (
	ScheduleKindInterval = "interval" // 每 N 秒
	ScheduleKindDaily    = "daily"    // 每天若干时刻（HH:MM），配合执行窗口
	ScheduleKindCron     = "cron"     // 标准 5 段 Cron 表达式（分 时 日 月 周），见 internal/service/cron
)

// 任务定义的上次调度结果（asyncd_jobs.last_status）
const (
	AsyncdJobStatusIdle    = "idle"    // 还没调度过
	AsyncdJobStatusQueued  = "queued"  // 本次已入队
	AsyncdJobStatusSkipped = "skipped" // 单飞键命中：窗口内已有待执行/执行中的任务，本次跳过
	AsyncdJobStatusFailed  = "failed"  // 入队失败（详见服务端日志）
)

// AsyncdJob 任务定义：一个任务"是什么"（处理器绑定 + 调度规则 + 启停）。
//
// 由代码在启动时声明并幂等 upsert（界面不提供增删改）；运行时**真的读它**：
// 调度器按它判到期、入队按它解析处理器、目录与执行记录按它归属。
type AsyncdJob struct {
	ID       int64  `db:"id" json:"id"`
	Name     string `db:"name" json:"name"`           // 定义名（历史记录按它归属）
	TaskType string `db:"task_type" json:"task_type"` // 处理器注册名（asyncd.Registry）
	Kind     string `db:"kind" json:"kind"`           // async | scheduled
	// ScheduleKind 调度规则：'' | interval | daily | cron
	ScheduleKind    string   `db:"schedule_kind" json:"schedule_kind"`
	IntervalSeconds int      `db:"interval_seconds" json:"interval_seconds"`
	DailyTimes      []string `db:"daily_times" json:"daily_times"`
	// CronExpr schedule_kind=cron 时的 5 段表达式
	CronExpr string `db:"cron_expr" json:"cron_expr"`
	// WindowMinutes 执行窗口：到点后该窗口内入队一次（窗口内已有任务则跳过）
	WindowMinutes int `db:"window_minutes" json:"window_minutes"`
	// Payload 调度入队时的载荷模板（异步任务的载荷由调用方给出）
	Payload []byte `db:"payload" json:"payload"` // JSONB
	// Enabled 仅 kind=scheduled 可停用（异步任务停用会变成静默丢任务）
	Enabled bool `db:"enabled" json:"enabled"`
	// UniqueKey 声明式单飞键（空 = 调度器回落 job:<name>）。
	// 身份连接的计划写 syncer:<id>，与手工「立即同步」共用同一个键——两者不会并发跑同一连接。
	UniqueKey string `db:"unique_key" json:"unique_key"`
	// OwnerSyncerID 归属的身份连接（非空 = 该定义由连接持有；界面不提供启停，改由连接的计划控制）
	OwnerSyncerID *int64 `db:"owner_syncer_id" json:"owner_syncer_id,omitempty"`
	// RetiredAt 代码声明退役时间（保留历史执行记录；任务目录默认不列）
	RetiredAt *time.Time `db:"retired_at" json:"retired_at,omitempty"`
	// OwnerName 归属连接名：由查询 join syncers 带出（**不是 asyncd_jobs 的列**），供界面显示"来源"
	OwnerName string     `db:"-" json:"owner_name,omitempty"`
	NextRunAt time.Time  `db:"next_run_at" json:"next_run_at"`
	LastRunAt *time.Time `db:"last_run_at" json:"last_run_at,omitempty"`
	// LastStatus idle | queued | skipped | failed
	LastStatus string    `db:"last_status" json:"last_status"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

// 执行实例的状态（asyncd_job_runs.status）
const (
	AsyncdJobRunQueued    = "queued"    // 待领取（含延迟执行与退避等待）
	AsyncdJobRunRunning   = "running"   // 已被某个执行器槽位领取（持租约）
	AsyncdJobRunSucceeded = "succeeded" // 成功完成
	AsyncdJobRunDead      = "dead"      // 死信：重试耗尽或不可重试，等待人工处理
)

// AsyncdJobRun 执行实例：一次执行的载体（队列语义 + 状态机 + 租约 + 单飞键）。
//
// 与"执行记录"（AsyncdJobRunAttempt）的分工：本表回答"现在怎么样"（会被领取、回收、重试），
// 尝试表回答"当时发生了什么"（append-only，每次尝试一行）。
type AsyncdJobRun struct {
	ID    int64 `db:"id" json:"id"`
	JobID int64 `db:"job_id" json:"job_id"`
	// Status queued | running | succeeded | dead
	Status         string    `db:"status" json:"status"`
	Payload        []byte    `db:"payload" json:"payload"` // JSONB
	PayloadVersion int       `db:"payload_version" json:"payload_version"`
	Priority       int       `db:"priority" json:"priority"`
	Attempts       int       `db:"attempts" json:"attempts"`
	MaxAttempts    int       `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt  time.Time `db:"next_attempt_at" json:"next_attempt_at"`
	// Deadline 任务级超时（零值 = 不限）
	Deadline *time.Time `db:"deadline" json:"deadline,omitempty"`
	// UniqueKey 单飞键：同一键在 queued/running 下唯一（空 = 不去重）
	UniqueKey string `db:"unique_key" json:"unique_key,omitempty"`
	// WorkerID 领取者（见 AsyncdWorker.ID，形如 host-pid#slot），配合租约实现崩溃恢复
	WorkerID       string     `db:"worker_id" json:"worker_id,omitempty"`
	LeaseExpiresAt *time.Time `db:"lease_expires_at" json:"lease_expires_at,omitempty"`
	LastError      string     `db:"last_error" json:"last_error,omitempty"`
	InstitutionID  *int64     `db:"institution_id" json:"institution_id,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
	FinishedAt     *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	// TaskType 处理器类型：由 Claim 从任务定义 join 带出（**不是 asyncd_job_runs 的列**），
	// 执行器据此找到处理器，因此不落列、不落缓存。
	TaskType string `db:"-" json:"task_type,omitempty"`
}

// 执行记录（每次尝试）的状态（asyncd_job_run_attempts.status）
const (
	AsyncdJobRunAttemptRunning   = "running"    // 正在跑
	AsyncdJobRunAttemptSucceeded = "succeeded"  // 本次尝试成功
	AsyncdJobRunAttemptFailed    = "failed"     // 本次尝试失败（可能还有下一次）
	AsyncdJobRunAttemptLeaseLost = "lease_lost" // 租约过期被判"执行者失联"，结果不可信
)

// AsyncdJobRunAttempt 执行记录：一次执行里的**单次尝试**（append-only）。
//
// 为什么要独立于执行实例：重试会覆盖实例上的 last_error / finished_at，
// 于是"第 2 次为什么失败、花了多久"这类问题在第一阶段的模型里答不出来。
type AsyncdJobRunAttempt struct {
	ID         int64      `db:"id" json:"id"`
	JobRunID   int64      `db:"job_run_id" json:"job_run_id"`
	AttemptNo  int        `db:"attempt_no" json:"attempt_no"`
	WorkerID   string     `db:"worker_id" json:"worker_id"`
	Status     string     `db:"status" json:"status"`
	StartedAt  time.Time  `db:"started_at" json:"started_at"`
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	// DurationMs 本次尝试的真实耗时（不含退避等待）
	DurationMs int    `db:"duration_ms" json:"duration_ms"`
	Error      string `db:"error" json:"error,omitempty"`
}

// AsyncdWorker 执行器实例：一个可领取任务的槽位（进程 × 槽位序号）。
//
// 在线状态**不存列**，由 last_heartbeat_at / stopped_at 计算得出（见 Service.Workers）；
// 在跑任务数同样按 asyncd_job_runs.worker_id 实时聚合，避免派生标记漂移。
type AsyncdWorker struct {
	ID              string     `db:"id" json:"id"`                   // <host>-<pid>#<slot>
	InstanceID      string     `db:"instance_id" json:"instance_id"` // <host>-<pid>
	Host            string     `db:"host" json:"host"`
	PID             int        `db:"pid" json:"pid"`
	SlotIndex       int        `db:"slot_index" json:"slot_index"`
	StartedAt       time.Time  `db:"started_at" json:"started_at"`
	LastHeartbeatAt time.Time  `db:"last_heartbeat_at" json:"last_heartbeat_at"`
	StoppedAt       *time.Time `db:"stopped_at" json:"stopped_at,omitempty"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
}
