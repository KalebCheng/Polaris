// Package provider 是「提供商」的**唯一声明处**：类别、类型、字段 schema、图标与能力。
//
// 设计要点（见 docs/specs/261013-provider-module-phase4.md FR-2 / FR-5）：
//   - 后端声明类型与字段：前端目录页、向导表单、列表卡片全部据此渲染，前端不再硬编码
//     类别/类型/字段清单（历史教训：类别/类型/字段曾分散在三处，新增一个类型要改三个文件）。
//   - 目录只声明**已实现**的类型：每一种类型都对应运行时真实读取点
//     （通知/存储/人机校验见 internal/service/notify|storage|captcha，登录见 internal/auth/oauth），
//     杜绝"能选但没实现"与"配了不生效"。
//   - 字段声明三件事：落在向导哪一步（Group）、进入请求体哪个桶（secrets/policy/config）、
//     落库到哪一列（Store）。因此"界面上填的字段"与"后端读的字段"由同一份声明决定。
package provider

import (
	"sort"
	"strings"
)

// Category 提供商类别（小写枚举，与 providers.category 一致；迁移 027 已归一化）
type Category string

const (
	// CategoryOAuth 外部身份提供方（第三方登录 / 标准 OIDC），可作为应用登录方式
	CategoryOAuth Category = "oauth"
	// CategoryEmail 邮件（验证码 / 通知）
	CategoryEmail Category = "email"
	// CategorySMS 短信（验证码）
	CategorySMS Category = "sms"
	// CategoryCaptcha 人机校验
	CategoryCaptcha Category = "captcha"
	// CategoryStorage 存储（本地目录 / S3 兼容对象存储）
	CategoryStorage Category = "storage"
	// CategoryMFA 多因素认证提供商（TOTP / 短信）
	CategoryMFA Category = "mfa"
)

// 类型 key（providers.type）
const (
	TypeDingTalk = "dingtalk"
	TypeWeChat   = "wechat"
	TypeFeishu   = "feishu"
	TypeLark     = "lark"
	TypeOIDC     = "oidc"

	TypeSMTP   = "smtp"
	TypeAliyun = "aliyun"

	TypeCaptchaImage    = "image"
	TypeCaptchaHCaptcha = "hcaptcha"
	TypeCaptchaRecaptch = "recaptcha"
	TypeTurnstile       = "turnstile"

	TypeStorageLocal = "local"
	TypeStorageS3    = "s3"
	TypeStorageMinIO = "minio"

	TypeMFATOTP = "totp"
	TypeMFASMS  = "sms"
)

// FieldType 字段控件类型（前端据此选择渲染器）
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldPassword FieldType = "password"
	FieldNumber   FieldType = "number"
	FieldSwitch   FieldType = "switch"
	FieldSelect   FieldType = "select"
	FieldTags     FieldType = "tags"
	FieldTextArea FieldType = "textarea"
	// FieldMultiSelect 受限多选：取值必须来自字段声明的 Options（用于"模板支持哪些变量"这类封闭集合）
	FieldMultiSelect FieldType = "multiselect"
)

// 字段分组：决定该字段出现在向导的哪一步（后端声明，前端照此渲染，避免"字段乱串"）
const (
	GroupBasic      = "basic"      // 基本信息：标识 / 名称 / 状态
	GroupConnection = "connection" // 连接配置：凭据与地址（类型专属）
	GroupAdvanced   = "advanced"   // 高级与策略：邮件模板 / SSO 信任策略 / 自检目标
)

// FieldOption 下拉选项
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	// Group 选项分组标题（如「系统字段」/「身份链接」）；空表示不分组。
	// 仅用于下拉展示，不影响取值校验。
	Group string `json:"group,omitempty"`
}

// Condition 条件展示（按同类型内另一个字段的取值决定是否展示）
type Condition struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

