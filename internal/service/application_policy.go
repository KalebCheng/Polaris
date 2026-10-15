package service

import (
	"context"
	"net"
	"strings"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// 本文件集中实现"应用（Application）配置在运行时的真实生效逻辑"。
// 原则：后台能配置的字段必须影响实际行为，避免出现"配了但不生效"的死配置。

// AppInfo 登录页所需的公开应用信息（不含任何密钥）。
type AppInfo struct {
	// Institution 组织标识（组织 name）：兼容旧客户端；新客户端请用 InstitutionID（主键）。
	// 历史教训：这里曾写成驼峰 `institutionID`（名字像 id、值却是 name），前端读 `info.institution`
	// 永远拿到 undefined，于是"应用驱动登录页"虽渲染出了组织名/Logo，却始终不往登录、授权请求里带组织。
	Institution string `json:"institution"`
	// InstitutionID 组织主键：公开面组织引用的**权威值**（登录/发码/授权请求都传它）
	InstitutionID int64                 `json:"institution_id"`
	Code          string                `json:"code"`
	Name          string                `json:"name"`
	Logo          string                `json:"logo,omitempty"`
	Favicon       string                `json:"favicon,omitempty"`
	Title         string                `json:"title,omitempty"`
	ThemeData     *repository.ThemeData `json:"theme_data,omitempty"`

	// 登录方式：**已按组织能力过滤**——组织缺邮件/短信提供商时对应方式不下发，
	// 前端因此只会渲染真正可用的入口（避免"配了不生效"）。
	SigninMethods       []repository.SigninMethod `json:"signin_methods"`
	SelfSignupEnabled   bool                      `json:"self_signup_enabled"`
	SignupAutoSignin    bool                      `json:"signup_auto_signin"`
	MagicLinkTTLSeconds int                       `json:"magic_link_ttl_seconds"`

	OrgChoiceMode   string                    `json:"org_choice_mode,omitempty"`
	SMSProviders    []string                  `json:"sms_providers"`
	EmailProviders  []string                  `json:"email_providers"`
	Providers       []repository.ProviderItem `json:"providers"`
	SignupItems     []repository.SignupItem   `json:"signup_items"`
	SignupURL       string                    `json:"signup_url,omitempty"`
	TermsOfUse      string                    `json:"terms_of_use,omitempty"`
	CaptchaRequired bool                      `json:"captcha_required"`

	// OrgDisplayName / OrgDescription：登录页左上角品牌名与欢迎语取自**组织**（而非平台名），
	// 仅下发展示性字段（不含任何敏感信息）。
	OrgDisplayName string `json:"org_display_name,omitempty"`
	OrgDescription string `json:"org_description,omitempty"`
	OrgLogo        string `json:"org_logo,omitempty"`
	OrgFavicon     string `json:"org_favicon,omitempty"`
}

// AppInfo 汇总登录页需要的公开信息：
// 应用策略 + 该应用启用的身份提供方 + 组织可用的短信/邮件/验证码能力开关。
func (s *AuthService) AppInfo(ctx context.Context, institution string, application string) (*AppInfo, error) {
	app, err := s.resolveApplication(ctx, institution, application)
	if err != nil {
		return nil, err
	}
	return s.buildAppInfo(ctx, app), nil
}

// InstitutionNameByID 把组织主键解析为组织标识（公开面以主键为权威引用，服务层内部仍以标识定位）。
func (s *AuthService) InstitutionNameByID(ctx context.Context, id int64) (string, error) {
	org, err := s.orgs.GetByID(ctx, id)
	if err != nil || org == nil {
		return "", bizerrors.ErrNotFound.WithDetail("institution_id", id)
	}
	return org.Name, nil
}

// AppInfoByClientID 按 client_id 解析应用并汇总登录页信息（应用驱动登录页：/oauth/authorize 携带 client_id）。
func (s *AuthService) AppInfoByClientID(ctx context.Context, clientID string) (*AppInfo, error) {
	if clientID == "" {
		return nil, bizerrors.New(bizerrors.CodeApplicationNotFound, "application not found")
	}
	app, err := s.apps.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("client_id", clientID)
	}
	return s.buildAppInfo(ctx, app), nil
}

