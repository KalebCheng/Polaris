package handler

import (
	"time"

	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// applicationDTO 应用响应 DTO。
// 安全约定：client_secret 一律不下发（仅"创建"与"重置密钥"的响应会一次性返回明文），
// 其余敏感项以 has_* 布尔表达"是否已配置"。
// 完整性约定：实体中所有可编辑字段都必须出现在此处——历史教训：DTO 漏字段会让编辑页
// 提交时用空值把未回显的字段覆盖掉（编辑即重置）。
type applicationDTO struct {
	ID            int64    `json:"id"`
	InstitutionID int64    `json:"institution_id"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Type          string   `json:"type"`
	Logo          string   `json:"logo,omitempty"`
	LogoDark      string   `json:"logo_dark,omitempty"`
	Favicon       string   `json:"favicon,omitempty"`
	Title         string   `json:"title,omitempty"`
	Description   string   `json:"description,omitempty"`
	HomepageURL   string   `json:"homepage_url,omitempty"`
	Tags          []string `json:"tags"`
	Order         int      `json:"order"`

	Cert         string `json:"cert,omitempty"`
	DefaultGroup string `json:"default_group,omitempty"`

	// 登录方式与自助注册
	SigninMethods       []repository.SigninMethod `json:"signin_methods"`
	SignupItems         []repository.SignupItem   `json:"signup_items"`
	SelfSignupEnabled   bool                      `json:"self_signup_enabled"`
	SignupAutoSignin    bool                      `json:"signup_auto_signin"`
	EmailCodeTTLSeconds int                       `json:"email_code_ttl_seconds"`
	SMSCodeTTLSeconds   int                       `json:"sms_code_ttl_seconds"`
	MagicLinkTTLSeconds int                       `json:"magic_link_ttl_seconds"`

	// 会话策略（0 = 不限制；1 = 独占；N = 最多 N 个）
	MaxSessions int `json:"max_sessions"`

	// 登录失败限制（0 = 沿用平台默认 auth.rate_limit）
	SigninErrorLimit       int `json:"signin_error_limit"`
	SigninRetryWaitMinutes int `json:"signin_retry_wait_minutes"`

	// 登录页跳转地址与访问限制
	OrgChoiceMode string `json:"org_choice_mode"`
	IPRestriction string `json:"ip_restriction"`
	IPWhitelist   string `json:"ip_whitelist"`
	// 访问模式（all | restricted）与授权主体数：列表「访问范围」列用。
	// 计数只在**列表**里由 access 服务批量装配（避免 N+1）；详情走 GET /applications/:id/access。
	AccessMode           string `json:"access_mode"`
	AccessSubjectCount   int    `json:"access_subject_count"`
	SignupURL            string `json:"signup_url"`
	TermsOfUse           string `json:"terms_of_use"`
	ForcedRedirectOrigin string `json:"forced_redirect_origin"`
	BackchannelLogoutURI string `json:"backchannel_logout_uri"`

	GrantTypes   []string                  `json:"grant_types"`
	RedirectURIs []string                  `json:"redirect_uris"`
	Providers    []repository.ProviderItem `json:"providers"`
	Scopes       []repository.ScopeItem    `json:"scopes"`

	ClientID        string `json:"client_id"`
	HasClientSecret bool   `json:"has_client_secret"`

	TokenFormat            string   `json:"token_format"`
	TokenSigningMethod     string   `json:"token_signing_method"`
	TokenFields            []string `json:"token_fields"`
	DisableRefreshRotation bool     `json:"disable_refresh_rotation"`
	ExpireInHours          float64  `json:"expire_in_hours"`
	RefreshExpireInHours   float64  `json:"refresh_expire_in_hours"`
	CookieExpireInHours    int64    `json:"cookie_expire_in_hours"`

	// 应用适配器（D6）：default 为标准 OIDC claims；custom 时以 adapter_config.claimMap 驱动
	Adapter       string                    `json:"adapter"`
	AdapterConfig *repository.AdapterConfig `json:"adapter_config"`

	// 应用级品牌主题（覆盖组织默认）
	ThemeData *repository.ThemeData `json:"theme_data"`

	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toApplicationDTO(a *repository.Application) *applicationDTO {
	if a == nil {
		return nil
	}
	return &applicationDTO{
		ID: a.ID, InstitutionID: a.InstitutionID, Code: a.Code, Name: a.Name,
		Category: a.Category, Type: a.Type, Logo: a.Logo, LogoDark: a.LogoDark,
		Favicon: a.Favicon, Title: a.Title, Description: a.Description,
		HomepageURL: a.HomepageURL, Tags: orEmptySlice(a.Tags), Order: a.Order,
		Cert: a.Cert, DefaultGroup: a.DefaultGroup,
		SigninMethods: orEmptyMethods(a.SigninMethods), SignupItems: orEmptySignupItems(a.SignupItems),
		SelfSignupEnabled: a.SelfSignupEnabled, SignupAutoSignin: a.SignupAutoSignin,
		EmailCodeTTLSeconds: a.EmailCodeTTLSeconds, SMSCodeTTLSeconds: a.SMSCodeTTLSeconds,
		MagicLinkTTLSeconds: a.MagicLinkTTLSeconds,
		MaxSessions:         a.MaxSessions,
		SigninErrorLimit:    a.SigninErrorLimit, SigninRetryWaitMinutes: a.SigninRetryWaitMinutes,
		OrgChoiceMode: a.OrgChoiceMode, IPRestriction: a.IPRestriction, IPWhitelist: a.IPWhitelist,
		AccessMode: accessModeOrDefault(a.AccessMode),
		SignupURL:  a.SignupURL, TermsOfUse: a.TermsOfUse,
		ForcedRedirectOrigin: a.ForcedRedirectOrigin, BackchannelLogoutURI: a.BackchannelLogoutURI,
		GrantTypes: orEmptySlice(a.GrantTypes), RedirectURIs: orEmptySlice(a.RedirectURIs),
		Providers: orEmptyItems(a.Providers), Scopes: orEmptyScopes(a.Scopes),
		ClientID: a.ClientID, HasClientSecret: a.ClientSecret != "",
		TokenFormat: a.TokenFormat, TokenSigningMethod: a.TokenSigningMethod,
		TokenFields: orEmptySlice(a.TokenFields), DisableRefreshRotation: a.DisableRefreshRotation,
		ExpireInHours: a.ExpireInHours, RefreshExpireInHours: a.RefreshExpireInHours,
		CookieExpireInHours: a.CookieExpireInHours,
		Adapter:             a.Adapter, AdapterConfig: a.AdapterConfig,
		ThemeData: a.ThemeData, State: a.State,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// accessModeOrDefault 空串按默认（全员可访问）下发；兼容迁移前的旧行与测试构造的实体。
func accessModeOrDefault(mode string) string {
	if repository.IsAccessMode(mode) {
		return mode
	}
	return repository.AccessModeAll
}

func toApplicationDTOs(items []*repository.Application) []*applicationDTO {
	out := make([]*applicationDTO, 0, len(items))
	for _, a := range items {
		out = append(out, toApplicationDTO(a))
	}
	return out
}

// providerDTO 提供方响应 DTO（client_secret 一律不下发，仅表示是否已配置）。
// providerDTO 提供商响应。
//
// 敏感字段只以 has_secret 表达；config（非敏感配置）与 policy（SSO 信任策略）
// 由 internal/service/provider 的目录声明决定具体键——前端按同一份目录渲染，不做二次解释。
type providerDTO struct {
	ID            int64          `json:"id"`
	InstitutionID int64          `json:"institution_id"`
	Name          string         `json:"name"`
	DisplayName   string         `json:"display_name"`
	Category      string         `json:"category"`
	Type          string         `json:"type"`
	State         string         `json:"state"`
	Config        map[string]any `json:"config"`
	Policy        map[string]any `json:"policy"`
	HasSecret     bool           `json:"has_secret"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func toProviderDTO(p *repository.Provider) *providerDTO {
	if p == nil {
		return nil
	}
	category := providerpkg.NormalizeCategory(p.Category)
	kind := providerpkg.Kind(p)
	info, _ := providerpkg.Find(providerpkg.Category(category), kind)
	config, policy := providerpkg.Extract(p, info)
	if config == nil {
		config = map[string]any{}
	}
	if policy == nil {
		policy = map[string]any{}
	}
	return &providerDTO{
		ID: p.ID, InstitutionID: p.InstitutionID, Name: p.Name, DisplayName: p.DisplayName,
		Category: category, Type: kind, State: p.State,
		Config: config, Policy: policy, HasSecret: p.HasSecret,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toProviderDTOs(items []*repository.Provider) []*providerDTO {
	out := make([]*providerDTO, 0, len(items))
	for _, p := range items {
		out = append(out, toProviderDTO(p))
	}
	return out
}

// certDTO 证书响应 DTO（私钥不下发）。
type certDTO struct {
	ID              int64      `json:"id"`
	InstitutionID   int64      `json:"institution_id"`
	Name            string     `json:"name"`
	DisplayName     string     `json:"display_name"`
	Scope           string     `json:"scope"`
	Type            string     `json:"type"`
	CryptoAlgorithm string     `json:"crypto_algorithm"`
	BitSize         int        `json:"bit_size"`
	ExpireInYears   int        `json:"expire_in_years"`
	Certificate     string     `json:"certificate,omitempty"`
	HasPrivateKey   bool       `json:"has_private_key"`
	ExpireTime      *time.Time `json:"expire_time,omitempty"`
	IsSystem        bool       `json:"is_system"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func toCertDTO(c *repository.Cert) *certDTO {
	if c == nil {
		return nil
	}
	return &certDTO{
		ID: c.ID, InstitutionID: c.InstitutionID, Name: c.Name, DisplayName: c.DisplayName,
		Scope: c.Scope, Type: c.Type, CryptoAlgorithm: c.CryptoAlgorithm, BitSize: c.BitSize,
		ExpireInYears: c.ExpireInYears, Certificate: c.Certificate, HasPrivateKey: c.PrivateKey != "",
		ExpireTime: c.ExpireTime, IsSystem: c.IsSystem, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func toCertDTOs(items []*repository.Cert) []*certDTO {
	out := make([]*certDTO, 0, len(items))
	for _, c := range items {
		out = append(out, toCertDTO(c))
	}
	return out
}

// tokenDTO 令牌响应 DTO（令牌明文不下发，仅保留前缀用于识别）。
type tokenDTO struct {
	ID            int64  `json:"id"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application,omitempty"`
	User          string `json:"user,omitempty"`
	// 主键关联（Spec Q5 第 1 批）：名称为兼容字段，主键用于精确引用
	ApplicationID   *int64     `json:"application_id,omitempty"`
	UserID          *int64     `json:"user_id,omitempty"`
	Name            string     `json:"name"`
	ClientIP        string     `json:"-"`
	Scope           string     `json:"scope,omitempty"`
	TokenType       string     `json:"token_type"`
	GrantType       string     `json:"grant_type,omitempty"`
	ExpiresIn       int        `json:"expires_in"`
	Revoked         bool       `json:"revoked"`
	HasAccessToken  bool       `json:"has_access_token"`
	HasRefreshToken bool       `json:"has_refresh_token"`
	HasIDToken      bool       `json:"has_id_token"`
	ExpireTime      *time.Time `json:"expire_time,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func toTokenDTO(t *repository.Token) *tokenDTO {
	if t == nil {
		return nil
	}
	return &tokenDTO{
		ID: t.ID, InstitutionID: t.InstitutionID, Application: t.Application, User: t.User,
		ApplicationID: t.ApplicationID, UserID: t.UserID,
		Name: t.Name, Scope: t.Scope, TokenType: t.TokenType, GrantType: t.GrantType,
		ExpiresIn: t.ExpiresIn, Revoked: t.Revoked,
		HasAccessToken: t.AccessToken != "", HasRefreshToken: t.RefreshToken != "",
		HasIDToken: t.IDToken != "", ExpireTime: t.ExpireTime, CreatedAt: t.CreatedAt,
	}
}

func toTokenDTOs(items []*repository.Token) []*tokenDTO {
	out := make([]*tokenDTO, 0, len(items))
	for _, t := range items {
		out = append(out, toTokenDTO(t))
	}
	return out
}

// ── 小工具（避免下发 null，保证前端拿到数组/对象）──────

func orEmptySlice(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func orEmptyItems(v []repository.ProviderItem) []repository.ProviderItem {
	if v == nil {
		return []repository.ProviderItem{}
	}
	return v
}

func orEmptyMethods(v []repository.SigninMethod) []repository.SigninMethod {
	if v == nil {
		return []repository.SigninMethod{}
	}
	return v
}

func orEmptySignupItems(v []repository.SignupItem) []repository.SignupItem {
	if v == nil {
		return []repository.SignupItem{}
	}
	return v
}

func orEmptyScopes(v []repository.ScopeItem) []repository.ScopeItem {
	if v == nil {
		return []repository.ScopeItem{}
	}
	return v
}

func orEmptyMapCopy(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}