// Field 一个表单字段（含校验与展示元数据）
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Group 该字段归属的向导步骤（basic | connection | advanced）
	Group string `json:"group"`
	// Store 该字段的落库列（空表示与 Key 同名）；用于目录与实体列的一致性校验
	Store    string    `json:"store,omitempty"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	// Immutable 创建后不可修改（如提供商标识 name）：编辑态只作展示，不接受修改
	Immutable   bool          `json:"immutable,omitempty"`
	Secret      bool          `json:"secret,omitempty"`
	Policy      bool          `json:"policy,omitempty"` // 归入请求体的 policy 桶（SSO 信任策略）
	Placeholder string        `json:"placeholder,omitempty"`
	Hint        string        `json:"hint,omitempty"`
	Options     []FieldOption `json:"options,omitempty"`
	// RowGroup 同组字段并排展示（桌面端两列）；空表示独占一行。
	// 用于「匹配系统字段 + 匹配提供商字段」这类成对配置。
	RowGroup string `json:"row_group,omitempty"`
	// OptionsSource 动态选项来源（如 sms_providers / sms_templates）；前端据此向资源端点取值，
	// 避免把组织内资源清单硬编码进目录。
	OptionsSource string     `json:"options_source,omitempty"`
	Default       string     `json:"default,omitempty"`
	ShowWhen      *Condition `json:"show_when,omitempty"`
}

// Bucket 该字段进入请求体的哪个桶：secrets（明文敏感）| policy（SSO 信任策略）| config（其余）
func (f Field) Bucket() string {
	switch {
	case f.Secret:
		return "secrets"
	case f.Policy:
		return "policy"
	default:
		return "config"
	}
}

// Column 该字段落库的列名
func (f Field) Column() string {
	if f.Store != "" {
		return f.Store
	}
	return f.Key
}

// Capabilities 类型能力声明（决定界面展示哪些配置与动作，避免"配了不生效"）
type Capabilities struct {
	// Signin 可作为应用登录方式（外部身份提供方）
	Signin bool `json:"signin"`
	// Notify 通知类（邮件 / 短信），验证码登录依赖
	Notify bool `json:"notify"`
	// Captcha 人机校验
	Captcha bool `json:"captcha"`
	// Storage 存储（本地目录 / S3 兼容对象存储）
	Storage bool `json:"storage"`
	// MFA 多因素认证提供商
	MFA bool `json:"mfa"`
	// Test 支持连通性自检
	Test bool `json:"test"`
}

// TypeInfo 一个提供商类型
type TypeInfo struct {
	Key          string       `json:"key"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Icon         string       `json:"icon"`
	Category     Category     `json:"category"`
	Fields       []Field      `json:"fields"`
	Capabilities Capabilities `json:"capabilities"`
}