func (s *AuthService) buildAppInfo(ctx context.Context, app *repository.Application) *AppInfo {
	info := &AppInfo{
		Institution:         "",
		InstitutionID:       app.InstitutionID,
		Code:                app.Code,
		Name:                orDefault(app.Name, app.Code),
		Logo:                app.Logo,
		Favicon:             app.Favicon,
		Title:               app.Title,
		ThemeData:           app.ThemeData,
		SelfSignupEnabled:   app.SelfSignupEnabled,
		SignupAutoSignin:    app.SignupAutoSignin,
		MagicLinkTTLSeconds: app.MagicLinkTTLSeconds,
		OrgChoiceMode:       app.OrgChoiceMode,
		Providers:           orEmptyItems(app.Providers),
		SignupItems:         orEmptySignupItems(app.SignupItems),
		SignupURL:           app.SignupURL,
		TermsOfUse:          app.TermsOfUse,
	}
	if org, err := s.orgs.GetByID(ctx, app.InstitutionID); err == nil {
		info.Institution = org.Name
		info.OrgDisplayName = orDefault(org.DisplayName, org.Name)
		info.OrgDescription = org.Description
		info.OrgLogo = org.Logo
		info.OrgFavicon = org.Favicon
	}
	if s.providerRuntime != nil {
		info.SMSProviders, info.EmailProviders, info.CaptchaRequired = s.providerCapabilities(ctx, app.InstitutionID)
	}
	info.SigninMethods = availableSigninMethods(app, info.EmailProviders, info.SMSProviders)
	return info
}

// implementedSigninMethods 已实现运行时的登录方式白名单。
// Guest（匿名会话）尚未实现，故不在列——即使被写进配置也不会下发到登录页。
var implementedSigninMethods = map[string]bool{
	repository.SigninMethodPassword:  true,
	repository.SigninMethodEmailCode: true,
	repository.SigninMethodSMSCode:   true,
	repository.SigninMethodMagicLink: true,
	repository.SigninMethodProvider:  true,
}

