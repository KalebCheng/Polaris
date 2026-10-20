package repository

import "time"

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

	// 类别专属配置由 provider_<category> 子表承载；db:"-" 表示不进 providers 主表。
	// child tag 供目录绑定器在同名字段上读写子表列。
	ClientID     string `db:"-" child:"client_id" json:"client_id"`
	ClientSecret string `db:"-" child:"client_secret" json:"-"`

	// 外部身份提供方：签发端点（issuer_url 走 discovery；三项 custom_* 齐全时走静态端点）
	CustomAuthURL     string   `db:"-" child:"custom_auth_url" json:"custom_auth_url"`
	CustomTokenURL    string   `db:"-" child:"custom_token_url" json:"custom_token_url"`
	CustomUserInfoURL string   `db:"-" child:"custom_user_info_url" json:"custom_user_info_url"`
	IssuerURL         string   `db:"-" child:"issuer_url" json:"issuer_url"`
	Scopes            []string `db:"-" child:"scopes" json:"scopes"`
	EnablePKCE        bool     `db:"-" child:"enable_pkce" json:"enable_pkce"`

	// Email / SMS / Storage 共用连接字段。
	Host        string `db:"-" child:"host" json:"host"`
	Port        int    `db:"-" child:"port" json:"port"`
	SslMode     string `db:"-" child:"ssl_mode" json:"ssl_mode"`
	FromAddress string `db:"-" child:"from_address" json:"from_address"`
	Receiver    string `db:"-" child:"receiver" json:"receiver"` // 自检收件人/手机号

	// SMS：区域与签名（模板只存在 provider_email_templates / provider_sms_templates）。
	RegionID string `db:"-" child:"region_id" json:"region_id"`
	SignName string `db:"-" child:"sign_name" json:"sign_name"`

	// MFA 提供商参数（category=mfa）：TOTP 与短信 MFA 共用这些列。
	MfaIssuer         string `db:"-" child:"mfa_issuer" json:"mfa_issuer"`
	MfaDigits         int    `db:"-" child:"mfa_digits" json:"mfa_digits"`
	MfaPeriod         int    `db:"-" child:"mfa_period" json:"mfa_period"`
	MfaAlgorithm      string `db:"-" child:"mfa_algorithm" json:"mfa_algorithm"`
	MfaSMSProviderID  int64  `db:"-" child:"mfa_sms_provider_id" json:"mfa_sms_provider_id"`
	MfaSMSTemplateID  int64  `db:"-" child:"mfa_sms_template_id" json:"mfa_sms_template_id"`
	MfaCodeTTLSeconds int    `db:"-" child:"mfa_code_ttl_seconds" json:"mfa_code_ttl_seconds"`
	MfaRememberHours  int    `db:"-" child:"mfa_remember_hours" json:"mfa_remember_hours"`

	// Storage：端点、自定义域名、桶与路径前缀
	Endpoint   string `db:"-" child:"endpoint" json:"endpoint"`
	Domain     string `db:"-" child:"domain" json:"domain"`
	Bucket     string `db:"-" child:"bucket" json:"bucket"`
	PathPrefix string `db:"-" child:"path_prefix" json:"path_prefix"`

	// SSO 信任策略（仅 oauth 类别使用；均为运行时真实读取点，见 provider.Runtime/Resolver）
	AutoCreate bool `db:"-" child:"auto_create" json:"auto_create"`
	// MatchPolicy selects either local system fields or an inbound identity-link source.
	MatchType          string `db:"-" child:"match_type" json:"match_type"`
	MatchLocalField    string `db:"-" child:"match_local_field" json:"match_local_field"`
	MatchProviderField string `db:"-" child:"match_provider_field" json:"match_provider_field"`
	MatchSyncerID      int64  `db:"-" child:"match_syncer_id" json:"match_syncer_id,omitempty"`
	MatchSyncerField   string `db:"-" child:"match_syncer_field" json:"match_syncer_field"`
	// LinkByEmail 旧版开关，仅作为兼容：Match* 为空且该值为 true 时等价于 email/email。
	LinkByEmail    bool     `db:"-" child:"link_by_email" json:"link_by_email"`
	AllowedDomains []string `db:"-" child:"allowed_domains" json:"allowed_domains"`
	RedirectURI    string   `db:"-" child:"redirect_uri" json:"redirect_uri"`

	// JIT（不存在时自动建号）的字段映射：本地字段 ← 提供商字段；空串表示使用默认字段。
	// 与 match_* 一样都是运行时真实读取点（见 auth_sso.go provision）。
	ProvisionUsernameField string `db:"-" child:"provision_username_field" json:"provision_username_field"`
	ProvisionNameField     string `db:"-" child:"provision_name_field" json:"provision_name_field"`
	ProvisionEmailField    string `db:"-" child:"provision_email_field" json:"provision_email_field"`
	ProvisionPhoneField    string `db:"-" child:"provision_phone_field" json:"provision_phone_field"`
	ProvisionAvatarField   string `db:"-" child:"provision_avatar_field" json:"provision_avatar_field"`

	State string `db:"state" json:"state"` // active | disabled

	// HasSecret 非持久化：密钥出服务层即被剥离，对外只以"是否已配置"表达
	HasSecret bool `db:"-" json:"has_secret"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ProviderTemplate 邮件/短信提供商的场景模板。
// 场景只包含真实发送点：验证码登录、MFA 短信、魔法链接。
type ProviderTemplate struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	ProviderID    int64 `db:"provider_id" json:"provider_id"`
	// Channel 非持久化：email | sms；决定内容来自 provider_email_templates 还是 provider_sms_templates。
	Channel        string    `db:"-" json:"channel"`
	Name           string    `db:"name" json:"name"`
	Scene          string    `db:"scene" json:"scene"`
	TemplateCode   string    `db:"template_code" json:"template_code"`
	TemplateParams []string  `db:"template_params" json:"template_params"`
	Subject        string    `db:"subject" json:"subject"`
	Content        string    `db:"content" json:"content"`
	IsDefault      bool      `db:"is_default" json:"is_default"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// 通知模板场景。
const (
	TemplateSceneLoginCode = "login_code"
	TemplateSceneMfaCode   = "mfa_code"
	TemplateSceneMagicLink = "magic_link"
)

// IsTemplateScene 判断模板场景取值是否受支持。
func IsTemplateScene(scene string) bool {
	switch scene {
	case TemplateSceneLoginCode, TemplateSceneMfaCode, TemplateSceneMagicLink:
		return true
	default:
		return false
	}
}

// UserMFACredential 用户绑定的 MFA 凭证（一个用户可对多个 MFA 提供商各绑定一条）。
type UserMFACredential struct {
	ID                  int64          `db:"id" json:"id"`
	InstitutionID       int64          `db:"institution_id" json:"institution_id"`
	UserID              int64          `db:"user_id" json:"user_id"`
	ProviderID          int64          `db:"provider_id" json:"provider_id"`
	Type                string         `db:"type" json:"type"`
	Status              string         `db:"status" json:"status"`
	SecretEncrypted     string         `db:"secret_encrypted" json:"-"`
	RecoveryCodesHashed []string       `db:"recovery_codes_hashed" json:"-"`
	Phone               string         `db:"phone" json:"phone"`
	Metadata            map[string]any `db:"metadata" json:"metadata"`
	CreatedAt           time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time      `db:"updated_at" json:"updated_at"`
}

// MFA 凭证状态。
const (
	MfaCredentialPending  = "pending"
	MfaCredentialActive   = "active"
	MfaCredentialDisabled = "disabled"
)
