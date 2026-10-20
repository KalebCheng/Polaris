package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	"polaris/internal/auth/oauth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

type ssoState struct {
	// InstitutionID 使回调能在**同一组织**内解析出同一个提供商
	// （提供商名称只在组织内唯一，跨组织同名是允许的）
	InstitutionID int64  `json:"institution_id"`
	Provider      string `json:"provider"`
	// Application 应用标识（应用驱动入口发起 SSO 时带上）：回调建立会话前用它做应用准入判定；
	// 空 = 平台级 SSO 登录，不判定（Spec FR-3 判定作用域）。
	Application string `json:"application,omitempty"`
	From        string `json:"from"`
	RedirectURI string `json:"redirect_uri"`
	LinkSession string `json:"link_session,omitempty"`
	// PKCEVerifier 启用 PKCE 时随 state 暂存（回调时作为 code_verifier 提交）
	PKCEVerifier string `json:"pkce_verifier,omitempty"`
}

// ListSigninProviders 返回组织内启用的外部身份提供方（登录页用）。
//
// 数据源是 providers 表（category=oauth、state=active）——自第四阶段起，
// 控制台「提供商」是外部身份提供方的唯一入口；institutionID 为 0 时跨组织返回
// （登录页尚未选择组织时的通用入口）。
func (s *AuthService) ListSigninProviders(ctx context.Context, institutionID int64) []auth.OAuthProviderInfo {
	if s.providerRuntime == nil {
		return nil
	}
	items, err := s.providerRuntime.ListActive(ctx, institutionID, providerpkg.CategoryOAuth)
	if err != nil {
		logger.Warn("list signin providers failed", "institution_id", institutionID, "error", err)
		return nil
	}
	out := make([]auth.OAuthProviderInfo, 0, len(items))
	for _, item := range items {
		kind := providerpkg.Kind(item)
		icon := kind
		if info, ok := providerpkg.Find(providerpkg.CategoryOAuth, kind); ok && info.Icon != "" {
			icon = info.Icon
		}
		out = append(out, auth.OAuthProviderInfo{
			Name: item.Name, DisplayName: signinDisplayName(item), Type: kind, Icon: icon,
		})
	}
	return out
}

// signinDisplayName 展示名兜底：未填名称时用目录里的类型名（钉钉 / 标准 OIDC …）
func signinDisplayName(item *repository.Provider) string {
	if strings.TrimSpace(item.DisplayName) != "" {
		return item.DisplayName
	}
	if info, ok := providerpkg.Find(providerpkg.Category(item.Category), providerpkg.Kind(item)); ok {
		return info.Name
	}
	return item.Name
}

// ListAppProviders 返回指定应用可用的外部身份提供方：
// 应用配置了 providers 白名单时只返回其中已启用且真实存在的；未配置时返回该组织全部启用提供方。
func (s *AuthService) ListAppProviders(ctx context.Context, institution string, application string) []auth.OAuthProviderInfo {
	app, err := s.resolveApplication(ctx, institution, application)
	if err != nil || app == nil {
		// 明确指定了应用却解析不到：**不给任何入口**（fail-closed）。
		// 曾经的实现会退化成"列出组织内全部提供方"，于是随便传一个不存在的应用标识
		// 就能绕过应用白名单拿到全部第三方入口。
		if strings.TrimSpace(application) != "" {
			return []auth.OAuthProviderInfo{}
		}
		// 未指定应用 = 组织级登录页（该组织可能还没建应用）→ 按组织列出可用提供方
		if strings.TrimSpace(institution) == "" {
			return s.ListSigninProviders(ctx, 0)
		}
		org, orgErr := s.resolveOrgForLogin(ctx, institution)
		if orgErr != nil {
			return []auth.OAuthProviderInfo{}
		}
		return s.ListSigninProviders(ctx, org.ID)
	}
	return appSigninProviders(s.ListSigninProviders(ctx, app.InstitutionID), app)
}

// ListAppProvidersByClientID 按 client_id 定位应用后过滤其启用的提供方（应用驱动登录页）。
func (s *AuthService) ListAppProvidersByClientID(ctx context.Context, clientID string) []auth.OAuthProviderInfo {
	if clientID == "" {
		return s.ListSigninProviders(ctx, 0)
	}
	app, err := s.apps.GetByClientID(ctx, clientID)
	if err != nil || app == nil {
		// client_id 无效：登录页本来就没有可信的应用上下文，宁可不给入口（fail-closed）
		return []auth.OAuthProviderInfo{}
	}
	return appSigninProviders(s.ListSigninProviders(ctx, app.InstitutionID), app)
}