// availableSigninMethods 依据组织能力过滤登录方式（不可用的方式不下发）：
//   - 邮箱验证码 / 魔法链接：需要组织启用邮件提供商；
//   - 短信验证码：需要组织启用短信提供商；
//   - 第三方：需要应用自身配置了提供方。
func availableSigninMethods(app *repository.Application, email, sms []string) []repository.SigninMethod {
	// 与 HasSigninMethod 保持一致：signin_methods 为空的历史应用兜底为"仅密码"，
	// 避免登录页无任何入口而后端却仍放行密码登录（两边语义必须一致）。
	if len(app.SigninMethods) == 0 {
		return []repository.SigninMethod{{Name: repository.SigninMethodPassword, DisplayName: "Password", Rule: "All"}}
	}
	out := []repository.SigninMethod{}
	for _, m := range app.SigninMethods {
		if !implementedSigninMethods[m.Name] {
			continue // 尚未实现运行时的登录方式不下发（避免"配了不生效"）
		}
		switch m.Name {
		case repository.SigninMethodEmailCode, repository.SigninMethodMagicLink:
			if len(email) == 0 {
				continue
			}
		case repository.SigninMethodSMSCode:
			if len(sms) == 0 {
				continue
			}
		case repository.SigninMethodProvider:
			if len(app.Providers) == 0 {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// providerCapabilities 返回组织内可用于验证码登录/邮件的提供商名，以及是否要求人机校验。
// 数据源与通知发送链路**同源**（provider.Runtime），避免"这里说有、发码时说没有"。
func (s *AuthService) providerCapabilities(ctx context.Context, institutionID int64) (sms, email []string, captcha bool) {
	sms, email = []string{}, []string{}
	if s.providerRuntime == nil {
		return sms, email, false
	}
	for _, category := range []providerpkg.Category{
		providerpkg.CategorySMS, providerpkg.CategoryEmail, providerpkg.CategoryCaptcha,
	} {
		items, err := s.providerRuntime.ListActive(ctx, institutionID, category)
		if err != nil {
			continue
		}
		for _, item := range items {
			switch category {
			case providerpkg.CategorySMS:
				sms = append(sms, item.Name)
			case providerpkg.CategoryEmail:
				email = append(email, item.Name)
			case providerpkg.CategoryCaptcha:
				captcha = true
			}
		}
	}
	return sms, email, captcha
}

// resolveOrgForLogin 解析登录目标组织实体。
//
// 认证入口（登录 / 注册 / 应用信息）对外仍是**组织标识**（名字或域名）——用户看到的是组织名，
// 不是主键；解析成实体后，内部一律用 org.ID 访问数据（库层已是 id 语义）。
func (s *AuthService) resolveOrgForLogin(ctx context.Context, institutionName string) (*repository.Institution, error) {
	name := institutionName
	if name == "" {
		name = s.oauthCfg.DefaultInstitution
	}
	org, err := s.orgs.GetByName(ctx, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", name)
	}
	return org, nil
}

// resolveApplication 解析目标应用：未指定时回退到组织的默认应用（再回退到该组织第一个应用）。
func (s *AuthService) resolveApplication(ctx context.Context, institutionName, application string) (*repository.Application, error) {
	org, err := s.resolveOrgForLogin(ctx, institutionName)
	if err != nil {
		return nil, err
	}
	if application == "" {
		application = org.DefaultApplication
	}
	if application == "" {
		apps, err := s.apps.List(ctx, repository.ListQuery{InstitutionID: org.ID, Limit: 1})
		if err != nil || len(apps) == 0 {
			return nil, bizerrors.New(bizerrors.CodeApplicationNotFound, "application not found").
				WithDetail("institution_id", org.Name)
		}
		return apps[0], nil
	}
	app, err := s.apps.GetByCode(ctx, org.ID, application)
	if err != nil {
		return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).
			WithDetail("institution_id", org.Name).WithDetail("application", application)
	}
	return app, nil
}

// resolveApplicationOptional 宽松解析目标应用，供**登录 / 注册**等"应用级策略可选"的路径使用：
// 组织尚未配置任何应用时返回 (nil, nil)，由调用方跳过应用级策略，而不是让登录失败。
//   - 显式指定了应用名却不存在：仍返回 APPLICATION_NOT_FOUND（应用驱动场景需明确失败）；
//   - 未显式指定：默认应用（可能已被删除 → 忽略）→ 组织首个应用 → 都没有则返回 nil。
func (s *AuthService) resolveApplicationOptional(ctx context.Context, institutionName, application string) (*repository.Application, error) {
	org, err := s.resolveOrgForLogin(ctx, institutionName)
	if err != nil {
		return nil, nil // 组织不存在时交由调用方处理（登录已单独校验组织）
	}
	if application != "" {
		app, err := s.apps.GetByCode(ctx, org.ID, application)
		if err != nil {
			return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).
				WithDetail("institution_id", org.Name).WithDetail("application", application)
		}
		return app, nil
	}
	if org.DefaultApplication != "" {
		if app, err := s.apps.GetByCode(ctx, org.ID, org.DefaultApplication); err == nil {
			return app, nil
		}
	}
	apps, err := s.apps.List(ctx, repository.ListQuery{InstitutionID: org.ID, Limit: 1})
	if err != nil || len(apps) == 0 {
		return nil, nil
	}
	return apps[0], nil
}

// enforceSigninPolicy 校验应用级登录策略（禁用登录 / 是否允许密码登录 / IP 限制）。
func (s *AuthService) enforceSigninPolicy(app *repository.Application, ip string) error {
	if app == nil {
		return nil
	}
	if app.State != "" && app.State != "active" {
		return bizerrors.New(bizerrors.CodeForbidden, "application is disabled")
	}
	if !ipAllowed(app.IPWhitelist, ip) || !ipAllowed(app.IPRestriction, ip) {
		return bizerrors.New(bizerrors.CodeForbidden, "sign-in from this ip is not allowed").WithDetail("ip", ip)
	}
	return nil
}

// ipAllowed 校验 IP 是否命中白名单；空列表表示不限制。
// 支持精确 IP 与 CIDR（如 10.0.0.0/8）；配置项既用于白名单也用于组织级限制。
func ipAllowed(csv string, ip string) bool {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return true
	}
	target := net.ParseIP(strings.TrimSpace(ip))
	if target == nil {
		return false
	}
	for _, entry := range strings.Split(csv, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			if network.Contains(target) {
				return true
			}
			continue
		}
		if parsed := net.ParseIP(entry); parsed != nil && parsed.Equal(target) {
			return true
		}
	}
	return false
}