// CategoryInfo 一个类别（前端标签页）
type CategoryInfo struct {
	Key         Category `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	// Icon 类别图标 key（与类型图标同一套 key；前端 ProviderIcon 渲染，避免前端另起一份映射）
	Icon string `json:"icon"`
	// Singleton 该类别在**同一组织内最多只能有一个启用项**。
	//
	// 依据运行时语义：通知（邮件/短信）、人机校验、存储都是"取该类别首个启用项"来用，
	// 配多个启用只会产生"到底用哪个"的歧义（历史实现按名称升序取第一个，界面上完全看不出来）。
	// 外部身份提供方（oauth）相反：它是"列出全部启用项供登录页选择"，允许多个。
	Singleton bool       `json:"singleton"`
	Types     []TypeInfo `json:"types"`
}

// Catalog 完整目录
type Catalog struct {
	Categories []CategoryInfo `json:"categories"`
}

// Build 返回完整目录（静态声明，调用点直接使用）。
func Build() Catalog {
	return Catalog{Categories: []CategoryInfo{
		{
			Key:         CategoryOAuth,
			Singleton:   false,
			Name:        "外部身份提供方",
			Description: "第三方账号登录与标准 OIDC 单点登录；勾选后可作为应用登录方式。",
			Icon:        "signin",
			Types: []TypeInfo{
				{
					Key: TypeDingTalk, Name: "钉钉", Icon: "dingtalk",
					Description:  "钉钉扫码登录（企业内部应用）",
					Category:     CategoryOAuth,
					Fields:       oauthFields(TypeDingTalk, false),
					Capabilities: Capabilities{Signin: true, Test: true},
				},
				{
					Key: TypeWeChat, Name: "微信", Icon: "wechat",
					Description:  "微信开放平台网站应用扫码登录",
					Category:     CategoryOAuth,
					Fields:       oauthFields(TypeWeChat, false),
					Capabilities: Capabilities{Signin: true, Test: true},
				},
				{
					Key: TypeFeishu, Name: "飞书", Icon: "feishu",
					Description:  "飞书（Feishu）网页授权登录",
					Category:     CategoryOAuth,
					Fields:       oauthFields(TypeFeishu, false),
					Capabilities: Capabilities{Signin: true, Test: true},
				},
				{
					Key: TypeLark, Name: "Lark", Icon: "lark",
					Description:  "Lark（国际版飞书）网页授权登录",
					Category:     CategoryOAuth,
					Fields:       oauthFields(TypeLark, false),
					Capabilities: Capabilities{Signin: true, Test: true},
				},
				{
					Key: TypeOIDC, Name: "标准 OIDC", Icon: "oidc",
					Description:  "任意符合 OpenID Connect 的身份提供方（Keycloak / Authing / Azure AD / 自建）",
					Category:     CategoryOAuth,
					Fields:       oauthFields(TypeOIDC, true),
					Capabilities: Capabilities{Signin: true, Test: true},
				},
			},
		},
		{
			Key:         CategoryMFA,
			Singleton:   false,
			Name:        "多因素认证",
			Description: "登录时的第二因子；TOTP 与短信验证码，按登录方式选择允许的提供商。",
			Icon:        "mfa",
			Types: []TypeInfo{
				{
					Key: TypeMFATOTP, Name: "TOTP", Icon: "totp",
					Description:  "虚拟 MFA（Google Authenticator / 1Password 等）",
					Category:     CategoryMFA,
					Fields:       mfaTOTPFields(),
					Capabilities: Capabilities{MFA: true, Test: true},
				},
				{
					Key: TypeMFASMS, Name: "短信验证码", Icon: "sms",
					Description:  "通过组织内短信提供商发送 MFA 验证码",
					Category:     CategoryMFA,
					Fields:       mfaSMSFields(),
					Capabilities: Capabilities{MFA: true, Test: true},
				},
			},
		},
		{
			Key:         CategoryEmail,
			Singleton:   false,
			Name:        "邮件",
			Description: "邮件验证码与通知；同时是「邮件魔法链接」的发送通道。",
			Icon:        "email",
			Types: []TypeInfo{
				{
					Key: TypeSMTP, Name: "SMTP", Icon: "email",
					Description:  "标准 SMTP 发信（QQ / 企业邮箱 / 自建 Postfix 均可）",
					Category:     CategoryEmail,
					Fields:       emailFields(),
					Capabilities: Capabilities{Notify: true, Test: true},
				},
			},
		},
		{
			Key:         CategorySMS,
			Singleton:   false,
			Name:        "短信",
			Description: "短信验证码发送通道。",
			Icon:        "sms",
			Types: []TypeInfo{
				{
					Key: TypeAliyun, Name: "阿里云短信", Icon: "aliyun",
					Description:  "阿里云短信服务（SendSms，HMAC-SHA1 签名）",
					Category:     CategorySMS,
					Fields:       smsFields(true),
					Capabilities: Capabilities{Notify: true, Test: true},
				},
				{
					Key: "http", Name: "通用 HTTP 网关", Icon: "http",
					Description:  "按 JSON POST 契约对接自有短信网关",
					Category:     CategorySMS,
					Fields:       smsFields(false),
					Capabilities: Capabilities{Notify: true, Test: true},
				},
			},
		},
		{
			Key:         CategoryCaptcha,
			Singleton:   true,
			Name:        "人机校验",
			Description: "登录/注册环节的人机校验；启用后本组织的登录需要校验通过。",
			Icon:        "captcha",
			Types: []TypeInfo{
				{
					Key: TypeCaptchaImage, Name: "本地图形验证码", Icon: "captcha",
					Description:  "平台本地生成图形码，答案存 Redis（多实例一致）",
					Category:     CategoryCaptcha,
					Fields:       []Field{nameField(), displayNameField(), stateField()},
					Capabilities: Capabilities{Captcha: true, Test: true},
				},
				{
					Key: TypeCaptchaHCaptcha, Name: "hCaptcha", Icon: "hcaptcha",
					Description:  "hCaptcha 站点校验（服务端转发核验）",
					Category:     CategoryCaptcha,
					Fields:       captchaSiteFields(),
					Capabilities: Capabilities{Captcha: true, Test: true},
				},
				{
					Key: TypeCaptchaRecaptch, Name: "reCAPTCHA", Icon: "recaptcha",
					Description:  "Google reCAPTCHA v2 / v3（服务端转发核验）",
					Category:     CategoryCaptcha,
					Fields:       captchaSiteFields(),
					Capabilities: Capabilities{Captcha: true, Test: true},
				},
				{
					Key: TypeTurnstile, Name: "Cloudflare Turnstile", Icon: "turnstile",
					Description:  "Cloudflare Turnstile（服务端转发核验）",
					Category:     CategoryCaptcha,
					Fields:       captchaSiteFields(),
					Capabilities: Capabilities{Captcha: true, Test: true},
				},
			},
		},
		{
			Key:         CategoryStorage,
			Singleton:   true,
			Name:        "存储",
			Description: "组织级文件存储：本地目录或 S3 兼容对象存储（头像、Logo、上传附件）；未配置时回落平台默认存储。",
			Icon:        "storage",
			Types: []TypeInfo{
				{
					Key: TypeStorageLocal, Name: "本地文件系统", Icon: "disk",
					Description:  "保存到服务器本地目录（适合单机部署）",
					Category:     CategoryStorage,
					Fields:       storageLocalFields(),
					Capabilities: Capabilities{Storage: true, Test: true},
				},
				{
					Key: TypeStorageS3, Name: "AWS S3", Icon: "s3",
					Description:  "兼容 S3 协议的对象存储（AWS / 阿里云 OSS / 腾讯云 COS）",
					Category:     CategoryStorage,
					Fields:       storageS3Fields(),
					Capabilities: Capabilities{Storage: true, Test: true},
				},
				{
					Key: TypeStorageMinIO, Name: "MinIO", Icon: "minio",
					Description:  "自建 MinIO（S3 兼容，需开启 PathStyle）",
					Category:     CategoryStorage,
					Fields:       storageS3Fields(),
					Capabilities: Capabilities{Storage: true, Test: true},
				},
			},
		},
	}}
}

// ── 字段声明（各类型共用） ──────────────────────────────────────────────────

func nameField() Field {
	return Field{Key: "name", Label: "标识", Group: GroupBasic, Type: FieldText, Required: true,
		Immutable: true, Placeholder: "corp-sso",
		Hint: "组织内唯一；创建后不可修改（唯一可留空的类型是本地图形验证码）"}
}

func displayNameField() Field {
	return Field{Key: "display_name", Label: "名称", Group: GroupBasic, Type: FieldText,
		Placeholder: "企业单点登录", Hint: "展示在登录页与服务列表"}
}

func stateField() Field {
	return Field{Key: "state", Label: "状态", Group: GroupBasic, Type: FieldSelect, Default: "active",
		Options: []FieldOption{{Value: "active", Label: "启用"}, {Value: "disabled", Label: "停用"}}}
}

// oauthFields 外部身份提供方字段。fullDiscovery=true（OIDC）时额外声明端点与 PKCE。
func oauthFields(providerType string, fullDiscovery bool) []Field {
	fields := []Field{
		nameField(), displayNameField(),
		{Key: "client_id", Label: "客户端 ID", Group: GroupConnection, Type: FieldText, Required: true,
			Placeholder: "AppKey / Client ID"},
		{Key: "client_secret", Label: "客户端密钥", Group: GroupConnection, Type: FieldPassword, Required: true,
			Secret: true, Hint: "加密存储；编辑时留空表示保持原值"},
		{Key: "scopes", Label: "授权范围", Group: GroupAdvanced, Type: FieldTags,
			Placeholder: "openid, profile, email", Hint: "留空使用提供方默认范围"},
		stateField(),
	}
	fields = append(fields, policyFields(providerType)...)
	if !fullDiscovery {
		return fields
	}
	// OIDC：issuer 走 discovery；三项自定义端点齐全时改为静态端点（不访问 discovery）
	fields = append(fields,
		Field{Key: "issuer_url", Label: "Issuer 地址", Group: GroupConnection, Type: FieldText, Required: true,
			Placeholder: "https://idp.example.com", Hint: "OIDC 签发方地址（将以 /.well-known/openid-configuration 发现端点）"},
		Field{Key: "custom_auth_url", Label: "授权端点", Group: GroupAdvanced, Type: FieldText,
			Placeholder: "https://idp.example.com/oauth2/authorize", Hint: "非标准 OIDC 时填写；三项端点齐全则不再走 discovery"},
		Field{Key: "custom_token_url", Label: "令牌端点", Group: GroupAdvanced, Type: FieldText,
			Placeholder: "https://idp.example.com/oauth2/token"},
		Field{Key: "custom_user_info_url", Label: "用户信息端点", Group: GroupAdvanced, Type: FieldText,
			Placeholder: "https://idp.example.com/oauth2/userinfo"},
		Field{Key: "enable_pkce", Label: "启用 PKCE", Group: GroupAdvanced, Type: FieldSwitch,
			Default: "true", Hint: "公开客户端或强安全要求时启用（S256）"},
	)
	return fields
}

// policyFields SSO 信任策略（原 Config.Auth.OAuth.Providers[].provisioning 与 redirect_uri）
func policyFields(providerType string) []Field {
	return []Field{
		{Key: "auto_create", Label: "自动创建本地账号", Group: GroupAdvanced, Type: FieldSwitch,
			Policy: true, Default: "true",
			Hint: "未匹配到本地账号时自动建号；若同一外部系统另有入方向身份连接，请把「匹配类型」设为「身份链接」，否则登录与同步会各建一条账号"},
		{Key: "match_type", Label: "匹配类型", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Options: []FieldOption{
				{Value: "", Label: "不启用"},
				{Value: "system", Label: "系统字段"},
				{Value: "identity_link", Label: "身份链接"},
			}, Hint: "先选择系统字段或身份链接；未选择类型时不启用自动匹配"},
		{Key: "match_local_field", Label: "匹配系统字段", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, RowGroup: "identity-match",
			ShowWhen: &Condition{Field: "match_type", Equals: "system"},
			Options: []FieldOption{
				{Value: "", Label: "请选择"},
				{Value: "username", Label: "用户名", Group: "系统字段"},
				{Value: "email", Label: "邮箱", Group: "系统字段"},
				{Value: "phone", Label: "手机号", Group: "系统字段"},
			}, Hint: "按本地用户资料匹配；唯一命中才自动绑定，多个匹配会拒绝登录"},
		{Key: "match_syncer_id", Label: "身份连接", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Required: true, RowGroup: "identity-match",
			ShowWhen:      &Condition{Field: "match_type", Equals: "identity_link"},
			OptionsSource: "syncers", Hint: "选择一条入方向身份连接，匹配只在该连接的映射数据中进行"},
		{Key: "match_syncer_field", Label: "身份连接字段", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Required: true, RowGroup: "identity-match",
			ShowWhen:      &Condition{Field: "match_type", Equals: "identity_link"},
			OptionsSource: "syncer_source_fields",
			Hint:          "选择该连接源端的唯一标识字段（如 external_id / unionid）；同一性由「提供商字段 = 连接字段」判定"},
		{Key: "match_provider_field", Label: "匹配提供商字段", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, RowGroup: "identity-match", Options: providerMatchFields(providerType),
			Hint: "建议使用 unionid / sub 等稳定字段；留空表示不启用匹配"},
		{Key: "provision_username_field", Label: "用户名来源", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, ShowWhen: &Condition{Field: "auto_create", Equals: "true"},
			Options: provisionFieldOptions, Hint: "留空按用户名 / 邮箱前缀生成"},
		{Key: "provision_name_field", Label: "显示名来源", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Default: "nick", ShowWhen: &Condition{Field: "auto_create", Equals: "true"},
			Options: provisionFieldOptions},
		{Key: "provision_email_field", Label: "邮箱来源", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Default: "email", ShowWhen: &Condition{Field: "auto_create", Equals: "true"},
			Options: provisionFieldOptions},
		{Key: "provision_phone_field", Label: "手机号来源", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Default: "mobile", ShowWhen: &Condition{Field: "auto_create", Equals: "true"},
			Options: provisionFieldOptions},
		{Key: "provision_avatar_field", Label: "头像来源", Group: GroupAdvanced, Type: FieldSelect,
			Policy: true, Default: "avatar", ShowWhen: &Condition{Field: "auto_create", Equals: "true"},
			Options: provisionFieldOptions},
		{Key: "allowed_domains", Label: "允许的邮箱域名", Group: GroupAdvanced, Type: FieldTags,
			Policy: true, Placeholder: "corp.example.com", Hint: "留空表示不限制"},
		{Key: "redirect_uri", Label: "回调地址覆盖", Group: GroupAdvanced, Type: FieldText,
			Policy: true, Hint: "默认 /sso/callback（前端命名空间）；可填站内相对路径或完整 URL，留空使用平台默认"},
	}
}

// providerMatchFields 声明各提供商类型可用于唯一性匹配的字段。
// 只列连接器实际能稳定返回的字段；运行时由 authService.matchProviderValue 取值。
func providerMatchFields(providerType string) []FieldOption {
	fields, ok := providerMatchFieldStrategies[providerType]
	if !ok {
		fields = defaultProviderMatchFields
	}
	options := make([]FieldOption, 0, len(fields)+1)
	options = append(options, FieldOption{Value: "", Label: "不启用"})
	return append(options, fields...)
}

// provisionFieldOptions 是自动建号（JIT）可用的提供商字段全集：
// 运行时由 authService 的 matchProviderValue 按同名策略取值，取不到的字段返回空串。
// 类型之间共用一份清单——不同 IdP 返回什么字段由管理员按实际情况选择，不在目录里穷举类型。
var provisionFieldOptions = []FieldOption{
	{Value: "nick", Label: "昵称"},
	{Value: "name", Label: "姓名"},
	{Value: "preferred_username", Label: "用户名"},
	{Value: "email", Label: "邮箱"},
	{Value: "mobile", Label: "手机号"},
	{Value: "phone_number", Label: "手机号（标准 claim）"},
	{Value: "unionid", Label: "UnionID"},
	{Value: "openid", Label: "OpenID"},
	{Value: "sub", Label: "sub"},
	{Value: "avatar", Label: "头像"},
}

var defaultProviderMatchFields = []FieldOption{
	{Value: "sub", Label: "sub"},
	{Value: "email", Label: "邮箱"},
	{Value: "preferred_username", Label: "preferred_username"},
}

// providerMatchFieldStrategies 按提供商类型声明可匹配字段。
// 新增类型只注册一组选项，不再修改 providerMatchFields 的分支。
var providerMatchFieldStrategies = map[string][]FieldOption{
	TypeDingTalk: {
		{Value: "unionid", Label: "UnionID"},
		{Value: "email", Label: "邮箱"},
		{Value: "mobile", Label: "手机号"},
		{Value: "nick", Label: "昵称"},
	},
	TypeOIDC: {
		{Value: "sub", Label: "sub"},
		{Value: "email", Label: "邮箱"},
		{Value: "preferred_username", Label: "preferred_username"},
		{Value: "phone_number", Label: "手机号"},
		{Value: "name", Label: "姓名"},
	},
	TypeWeChat: {
		{Value: "openid", Label: "OpenID"},
		{Value: "unionid", Label: "UnionID"},
		{Value: "email", Label: "邮箱"},
		{Value: "mobile", Label: "手机号"},
		{Value: "nick", Label: "昵称"},
	},
	TypeFeishu: {
		{Value: "openid", Label: "OpenID"},
		{Value: "unionid", Label: "UnionID"},
		{Value: "email", Label: "邮箱"},
		{Value: "mobile", Label: "手机号"},
		{Value: "nick", Label: "昵称"},
	},
	TypeLark: {
		{Value: "openid", Label: "OpenID"},
		{Value: "unionid", Label: "UnionID"},
		{Value: "email", Label: "邮箱"},
		{Value: "mobile", Label: "手机号"},
		{Value: "nick", Label: "昵称"},
	},
}

func emailFields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "host", Label: "SMTP 主机", Group: GroupConnection, Type: FieldText, Required: true,
			Placeholder: "smtp.example.com"},
		{Key: "port", Label: "端口", Group: GroupConnection, Type: FieldNumber, Required: true, Default: "465",
			Placeholder: "465", Hint: "465（SSL）/ 587（STARTTLS）/ 25（明文）"},
		{Key: "client_id", Label: "账号", Group: GroupConnection, Type: FieldText,
			Placeholder: "noreply@example.com", Hint: "SMTP 登录账号"},
		{Key: "client_secret", Label: "密码 / 授权码", Group: GroupConnection, Type: FieldPassword, Secret: true,
			Hint: "加密存储；编辑时留空表示保持原值"},
		{Key: "ssl_mode", Label: "加密方式", Group: GroupConnection, Type: FieldSelect, Default: "auto",
			Options: []FieldOption{{Value: "auto", Label: "自动（STARTTLS）"}, {Value: "disable", Label: "不加密"}}},
		{Key: "from_address", Label: "发信人地址", Group: GroupAdvanced, Type: FieldText,
			Placeholder: "Polaris <noreply@example.com>"},
		{Key: "receiver", Label: "自检收件人", Group: GroupAdvanced, Type: FieldText,
			Hint: "填写后「检查连通性」会真实发送一条测试邮件"},
		stateField(),
	}
}

func smsFields(aliyun bool) []Field {
	fields := []Field{nameField(), displayNameField()}
	if aliyun {
		// 阿里云走 HMAC-SHA1 签名，必须要有 AccessKey
		fields = append(fields,
			Field{Key: "client_id", Label: "AccessKey ID", Group: GroupConnection, Type: FieldText,
				Required: true, Placeholder: "LTAI5t..."},
			Field{Key: "client_secret", Label: "AccessKey Secret", Group: GroupConnection, Type: FieldPassword,
				Required: true, Secret: true, Hint: "加密存储；编辑时留空表示保持原值"},
		)
	}
	// 通用 HTTP 网关按固定 JSON 契约调用（phone/code/signName/template），不读取任何凭据——
	// 因此**不声明** client_id/client_secret（避免又一处"配了不生效"）。
	fields = append(fields,
		Field{Key: "sign_name", Label: "短信签名", Group: GroupConnection, Type: FieldText, Placeholder: "Polaris"},
	)
	if aliyun {
		fields = append(fields,
			Field{Key: "endpoint", Label: "接口地址", Group: GroupAdvanced, Type: FieldText,
				Placeholder: "https://dysmsapi.aliyuncs.com", Hint: "留空使用阿里云默认地址"},
			Field{Key: "region_id", Label: "区域", Group: GroupAdvanced, Type: FieldText,
				Placeholder: "cn-hangzhou", Hint: "留空使用 cn-hangzhou"},
		)
	} else {
		fields = append(fields,
			Field{Key: "endpoint", Label: "网关地址", Group: GroupConnection, Type: FieldText, Required: true,
				Placeholder: "https://sms.example.com/send", Hint: "平台以 JSON POST 调用（字段：phone/code/signName/template）"},
		)
	}
	return append(fields,
		Field{Key: "receiver", Label: "自检手机号", Group: GroupAdvanced, Type: FieldText,
			Hint: "填写后「检查连通性」会真实发送一条测试短信"},
		stateField(),
	)
}

func mfaTOTPFields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "mfa_issuer", Label: "发行方", Group: GroupConnection, Type: FieldText,
			Placeholder: "Polaris", Hint: "显示在验证器 App 中；留空使用组织显示名"},
		{Key: "mfa_digits", Label: "动态码位数", Group: GroupConnection, Type: FieldSelect, Default: "6",
			Options: []FieldOption{{Value: "6", Label: "6 位"}, {Value: "8", Label: "8 位"}}},
		{Key: "mfa_period", Label: "刷新周期（秒）", Group: GroupConnection, Type: FieldNumber, Default: "30",
			Hint: "15–120 秒"},
		{Key: "mfa_algorithm", Label: "算法", Group: GroupAdvanced, Type: FieldSelect, Default: "SHA1",
			Options: []FieldOption{{Value: "SHA1", Label: "SHA1"}, {Value: "SHA256", Label: "SHA256"}, {Value: "SHA512", Label: "SHA512"}}},
		{Key: "mfa_remember_hours", Label: "记住设备时长（小时）", Group: GroupAdvanced, Type: FieldNumber,
			Default: "0", Hint: "0 = 不允许记住设备；仅对勾选「记住此设备」的用户生效"},
		stateField(),
	}
}

func mfaSMSFields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "mfa_sms_provider_id", Label: "短信提供商", Group: GroupConnection, Type: FieldSelect, Required: true,
			OptionsSource: "sms_providers", Hint: "MFA 验证码通过该短信渠道发送"},
		{Key: "mfa_sms_template_id", Label: "MFA 短信模板", Group: GroupConnection, Type: FieldSelect, Required: true,
			OptionsSource: "sms_templates", Hint: "只列出所选短信提供商下 scene=mfa_code 的模板"},
		{Key: "mfa_code_ttl_seconds", Label: "验证码有效期（秒）", Group: GroupAdvanced, Type: FieldNumber, Default: "300",
			Hint: "30–1800 秒"},
		{Key: "mfa_remember_hours", Label: "记住设备时长（小时）", Group: GroupAdvanced, Type: FieldNumber,
			Default: "0", Hint: "0 = 不允许记住设备；按 MFA 提供商分别生效"},
		stateField(),
	}
}

func captchaSiteFields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "client_id", Label: "Site Key", Group: GroupConnection, Type: FieldText, Required: true},
		{Key: "client_secret", Label: "Secret Key", Group: GroupConnection, Type: FieldPassword, Required: true,
			Secret: true, Hint: "加密存储；编辑时留空表示保持原值"},
		stateField(),
	}
}

func storageLocalFields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "endpoint", Label: "存储根目录", Group: GroupConnection, Type: FieldText, Required: true,
			Placeholder: "/data/polaris/storage", Hint: "服务器上的绝对路径，需可写"},
		{Key: "path_prefix", Label: "对象前缀", Group: GroupConnection, Type: FieldText, Placeholder: "polaris/"},
		{Key: "domain", Label: "自定义访问域名", Group: GroupConnection, Type: FieldText,
			Placeholder: "https://cdn.example.com", Hint: "留空使用平台地址"},
		stateField(),
	}
}

func storageS3Fields() []Field {
	return []Field{
		nameField(), displayNameField(),
		{Key: "endpoint", Label: "端点", Group: GroupConnection, Type: FieldText, Required: true,
			Placeholder: "https://s3.example.com"},
		{Key: "bucket", Label: "Bucket", Group: GroupConnection, Type: FieldText, Required: true},
		{Key: "region_id", Label: "区域", Group: GroupConnection, Type: FieldText, Placeholder: "us-east-1"},
		{Key: "client_id", Label: "AccessKey ID", Group: GroupConnection, Type: FieldText},
		{Key: "client_secret", Label: "AccessKey Secret", Group: GroupConnection, Type: FieldPassword, Secret: true,
			Hint: "加密存储；编辑时留空表示保持原值"},
		{Key: "path_prefix", Label: "对象前缀", Group: GroupConnection, Type: FieldText, Placeholder: "polaris/"},
		{Key: "domain", Label: "自定义访问域名", Group: GroupConnection, Type: FieldText,
			Placeholder: "https://cdn.example.com"},
		stateField(),
	}
}

// ── 查询辅助 ────────────────────────────────────────────────────────────────

// Categories 返回全部类别（前端标签页顺序）
func Categories() []CategoryInfo { return Build().Categories }

// IsSingleton 该类别在同一组织内是否最多只能有一个启用项
func IsSingleton(category Category) bool {
	for _, group := range Build().Categories {
		if string(group.Key) == NormalizeCategory(string(category)) {
			return group.Singleton
		}
	}
	return false
}

// Find 按类别与类型 key 查类型
func Find(category Category, key string) (TypeInfo, bool) {
	for _, group := range Build().Categories {
		if string(group.Key) != NormalizeCategory(string(category)) {
			continue
		}
		for _, item := range group.Types {
			if item.Key == key {
				return item, true
			}
		}
	}
	return TypeInfo{}, false
}

// FindByType 仅按类型 key 查（用于已知行的运行时分派；同 key 在不同类别下不得重复）
func FindByType(key string) (TypeInfo, bool) {
	for _, group := range Build().Categories {
		for _, item := range group.Types {
			if item.Key == key {
				return item, true
			}
		}
	}
	return TypeInfo{}, false
}

// TypeKeys 返回某类别的类型 key（供服务层做"未知类型拒绝"校验）
func TypeKeys(category Category) []string {
	normalized := NormalizeCategory(string(category))
	keys := make([]string, 0, 8)
	for _, group := range Build().Categories {
		if string(group.Key) != normalized {
			continue
		}
		for _, item := range group.Types {
			keys = append(keys, item.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// 各类别的默认类型：类型留空（或历史数据无类型）时的回落值。
// 必须是"无需任何额外凭据、开箱即用"的那一个（对齐 Casdoor 的 Default 语义）。
var defaultTypes = map[string]string{
	string(CategoryOAuth):   TypeOIDC,
	string(CategoryEmail):   TypeSMTP,
	string(CategorySMS):     TypeAliyun,
	string(CategoryCaptcha): TypeCaptchaImage,
	string(CategoryStorage): TypeStorageLocal,
	string(CategoryMFA):     TypeMFATOTP,
}

// DefaultType 返回某类别的默认类型（创建时类型留空则取此值）
func DefaultType(category Category) string {
	if key, ok := defaultTypes[NormalizeCategory(string(category))]; ok {
		return key
	}
	keys := TypeKeys(category)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// NormalizeCategory 归一化类别取值（历史数据/外部输入的大小写与旧别名）。
func NormalizeCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(CategoryOAuth), "oidc":
		// "OIDC" 曾是一个独立类别：外部身份提供方是同一件事，统一归入 oauth
		return string(CategoryOAuth)
	case string(CategoryEmail), "mail":
		return string(CategoryEmail)
	case string(CategorySMS), "sms_code":
		return string(CategorySMS)
	case string(CategoryCaptcha), "verify":
		return string(CategoryCaptcha)
	case string(CategoryStorage), "oss":
		return string(CategoryStorage)
	case string(CategoryMFA), "2fa":
		return string(CategoryMFA)
	default:
		return ""
	}
}

// NormalizeType 归一化类型取值（历史数据的大小写与旧别名）。
func NormalizeType(category Category, value string) string {
	kind := strings.ToLower(strings.TrimSpace(value))
	switch kind {
	case "lark_feishu":
		kind = TypeFeishu
	case "recaptcha_v2", "recaptcha_v3", "google_recaptcha":
		kind = TypeCaptchaRecaptch
	case "default", "":
		return DefaultType(category)
	}
	if _, ok := Find(category, kind); ok {
		return kind
	}
	return ""
}

// FieldsIn 返回某类型下属于指定步骤的字段
func FieldsIn(info TypeInfo, group string) []Field {
	out := make([]Field, 0, len(info.Fields))
	for _, field := range info.Fields {
		if field.Group == group {
			out = append(out, field)
		}
	}
	return out
}

// LookupField 按 key 查某类型的字段声明
func LookupField(info TypeInfo, key string) (Field, bool) {
	for _, field := range info.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return Field{}, false
}