// appSigninProviders 是"某个应用到底有哪些第三方登录入口"的**唯一判定入口**。
//
// 语义（与写入侧的 validateApplication「启用第三方登录必须带具体提供方」、
// 下发侧的 availableSigninMethods「providers 为空则不下发 Provider 登录方式」保持一致）：
//
//	app == nil（组织级登录页，没有应用上下文）      → 组织内全部启用的提供方
//	应用未启用 Provider 登录方式 / providers 为空   → 该应用**没有**第三方入口
//	否则                                          → 按白名单过滤，只保留 canSignIn 的项
//
// 历史缺陷：`providers` 为空时曾被当作"全都要"，于是**没配钉钉的应用，登录页却出现钉钉按钮**——
// 空的语义只能是"一个都不要"（对齐 §3.5 对"空值含义必须唯一"的要求），
// 否则"配了才生效"这条原则在读取侧就被推翻了。
func appSigninProviders(all []auth.OAuthProviderInfo, app *repository.Application) []auth.OAuthProviderInfo {
	if app == nil {
		return all
	}
	if len(app.Providers) == 0 || !app.HasSigninMethod(repository.SigninMethodProvider) {
		return []auth.OAuthProviderInfo{}
	}
	return filterProviders(all, app.Providers)
}

// filterProviders 按应用的提供方白名单过滤（白名单项以名称引用提供商）
func filterProviders(all []auth.OAuthProviderInfo, whitelist []repository.ProviderItem) []auth.OAuthProviderInfo {
	enabled := make(map[string]bool, len(whitelist))
	for _, item := range whitelist {
		enabled[item.Name] = item.CanSignIn
	}
	filtered := make([]auth.OAuthProviderInfo, 0, len(all))
	for _, item := range all {
		if enabled[item.Name] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// InstitutionOption 登录页可选项（仅暴露名称，不泄露组织配置）。
type InstitutionOption struct {
	// ID 组织主键：登录页据此以 institution_id 发起登录（公开面引用一律用主键）
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// InstitutionOptions 返回公开的组织选项（供登录页选择登录域）。
// 只暴露 id / name / display_name，避免未认证请求读到组织的安全配置。
func (s *AuthService) InstitutionOptions(ctx context.Context) ([]InstitutionOption, error) {
	orgs, err := s.orgs.List(ctx, repository.ListQuery{Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]InstitutionOption, 0, len(orgs))
	for _, org := range orgs {
		if org.DisableSignin {
			continue // 禁止登录的组织不在登录页展示
		}
		out = append(out, InstitutionOption{ID: org.ID, Name: org.Name, DisplayName: org.DisplayName})
	}
	return out, nil
}

// Authorize 生成外部 IdP 授权地址，并把 from 等上下文写入 state。
//
// institution 为组织标识（可空：此时按"唯一启用的同名提供商"解析）；提供商名称只在组织内唯一，
// 因此解析出的 organizationID 会随 state 暂存，回调时必须回到同一个提供商。
// Authorize 发起 SSO 授权。
//
// application / clientID 是**应用上下文**（登录页由 ?client_id= 驱动，旧链接可能只带 ?application=），
// 随 state 暂存并在回调建会话前用于应用准入判定（Spec FR-3）。
func (s *AuthService) Authorize(ctx context.Context, institution, providerName, application, clientID, from, redirectOverride string) (string, string, error) {
	if s.oauthResolver == nil {
		return "", "", bizerrors.ErrProviderUnavailable.WithDetail("provider", providerName)
	}
	institutionID, err := s.resolveSigninInstitutionID(ctx, institution, providerName)
	if err != nil {
		return "", "", err
	}
	connector, item, err := s.oauthResolver.GetBySignin(ctx, institutionID, providerName)
	if err != nil {
		return "", "", err
	}
	redirectURI := s.resolveRedirectURI(item, redirectOverride)
	app, err := s.resolveAppContext(ctx, institutionID, institution, application, clientID)
	if err != nil {
		return "", "", err
	}
	payload := ssoState{
		InstitutionID: institutionID, Provider: providerName,
		Application: applicationCode(app),
		From:        sanitizeFrom(from), RedirectURI: redirectURI,
	}
	authorizeURL := connector.AuthCodeURL(state0, redirectURI)
	if item.EnablePKCE {
		if pkce, ok := connector.(oauth.PKCEProvider); ok {
			verifier, verr := providerpkg.NewPKCEVerifier()
			if verr != nil {
				return "", "", bizerrors.Wrap(bizerrors.CodeOAuthProviderError, "failed to initialize pkce", verr)
			}
			payload.PKCEVerifier = verifier
			authorizeURL = pkce.AuthCodeURLWithPKCE(state0, redirectURI, providerpkg.PKCEChallenge(verifier))
		}
	}
	return s.saveStateAndURL(ctx, payload, authorizeURL)
}

// AuthorizeIdentityLink 发起当前登录用户的外部身份绑定流程。
func (s *AuthService) AuthorizeIdentityLink(ctx context.Context, institution, providerName, sessionID string) (string, string, error) {
	if sessionID == "" {
		return "", "", bizerrors.ErrUnauthorized
	}
	if s.oauthResolver == nil {
		return "", "", bizerrors.ErrProviderUnavailable.WithDetail("provider", providerName)
	}
	// 绑定流程的组织取自当前会话（组织内资源不得跨组织绑定）
	session, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return "", "", bizerrors.ErrUnauthorized
	}
	institutionID := session.InstitutionID
	if institutionID == 0 {
		institutionID, err = s.resolveSigninInstitutionID(ctx, institution, providerName)
		if err != nil {
			return "", "", err
		}
	}
	connector, item, err := s.oauthResolver.GetBySignin(ctx, institutionID, providerName)
	if err != nil {
		return "", "", err
	}
	redirectURI := s.resolveRedirectURI(item, "")
	payload := ssoState{
		InstitutionID: institutionID, Provider: providerName,
		From: "/portal/identities?linked=1", RedirectURI: redirectURI, LinkSession: sessionID,
	}
	authorizeURL := connector.AuthCodeURL(state0, redirectURI)
	if item.EnablePKCE {
		if pkce, ok := connector.(oauth.PKCEProvider); ok {
			verifier, verr := providerpkg.NewPKCEVerifier()
			if verr != nil {
				return "", "", bizerrors.Wrap(bizerrors.CodeOAuthProviderError, "failed to initialize pkce", verr)
			}
			payload.PKCEVerifier = verifier
			authorizeURL = pkce.AuthCodeURLWithPKCE(state0, redirectURI, providerpkg.PKCEChallenge(verifier))
		}
	}
	return s.saveStateAndURL(ctx, payload, authorizeURL)
}

// state0 授权地址里的占位 state，真实 state 在 saveStateAndURL 内生成后替换。
const state0 = "__polaris_state__"

// saveStateAndURL 生成一次性 state、暂存载荷并替换授权地址中的占位 state。
func (s *AuthService) saveStateAndURL(ctx context.Context, payload ssoState, authorizeURL string) (string, string, error) {
	state := uuid.NewString()
	raw, err := jsonMarshal(payload)
	if err != nil {
		return "", "", bizerrors.Wrap(bizerrors.CodeInternalError, "failed to encode sso state", err)
	}
	ttl := time.Duration(s.oauthCfg.StateTTL) * time.Second
	if ttl <= 0 {
		ttl = 600 * time.Second
	}
	if err := s.states.Save(ctx, state, raw, ttl); err != nil {
		return "", "", err
	}
	logger.Info("sso state prepared",
		"state", state, "provider", payload.Provider, "institution_id", payload.InstitutionID,
		"redirect_uri", payload.RedirectURI, "ttl", ttl.String())
	return strings.Replace(authorizeURL, state0, state, 1), state, nil
}

// resolveSigninInstitutionID 解析发起 SSO 的组织主键：
// 显式组织名 → 唯一启用的同名提供商 → 平台默认组织。
func (s *AuthService) resolveSigninInstitutionID(ctx context.Context, institution, providerName string) (int64, error) {
	if institution != "" {
		org, err := s.resolveOrgForLogin(ctx, institution)
		if err != nil {
			return 0, bizerrors.ErrNotFound.WithDetail("institution", institution)
		}
		return org.ID, nil
	}
	if s.providerRuntime != nil {
		items, err := s.providerRuntime.ListActive(ctx, 0, providerpkg.CategoryOAuth)
		if err == nil {
			matched := make([]*repository.Provider, 0, 2)
			for _, item := range items {
				if item.Name == providerName {
					matched = append(matched, item)
				}
			}
			if len(matched) == 1 {
				return matched[0].InstitutionID, nil
			}
		}
	}
	if s.oauthCfg.DefaultInstitution != "" {
		if org, err := s.resolveOrgForLogin(ctx, s.oauthCfg.DefaultInstitution); err == nil {
			return org.ID, nil
		}
	}
	return 0, bizerrors.New(bizerrors.CodeInvalidParams,
		"institution is required to start single sign-on").WithDetail("provider", providerName)
}

// Callback 处理外部 IdP 回调：登录或将身份绑定到发起流程的有效会话。
func (s *AuthService) Callback(ctx context.Context, code, state, ip, userAgent string) (*auth.LoginResponse, string, error) {
	logger.Info("sso callback received", "state", state, "code_len", len(code))
	raw, err := s.states.Take(ctx, state)
	if err != nil {
		logger.Warn("sso state take failed", "state", state, "error", err)
		return nil, "", err
	}
	var st ssoState
	if err := jsonUnmarshal(raw, &st); err != nil {
		return nil, "", bizerrors.ErrSSOStateInvalid
	}
	connector, item, err := s.oauthResolver.GetBySignin(ctx, st.InstitutionID, st.Provider)
	if err != nil {
		return nil, "", err
	}
	info, err := exchangeCode(ctx, connector, st, code)
	if err != nil {
		logger.Warn("sso exchange failed", "provider", st.Provider, "error", err)
		return nil, "", bizerrors.Wrap(bizerrors.CodeOAuthProviderError, "identity provider exchange failed", err)
	}
	if info.ProviderUserID == "" {
		return nil, "", bizerrors.New(bizerrors.CodeOAuthProviderError, "identity provider returned empty subject")
	}
	if st.LinkSession != "" {
		return s.completeIdentityLink(ctx, st.LinkSession, st.Provider, info)
	}
	user, created, err := s.provision(ctx, item, info)
	if err != nil {
		return nil, "", err
	}
	// 应用准入：应用上下文来自 SSO state（平台级 SSO 无该字段 → 不判定）。
	ssoApp, err := s.resolveAppContext(ctx, st.InstitutionID, "", st.Application, "")
	if err != nil {
		return nil, "", err
	}
	if err := s.enforceAppAccess(ctx, ssoApp, user); err != nil {
		return nil, "", err
	}
	session, err := s.sessions.Create(ctx, user, applicationCode(ssoApp), ip, userAgent)
	if err != nil {
		return nil, "", err
	}
	challenge, _, _, err := s.applyMFA(ctx, session, ssoApp, repository.SigninMethodProvider, user, "", "", false)
	if err != nil {
		_ = s.sessions.Delete(ctx, session)
		return nil, "", err
	}
	if !session.MfaPending {
		_ = s.users.UpdateLastSignin(ctx, user.ID, time.Now(), ip)
		_ = s.users.SetOnline(ctx, user.ID, true)
		s.audits.Record(ctx, user.Name, user.InstitutionID, "auth.sso_login", user.Name, map[string]interface{}{"provider": st.Provider})
		// 新建用户：触发"创建即建号"的账号策略（与管理员建号一致）
		if created && s.provisioning != nil {
			s.provisioning.TriggerUserProvisioning(ctx, user.InstitutionID, user.Name)
		}
	}
	resp := s.loginResponse(user, session, st.From)
	resp.Mfa = challenge
	return resp, session.ID, nil
}

// exchangeCode 用授权码换取用户信息；state 里带了 PKCE verifier 时走 PKCE 交换。
func exchangeCode(ctx context.Context, connector oauth.Provider, st ssoState, code string) (*oauth.UserInfo, error) {
	if st.PKCEVerifier != "" {
		if pkce, ok := connector.(oauth.PKCEProvider); ok {
			return pkce.ExchangeWithPKCE(ctx, code, st.RedirectURI, st.PKCEVerifier)
		}
	}
	return connector.Exchange(ctx, code, st.RedirectURI)
}

func (s *AuthService) completeIdentityLink(ctx context.Context, sessionID, provider string, info *oauth.UserInfo) (*auth.LoginResponse, string, error) {
	session, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return nil, "", bizerrors.ErrUnauthorized
	}
	identity, lookupErr := s.users.GetIdentity(ctx, session.InstitutionID, provider, info.ProviderUserID)
	if lookupErr == nil {
		if identity.InstitutionID != session.InstitutionID || identity.User != session.Name {
			return nil, "", bizerrors.New(bizerrors.CodeConflict, "identity is already linked to another account")
		}
	} else if _, err := s.users.BindIdentity(ctx, &repository.UserIdentity{
		InstitutionID: session.InstitutionID, User: session.Name, Provider: provider,
		ProviderUserID: info.ProviderUserID, Email: info.Email,
		Attributes: providerIdentityAttributes(info),
	}); err != nil {
		return nil, "", err
	}
	s.audits.Record(ctx, session.Name, session.InstitutionID, "user.bind_identity", session.Name,
		map[string]interface{}{"provider": provider})
	return &auth.LoginResponse{IdentityLinked: true}, "", nil
}

// provision 按提供商行承载的信任策略创建/关联本地用户。
// 返回 created=true 表示本次新建了本地用户（用于触发"创建即建号"供给）。
//
// 策略与落地组织都取自提供商行（第四阶段之前来自 configs/app.yaml 的
// auth.oauth.providers[].provisioning 与 default_institution，控制台无法管理）。
func (s *AuthService) provision(ctx context.Context, item *repository.Provider, info *oauth.UserInfo) (*repository.User, bool, error) {
	if item == nil {
		return nil, false, bizerrors.ErrNotFound.WithDetail("provider", "")
	}
	providerName := item.Name
	org, err := s.orgs.GetByID(ctx, item.InstitutionID)
	if err != nil {
		return nil, false, bizerrors.ErrNotFound.WithDetail("institution_id", item.InstitutionID)
	}
	if len(item.AllowedDomains) > 0 && !domainAllowed(info.Email, item.AllowedDomains) {
		return nil, false, bizerrors.New(bizerrors.CodeForbidden, "email domain is not allowed")
	}

	// 1) 已绑定：直接返回
	if identity, err := s.users.GetIdentity(ctx, item.InstitutionID, providerName, info.ProviderUserID); err == nil {
		user, err := s.users.GetByName(ctx, identity.InstitutionID, identity.User)
		if err == nil {
			return user, false, nil
		}
	}

	// 2) 提供商字段 → 本地用户字段的唯一性匹配。
	// 旧 link_by_email 仅在未配置新映射时作为 email/email 兼容预设。
	matchType := item.MatchType
	matchLocal := item.MatchLocalField
	matchProvider := item.MatchProviderField
	if matchType == "" && matchLocal == "" && item.LinkByEmail {
		matchType, matchLocal, matchProvider = "system", "email", "email"
	}
	if matchType != "" {
		value := matchProviderValue(info, matchProvider)
		if value == "" {
			// 配了映射但提供商没返回该字段：拒绝登录，避免再次 JIT 出重复用户。
			return nil, false, bizerrors.ErrIdentityNotBound.WithDetail("match_provider_field", matchProvider)
		}
		var candidates []*repository.User
		var err error
		switch matchType {
		case "system":
			candidates, err = s.users.FindMatchCandidates(ctx, org.ID, matchLocal, value, 2)
		case "identity_link":
			candidates, err = s.users.FindIdentityLinkMatchCandidates(
				ctx, org.ID, item.MatchSyncerID, item.MatchSyncerField, value, 2)
		default:
			return nil, false, bizerrors.ErrInvalidParams.WithDetail("match_type", matchType)
		}
		if err != nil {
			return nil, false, err
		}
		switch len(candidates) {
		case 1:
			existing := candidates[0]
			if _, err := s.users.BindIdentity(ctx, &repository.UserIdentity{
				InstitutionID: org.ID, User: existing.Name, Provider: providerName,
				ProviderUserID: info.ProviderUserID, Email: info.Email,
				Attributes: providerIdentityAttributes(info),
			}); err != nil {
				return nil, false, err
			}
			return existing, false, nil
		case 0:
			// 继续走自动建号
		default:
			return nil, false, bizerrors.New(bizerrors.CodeIdentityAmbiguous,
				"multiple local accounts match this identity")
		}
	}

	if !item.AutoCreate {
		return nil, false, bizerrors.ErrIdentityNotBound
	}

	// 3) JIT 建号：字段来源由提供商策略声明（provision_* 字段映射），未配置时用默认字段。
	name := s.uniqueUserName(ctx, org.ID, info, item.ProvisionUsernameField)
	displayName := matchProviderValue(info, provisionSource(item.ProvisionNameField, "nick"))
	email := matchProviderValue(info, provisionSource(item.ProvisionEmailField, "email"))
	phone := matchProviderValue(info, provisionSource(item.ProvisionPhoneField, "mobile"))
	avatar := matchProviderValue(info, provisionSource(item.ProvisionAvatarField, "avatar"))
	user := &repository.User{
		UID:            uuid.NewString(),
		InstitutionID:  org.ID,
		Name:           name,
		Type:           "normal",
		DisplayName:    firstNonEmptyStr(displayName, info.DisplayName),
		Avatar:         firstNonEmptyStr(avatar, info.Avatar),
		Email:          firstNonEmptyStr(email, info.Email),
		EmailVerified:  info.Email != "",
		Phone:          firstNonEmptyStr(phone, info.Phone),
		RegisterType:   "oauth",
		RegisterSource: providerName,
	}
	if _, err := s.users.CreateWithIdentity(ctx, user, &repository.UserIdentity{
		Provider: providerName, ProviderUserID: info.ProviderUserID, Email: user.Email,
		Attributes: providerIdentityAttributes(info),
	}); err != nil {
		return nil, false, err
	}
	s.audits.Record(ctx, name, org.ID, "user.provision", name, map[string]interface{}{"provider": providerName})
	return user, true, nil
}

func (s *AuthService) uniqueUserName(ctx context.Context, institutionID int64, info *oauth.UserInfo, configuredField string) string {
	base := matchProviderValue(info, strings.TrimSpace(configuredField))
	if base == "" {
		base = info.Username
	}
	if base == "" && info.Email != "" {
		base = strings.SplitN(info.Email, "@", 2)[0]
	}
	if base == "" {
		base = "user-" + randomHex(4)
	}
	name := sanitizeName(base)
	for i := 1; i < 100; i++ {
		if _, err := s.users.GetByName(ctx, institutionID, name); err != nil {
			return name
		}
		name = sanitizeName(base) + "-" + randomHex(2)
	}
	return sanitizeName(base) + "-" + randomHex(4)
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else if r > 127 {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "user-" + randomHex(4)
	}
	return b.String()
}

func domainAllowed(email string, domains []string) bool {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	for _, d := range domains {
		if strings.EqualFold(d, parts[1]) {
			return true
		}
	}
	return false
}

// OAuthPlatformInfo 平台级 OAuth 接入信息。
// 用于控制台「提供商 → 开发信息」：把这两项原样填到钉钉/飞书/企微/自建 IdP 的后台即可。
// 取值与登录链路**同源**（resolveRedirectURI 的实现），避免"界面上写的和实际用的不是同一个地址"。
func (s *AuthService) OAuthPlatformInfo() (callbackURL, homepageURL string) {
	base := strings.TrimSuffix(s.oauthCfg.BaseURL, "/")
	return s.resolveRedirectURI(nil, ""), base
}

// resolveRedirectURI 解析 IdP 授权回跳地址，优先级：
// 请求覆盖 > 提供商行上的 redirect_uri > 全局 oauth.redirect_uri > {base_url}/sso/callback。
//
// 注意：禁止把 from 等自定义参数拼进 redirect_uri（IdP 会按精确匹配拒绝）。
func (s *AuthService) resolveRedirectURI(item *repository.Provider, override string) string {
	if strings.TrimSpace(override) != "" {
		return s.absoluteRedirectURI(override)
	}
	if item != nil && strings.TrimSpace(item.RedirectURI) != "" {
		return s.absoluteRedirectURI(item.RedirectURI)
	}
	if strings.TrimSpace(s.oauthCfg.RedirectURI) != "" {
		return s.absoluteRedirectURI(s.oauthCfg.RedirectURI)
	}
	base := strings.TrimSuffix(s.oauthCfg.BaseURL, "/")
	return base + "/sso/callback"
}

// absoluteRedirectURI 把控制台里填写的「站内相对路径」补成完整地址；
// 外部 IdP 的 redirect_uri 必须是绝对 URL，不能把 /sso/callback 原样发过去。
func (s *AuthService) absoluteRedirectURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") {
		if base := strings.TrimSuffix(strings.TrimSpace(s.oauthCfg.BaseURL), "/"); base != "" {
			return base + raw
		}
	}
	return raw
}

// sanitizeFrom 仅允许站内相对路径作为回跳目标。
func sanitizeFrom(from string) string {
	if from == "" {
		return ""
	}
	u, err := url.Parse(from)
	if err != nil || u.IsAbs() || strings.HasPrefix(from, "//") {
		return ""
	}
	if !strings.HasPrefix(from, "/") || strings.HasPrefix(from, "/login") {
		return ""
	}
	return from
}

// Signup 自助注册已迁移至 auth_signup.go（按 signup_items 编排校验 + 可选自动登录）。