// enforceSessionPolicy 依据应用配置收敛用户会话数量。
//
// 单一字段 MaxSessions 表达全部语义（见迁移 015：独占登录已合并进来）：
//   - 0：不限制（返回，不做任何清理）；
//   - 1：独占——新登录踢掉该用户在同一应用下的其他会话；
//   - N：最多保留最近 N 个（含本次新会话），更早的下线。
func (s *AuthService) enforceSessionPolicy(ctx context.Context, app *repository.Application, user *repository.User, keepSessionID string) {
	if app == nil || app.MaxSessions == 0 {
		return // 未配置则不限制
	}
	ids, err := s.sessions.UserSessions(ctx, user.InstitutionID, user.Name)
	if err != nil {
		return
	}
	type candidate struct {
		id        string
		createdAt int64
	}
	var others []candidate
	for _, id := range ids {
		if id == keepSessionID {
			continue
		}
		session, err := s.sessions.Load(ctx, id)
		if err != nil {
			continue
		}
		if app.Code != "" && session.Application != "" && session.Application != app.Code {
			continue // 仅统计同一应用下的会话
		}
		others = append(others, candidate{id: id, createdAt: session.CreatedAt})
	}
	// 保留最近 (MaxSessions-1) 个，其余下线（新会话已占用 1 个名额）
	limit := app.MaxSessions - 1
	if limit < 0 {
		limit = 0
	}
	if len(others) <= limit {
		return
	}
	// 按创建时间升序，先淘汰最旧的（保持稳定：相等时按原顺序）
	for i := 0; i < len(others); i++ {
		for j := i + 1; j < len(others); j++ {
			if others[j].createdAt < others[i].createdAt {
				others[i], others[j] = others[j], others[i]
			}
		}
	}
	for _, item := range others[:len(others)-limit] {
		s.dropSession(ctx, user, item.id)
	}
}

// dropSession 删除指定会话（仅清理会话本体与用户索引）。
func (s *AuthService) dropSession(ctx context.Context, user *repository.User, sessionID string) {
	_ = s.sessions.Drop(ctx, user.InstitutionID, user.Name, sessionID)
}

// validateAppScopes 校验请求的 scope 是否在应用允许列表内（openid 恒定允许）。
func validateAppScopes(app *repository.Application, requested string) error {
	if app == nil || len(app.Scopes) == 0 {
		return nil // 应用未限定 scope 时不校验
	}
	allowed := map[string]bool{"openid": true, "offline_access": true}
	for _, scope := range app.Scopes {
		allowed[scope.Name] = true
	}
	for _, scope := range strings.Fields(requested) {
		if !allowed[scope] {
			return bizerrors.New(bizerrors.CodeOAuthInvalidScope, "scope is not granted to the application").
				WithDetail("scope", scope)
		}
	}
	return nil
}

// sanitizeTokenExtra 依据应用的 token_fields 白名单裁剪 access_token 内嵌字段；
// 未配置时返回默认字段（用户名与所属组织）。
func sanitizeTokenExtra(app *repository.Application, user *repository.User) map[string]interface{} {
	all := map[string]interface{}{
		"username": user.Name,
		"org":      user.InstitutionID,
		"email":    user.Email,
		"phone":    user.Phone,
		"name":     user.DisplayName,
		"is_admin": user.IsAdmin,
		"type":     user.Type,
	}
	if app == nil || len(app.TokenFields) == 0 {
		return map[string]interface{}{"username": user.Name, "org": user.InstitutionID}
	}
	out := make(map[string]interface{}, len(app.TokenFields))
	for _, field := range app.TokenFields {
		if value, ok := all[field]; ok {
			out[field] = value
		}
	}
	return out
}

// accessTokenTTL 解析应用级 access_token 有效期。
func accessTokenTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.ExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.ExpireInHours * float64(time.Hour))
}

// refreshTokenTTL 解析应用级 refresh_token 有效期。
func refreshTokenTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.RefreshExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.RefreshExpireInHours * float64(time.Hour))
}

// cookieTTL 解析应用级会话 Cookie 有效期；未配置时回退到全局会话 TTL。
func cookieTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.CookieExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.CookieExpireInHours) * time.Hour
}

// ── 应用适配器（P4，D6）────────────────────────────────────────

// userAttrNames 自定义适配器可映射的用户属性白名单（本地属性 → 取值）。
// 未收录的属性名一律忽略，避免任意取值/越权暴露。
var userAttrNames = map[string]func(u *repository.User) interface{}{
	"name":               func(u *repository.User) interface{} { return u.Name },
	"username":           func(u *repository.User) interface{} { return u.Name },
	"uid":                func(u *repository.User) interface{} { return u.UID },
	"display_name":       func(u *repository.User) interface{} { return firstNonEmptyStr(u.DisplayName, u.Name) },
	"email":              func(u *repository.User) interface{} { return u.Email },
	"email_verified":     func(u *repository.User) interface{} { return u.EmailVerified },
	"phone":              func(u *repository.User) interface{} { return u.Phone },
	"phone_verified":     func(u *repository.User) interface{} { return u.PhoneVerified },
	"avatar":             func(u *repository.User) interface{} { return u.Avatar },
	"picture":            func(u *repository.User) interface{} { return u.Avatar },
	"language":           func(u *repository.User) interface{} { return u.Language },
	"locale":             func(u *repository.User) interface{} { return u.Language },
	"institutionID":      func(u *repository.User) interface{} { return u.InstitutionID },
	"org":                func(u *repository.User) interface{} { return u.InstitutionID },
	"is_admin":           func(u *repository.User) interface{} { return u.IsAdmin },
	"type":               func(u *repository.User) interface{} { return u.Type },
	"first_name":         func(u *repository.User) interface{} { return u.FirstName },
	"last_name":          func(u *repository.User) interface{} { return u.LastName },
	"title":              func(u *repository.User) interface{} { return u.Title },
	"affiliation":        func(u *repository.User) interface{} { return u.Affiliation },
	"homepage":           func(u *repository.User) interface{} { return u.Homepage },
	"location":           func(u *repository.User) interface{} { return u.Location },
	"address":            func(u *repository.User) interface{} { return u.Address },
	"country_code":       func(u *repository.User) interface{} { return u.CountryCode },
	"gender":             func(u *repository.User) interface{} { return u.Gender },
	"birthday":           func(u *repository.User) interface{} { return u.Birthday },
	"education":          func(u *repository.User) interface{} { return u.Education },
	"bio":                func(u *repository.User) interface{} { return u.Bio },
	"signup_application": func(u *repository.User) interface{} { return u.SignupApplication },
}

// resolveUserAttr 按属性名取用户值；未知属性返回 (nil,false)。
func resolveUserAttr(u *repository.User, attr string) (interface{}, bool) {
	getter, ok := userAttrNames[attr]
	if !ok {
		return nil, false
	}
	return getter(u), true
}

// applyAdapter 依据应用的适配器配置覆盖 claims：
//   - adapter != "custom" 或无 claimMap：保持原样（标准 OIDC claims，default 行为）。
//   - custom：对 claimMap 中每个 <claim> → <userAttr>，用用户属性值覆盖/新增该 claim；
//     空值（零值字符串 / false）不写入，避免用空值覆盖默认 claim；未知属性忽略。
//
// 生效点：id_token 与 userinfo 的签发路径（配置即生效）。
func applyAdapter(app *repository.Application, user *repository.User, claims map[string]interface{}) {
	if app == nil || user == nil || app.Adapter != "custom" || app.AdapterConfig == nil {
		return
	}
	for claim, attr := range app.AdapterConfig.ClaimMap {
		claim = strings.TrimSpace(claim)
		attr = strings.TrimSpace(attr)
		if claim == "" || attr == "" {
			continue
		}
		value, ok := resolveUserAttr(user, attr)
		if !ok || isEmptyClaimValue(value) {
			continue
		}
		claims[claim] = value
	}
}

// isEmptyClaimValue 判定是否为空值（不覆盖既有 claim）。
func isEmptyClaimValue(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case bool:
		return !t
	default:
		return false
	}
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func orEmptyItems(items []repository.ProviderItem) []repository.ProviderItem {
	if items == nil {
		return []repository.ProviderItem{}
	}
	return items
}

func orEmptyMethods(items []repository.SigninMethod) []repository.SigninMethod {
	if items == nil {
		return []repository.SigninMethod{}
	}
	return items
}

func orEmptySignupItems(items []repository.SignupItem) []repository.SignupItem {
	if items == nil {
		return []repository.SignupItem{}
	}
	return items
}
