package service

import (
	"context"
	"strings"
	"time"

	"polaris/internal/auth"
	"polaris/internal/config"
	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// AuthService 认证服务：密码登录、SSO、登出、会话状态。
type AuthService struct {
	orgs        repository.InstitutionRepository
	users       repository.UserRepository
	apps        repository.ApplicationRepository
	jobTitles   repository.JobTitleRepository
	captcha     *CaptchaService
	mfa         *MFAService
	templates   *TemplateService
	codes       repository.CodeStore
	magicLinks  repository.MagicLinkStore
	userService *UserService
	sessions    *auth.SessionManager
	jwt         *auth.JWTManager
	tokens      repository.TokenRepository
	// providerRuntime / oauthResolver：外部身份提供方的运行时读取与连接器解析（见 internal/service/provider）
	providerRuntime *providerpkg.Runtime
	oauthResolver   *providerpkg.Resolver
	states          repository.StateStore
	rateLimiter     repository.RateLimitRepository
	audits          *AuditRecorder
	oauthCfg        config.OAuthConfig
	loginMax        int64
	loginWindow     time.Duration
	sessionTTL      time.Duration
	// provision 供给事件挂钩（可选；登录成功时触发 JIT 建号）。
	provisioning *ProvisioningService
	// backchannel 后端登出通知（可选；会话结束时通知下游 RP）。
	backchannel *BackchannelLogoutService
	// access 应用访问授权（可选；装配阶段注入）。应用准入的**唯一判定入口**——禁止在别处复制判定。
	access *ApplicationAccessService
}

// AuthServiceConfig 认证服务配置
type AuthServiceConfig struct {
	OAuth       config.OAuthConfig
	LoginMax    int64
	LoginWindow time.Duration
	SessionTTL  time.Duration
}

// NewAuthService 创建认证服务
func NewAuthService(
	orgs repository.InstitutionRepository,
	users repository.UserRepository,
	tokens repository.TokenRepository,
	apps repository.ApplicationRepository,
	jobTitles repository.JobTitleRepository,
	providerRuntime *providerpkg.Runtime,
	oauthResolver *providerpkg.Resolver,
	captcha *CaptchaService,
	mfa *MFAService,
	templates *TemplateService,
	codes repository.CodeStore,
	magicLinks repository.MagicLinkStore,
	userService *UserService,
	sessions *auth.SessionManager,
	jwt *auth.JWTManager,
	states repository.StateStore,
	rateLimiter repository.RateLimitRepository,
	audits *AuditRecorder,
	provisioning *ProvisioningService,
	backchannel *BackchannelLogoutService,
	access *ApplicationAccessService,
	cfg AuthServiceConfig,
) *AuthService {
	return &AuthService{
		orgs: orgs, users: users, tokens: tokens, apps: apps, jobTitles: jobTitles,
		providerRuntime: providerRuntime, oauthResolver: oauthResolver,
		captcha: captcha, mfa: mfa, templates: templates, codes: codes, magicLinks: magicLinks,
		userService: userService, sessions: sessions, jwt: jwt,
		states: states, rateLimiter: rateLimiter, audits: audits,
		provisioning: provisioning, backchannel: backchannel, access: access,
		oauthCfg: cfg.OAuth, loginMax: cfg.LoginMax, loginWindow: cfg.LoginWindow, sessionTTL: cfg.SessionTTL,
	}
}

// addDefaultJobTitle 把新注册/验证码 JIT 建号的用户加入应用配置的默认职务。
//
// 应用向导里的「默认职务」字段（applications.default_group）此前只写进了
// users.signup_application，职务关系从未建立——典型的"配了不生效"。
// 这里在用户落库后调用 AddMembers（幂等，只增不删），失败只记日志，不回滚已创建的用户。
func (s *AuthService) addDefaultJobTitle(ctx context.Context, institutionID, jobTitleID int64, user string) {
	if s.jobTitles == nil || jobTitleID == 0 || user == "" {
		return
	}
	if err := s.jobTitles.AddMembers(ctx, jobTitleID, []string{user}); err != nil {
		logger.Warn("add default job title member failed", "institution_id", institutionID, "job_title_id", jobTitleID, "user", user, "error", err)
		return
	}
	if s.provisioning != nil {
		s.provisioning.TriggerRoleGranted(ctx, institutionID, user)
	}
}

// Login 密码登录。
func (s *AuthService) Login(ctx context.Context, req auth.LoginRequest, host, ip, userAgent string) (*auth.LoginResponse, string, error) {
	// 租户自动解析：未显式指定时按"域名 / 唯一租户 / 默认租户 / 账号唯一命中"推断
	resolvedOrg, err := s.resolveLoginInstitution(ctx, req.InstitutionID, req.Institution, host, req.Username)
	if err != nil {
		return nil, "", err
	}
	req.Institution = resolvedOrg

	org, err := s.orgs.GetByName(ctx, req.Institution)
	if err != nil {
		return nil, "", bizerrors.ErrInvalidCredentials
	}
	if org.DisableSignin {
		return nil, "", bizerrors.New(bizerrors.CodeForbidden, "sign-in is disabled for this institutionID")
	}

	// 应用上下文（准入判定的前提）：请求显式携带 client_id / application 时以它为准。
	appCtx, err := s.resolveAppContext(ctx, org.ID, req.Institution, req.Application, req.ClientID)
	if err != nil {
		return nil, "", err
	}
	// 应用级策略（禁用登录 / IP 限制 / 登录方式 / 会话数）仍按既有语义解析：
	// Only an explicit application context applies application-level policy.
	// Platform login must remain independent from every application.
	app := appCtx
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return nil, "", err
	}
	if app != nil && !app.HasSigninMethod(repository.SigninMethodPassword) {
		return nil, "", bizerrors.New(bizerrors.CodeForbidden, "password sign-in is disabled for this application")
	}
	// 人机校验（组织启用 Captcha 提供商时强制；未启用则直接放行）
	if s.captcha != nil {
		if err := s.captcha.Verify(ctx, req.Institution, req.CaptchaID, req.CaptchaCode, req.CaptchaToken, ip); err != nil {
			return nil, "", err
		}
	}

	// 登录失败限制：应用级配置（0 = 沿用平台默认）→ 生效阈值与冻结时长
	maxFailures, retryWait := signinFailurePolicy(app, s.loginMax, s.loginWindow)
	appCode := ""
	if app != nil {
		appCode = app.Code
	}
	rateKey := loginRateKey(req.Institution, appCode, req.Username, ip)
	if s.rateLimiter != nil {
		if n, _ := s.rateLimiter.Get(ctx, rateKey); maxFailures > 0 && n >= maxFailures {
			return nil, "", bizerrors.ErrTooManyRequests
		}
	}

	passwordMethod, _ := app.FindSigninMethod(repository.SigninMethodPassword)
	user, err := s.findUser(ctx, org, req.Username, passwordMethod.ResolvePasswordIdentifiers(), req.IdentifierType)
	if err != nil {
		s.countFailure(ctx, rateKey, maxFailures, retryWait)
		return nil, "", bizerrors.ErrInvalidCredentials
	}
	if user.IsForbidden || user.IsDeleted {
		return nil, "", bizerrors.ErrUserDisabled
	}
	if user.PasswordHash == "" {
		s.countFailure(ctx, rateKey, maxFailures, retryWait)
		return nil, "", bizerrors.New(bizerrors.CodeInvalidCredentials, "this account only supports sso login")
	}
	if !auth.VerifyPasswordWith(req.Password, user.PasswordHash, user.PasswordType, user.PasswordSalt) {
		s.countFailure(ctx, rateKey, maxFailures, retryWait)
		s.audits.Record(ctx, req.Username, org.ID, "auth.login_failed", req.Username, nil)
		return nil, "", bizerrors.ErrInvalidCredentials
	}

	// 应用准入判定：凭证校验通过后、MFA 之前（避免让不可用的账号白做二次验证）。
	// 仅当请求携带应用上下文时生效；拒绝时不建立任何会话。
	if err := s.enforceAppAccess(ctx, appCtx, user); err != nil {
		return nil, "", err
	}

	// ── MFA 判定 ─────────────────────────────────────────
	// 主认证已经通过；MFA 流程在会话建立后统一处理（pending 会话由中间件门禁）。
	mfaChallenge := (*auth.MfaChallenge)(nil)
	mfaTrustToken, mfaTrustMaxAge := "", 0

	if err := s.checkPasswordExpiry(org, user); err != nil {
		return nil, "", err
	}

	// 应用名可空（组织未配置应用时）：会话与 JIT 仅在确有应用时关联
	appName := ""
	if app != nil {
		appName = app.Code
	}
	session, err := s.sessions.Create(ctx, user, appName, ip, userAgent)
	if err != nil {
		return nil, "", err
	}
	if s.rateLimiter != nil {
		_ = s.rateLimiter.Reset(ctx, rateKey)
	}
	mfaChallenge, mfaTrustToken, mfaTrustMaxAge, err = s.applyMFA(ctx, session, app, repository.SigninMethodPassword, user, req.MfaTrust, req.MfaCode, req.RememberMfa)
	if err != nil {
		_ = s.sessions.Delete(ctx, session)
		return nil, "", err
	}
	// 会话数量策略（独占登录 / 最大并发会话）
	s.enforceSessionPolicy(ctx, app, user, session.ID)
	if !session.MfaPending {
		_ = s.users.UpdateLastSignin(ctx, user.ID, time.Now(), ip)
		_ = s.users.SetOnline(ctx, user.ID, true)
		s.audits.Record(ctx, user.Name, user.InstitutionID, "auth.login", user.Name, nil)
		// 登录成功：触发 JIT 建号（仅针对账号策略 createWhen=jit 的应用）
		if s.provisioning != nil && appName != "" {
			s.provisioning.TriggerJIT(ctx, user.InstitutionID, appName, user.Name)
		}
	}

	resp := s.loginResponse(user, session, req.From)
	resp.MfaTrustToken, resp.MfaTrustMaxAge = mfaTrustToken, mfaTrustMaxAge
	resp.Mfa = mfaChallenge
	return resp, session.ID, nil
}

// LoadSession 读取原始会话（handler 将 MFA 端点交给 MFAService 时使用）。
func (s *AuthService) LoadSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	return s.sessions.Load(ctx, sessionID)
}

// MFA 返回 MFA 运行时（handler 唯一入口）。
func (s *AuthService) MFA() *MFAService { return s.mfa }

// ClearMfaPending 兼容旧绑定入口：清除会话的 MFA pending 标记。
func (s *AuthService) ClearMfaPending(ctx context.Context, sessionID string) error {
	return s.sessions.SetMfaPending(ctx, sessionID, false)
}

// PasswordIdentifiers 返回密码登录允许的标识类型（登录页展示）。
func (s *AuthService) PasswordIdentifiers(app *repository.Application) []string {
	method, ok := app.FindSigninMethod(repository.SigninMethodPassword)
	if !ok {
		return []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail}
	}
	return method.ResolvePasswordIdentifiers()
}

func (s *AuthService) loginResponse(user *repository.User, session *auth.Session, from string) *auth.LoginResponse {
	return &auth.LoginResponse{
		Session: &auth.LoginSession{ExpiresAt: session.ExpiresAt, From: from},
		User:    toUserView(user),
		Roles:   session.Roles,
	}
}

func toUserView(u *repository.User) *auth.UserView {
	return &auth.UserView{
		InstitutionID: u.InstitutionID,
		Name:          u.Name,
		DisplayName:   u.DisplayName,
		Email:         u.Email,
		Phone:         u.Phone,
		Avatar:        u.Avatar,
		IsAdmin:       u.IsAdmin,
		MfaEnabled:    u.MfaEnabled,
	}
}

func (s *AuthService) findUser(ctx context.Context, org *repository.Institution, identifier string, allowed []string, explicit string) (*repository.User, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, bizerrors.ErrInvalidCredentials
	}
	explicit = strings.ToLower(strings.TrimSpace(explicit))
	if explicit != "" {
		if !repository.IsPasswordIdentifier(explicit) || !contains(allowed, explicit) {
			return nil, bizerrors.ErrInvalidCredentials
		}
		return s.findUserByIdentifier(ctx, org.ID, identifier, explicit)
	}
	for _, kind := range []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail, repository.PasswordIdentifierPhone} {
		if !contains(allowed, kind) {
			continue
		}
		if user, err := s.findUserByIdentifier(ctx, org.ID, identifier, kind); err == nil {
			return user, nil
		}
	}
	return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
}

func (s *AuthService) findUserByIdentifier(ctx context.Context, institutionID int64, identifier, kind string) (*repository.User, error) {
	switch kind {
	case repository.PasswordIdentifierUsername:
		return s.users.GetByName(ctx, institutionID, identifier)
	case repository.PasswordIdentifierEmail:
		return s.users.GetByEmail(ctx, institutionID, identifier)
	case repository.PasswordIdentifierPhone:
		return s.findUserByPhone(ctx, institutionID, identifier)
	default:
		return nil, bizerrors.ErrInvalidCredentials
	}
}

// applyMFA 在会话建立后统一应用登录方式的 MFA 配置。
// 返回 challenge 表示会话进入 pending；challenge=nil 表示可直接完成登录。
func (s *AuthService) applyMFA(
	ctx context.Context,
	session *auth.Session,
	app *repository.Application,
	methodName string,
	user *repository.User,
	trustToken, legacyCode string,
	remember bool,
) (*auth.MfaChallenge, string, int, error) {
	enabled, required, providerIDs, err := s.mfaSettings(ctx, user.InstitutionID, app, methodName)
	if err != nil {
		return nil, "", 0, err
	}
	if !enabled || s.mfa == nil {
		return nil, "", 0, nil
	}
	active, err := s.mfa.ActiveCredentials(ctx, user.InstitutionID, user.ID, providerIDs)
	if err != nil {
		return nil, "", 0, err
	}
	// 响应里的 mfa_enabled 反映“当前登录方式允许集合内是否有可用凭证”，不读取 users.mfa_* 旧列。
	user.MfaEnabled = len(active) > 0
	// 信任令牌必须绑定具体 provider，且只能命中本次允许集合。
	for _, cred := range active {
		if trustToken != "" && s.jwt.VerifyMfaTrust(trustToken, user.InstitutionID, user.Name, cred.ProviderID) {
			_ = s.sessions.SetMfaTrustProvider(ctx, session.ID, cred.ProviderID)
			return nil, "", 0, nil
		}
	}
	// 兼容旧客户端：仅单一允许凭证时接受登录请求中的 mfa_code。
	if legacyCode != "" && len(active) == 1 {
		ok, verifyErr := s.mfa.VerifyCredentialDirect(ctx, user.InstitutionID, user.ID, active[0].ProviderID, legacyCode)
		if verifyErr != nil {
			return nil, "", 0, verifyErr
		}
		if !ok {
			return nil, "", 0, bizerrors.ErrMfaInvalid
		}
		token, maxAge := "", 0
		if remember {
			if provider, providerErr := s.mfa.ProviderByIDForUser(ctx, user.InstitutionID, active[0].ProviderID); providerErr == nil && provider.MfaRememberHours > 0 {
				if signed, signErr := s.jwt.GenerateMfaTrust(user.InstitutionID, user.Name, provider.ID,
					time.Duration(provider.MfaRememberHours)*time.Hour); signErr == nil {
					token, maxAge = signed, provider.MfaRememberHours*3600
				}
			}
		}
		return nil, token, maxAge, nil
	}
	providers := make([]*repository.Provider, 0, len(providerIDs))
	rememberHours := 0
	challengeType := "select"
	for _, id := range providerIDs {
		provider, providerErr := s.mfa.ProviderByIDForUser(ctx, user.InstitutionID, id)
		if providerErr != nil {
			continue
		}
		providers = append(providers, provider)
		if provider.MfaRememberHours > rememberHours {
			rememberHours = provider.MfaRememberHours
		}
	}
	if len(providers) == 0 {
		// 配置指向的 MFA 提供商均不可用：按“未启用 MFA”处理，避免把用户锁死。
		return nil, "", 0, nil
	}
	if len(providers) == 1 {
		challengeType = providerpkg.NormalizeType(providerpkg.CategoryMFA, providers[0].Type)
	}
	canSkip := len(active) == 0 && !required
	mustVerify := len(active) > 0 || required
	if err := s.sessions.SetMfaChallenge(ctx, session.ID, providerIDs, mustVerify, canSkip, methodName, remember); err != nil {
		return nil, "", 0, err
	}
	session.MfaPending = true
	session.MfaProviderIDs = append([]int64(nil), providerIDs...)
	session.MfaRequired = mustVerify
	session.MfaCanSkip = canSkip
	session.MfaMethod = methodName
	session.MfaRemember = remember
	return &auth.MfaChallenge{
		Required: mustVerify, Enroll: len(active) == 0, CanSkip: canSkip,
		Type: challengeType, RememberHours: rememberHours,
	}, "", 0, nil
}

// mfaSettings 解析登录方式的 MFA 配置，并在旧数据缺少 provider 列表时回落到默认 TOTP。
func (s *AuthService) mfaSettings(ctx context.Context, institutionID int64, app *repository.Application, methodName string) (bool, bool, []int64, error) {
	if s.mfa == nil {
		return false, false, nil, nil
	}
	enabled, required := false, false
	providerIDs := []int64{}
	if app == nil {
		if methodName != repository.SigninMethodPassword {
			return false, false, nil, nil
		}
		enabled = true
	} else if method, ok := app.FindSigninMethod(methodName); ok {
		enabled = method.MFAEnabled()
		required = method.MFARequired()
		providerIDs = append(providerIDs, method.MfaProviderIDs...)
	} else {
		return false, false, nil, nil
	}
	if !enabled {
		return false, false, nil, nil
	}
	if len(providerIDs) > 0 {
		return true, required, dedupeInt64(providerIDs), nil
	}
	items, err := s.providerRuntime.ListActive(ctx, institutionID, providerpkg.CategoryMFA)
	if err != nil {
		return false, false, nil, err
	}
	var fallback *repository.Provider
	for _, item := range items {
		if item.Name == "default-totp" {
			fallback = item
			break
		}
		if fallback == nil && providerpkg.NormalizeType(providerpkg.CategoryMFA, item.Type) == providerpkg.TypeMFATOTP {
			fallback = item
		}
	}
	if fallback == nil {
		return false, false, nil, nil
	}
	return true, required, []int64{fallback.ID}, nil
}

func dedupeInt64(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s *AuthService) checkPasswordExpiry(org *repository.Institution, user *repository.User) error {
	if org.PasswordExpireDays <= 0 || user.PasswordUpdatedAt == nil {
		return nil
	}
	if time.Since(*user.PasswordUpdatedAt) > time.Duration(org.PasswordExpireDays)*24*time.Hour {
		return bizerrors.New(bizerrors.CodePasswordExpired, "password expired, please change it")
	}
	return nil
}

// signinFailurePolicy 解析登录失败限制：应用级字段优先（0 = 沿用平台默认）。
func signinFailurePolicy(app *repository.Application, defaultMax int64, defaultWait time.Duration) (int64, time.Duration) {
	if app == nil {
		return defaultMax, defaultWait
	}
	if app.SigninErrorLimit > 0 {
		defaultMax = int64(app.SigninErrorLimit)
	}
	if app.SigninRetryWaitMinutes > 0 {
		defaultWait = time.Duration(app.SigninRetryWaitMinutes) * time.Minute
	}
	return defaultMax, defaultWait
}

// countFailure 记录一次登录失败。
// 达到次数上限时把计数窗口重置为等待时间——冻结自"达到上限"的那一刻开始计时
// （而不是首次失败），与「登入重试等待时间」的语义一致。
func (s *AuthService) countFailure(ctx context.Context, key string, max int64, wait time.Duration) {
	if s.rateLimiter == nil || max <= 0 || wait <= 0 {
		return
	}
	n, err := s.rateLimiter.Incr(ctx, key, wait)
	if err != nil {
		return
	}
	if n >= max {
		_ = s.rateLimiter.Expire(ctx, key, wait)
	}
}

// loginRateKey 失败计数键：按「组织 + 应用 + 账号 + IP」隔离，
// 使各应用配置的阈值/等待时间互不影响（应用为空时表示组织未配置应用）。
func loginRateKey(institutionID, application, username, ip string) string {
	return "polaris:login_fail:" + institutionID + ":" + application + ":" + username + ":" + ip
}

// Logout 登出（删除会话）。
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	session, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return nil
	}
	if err := s.sessions.Delete(ctx, session); err != nil {
		return err
	}
	if s.tokens != nil {
		if _, err := s.tokens.RevokeBySession(ctx, session.ID); err != nil {
			logger.Warn("failed to revoke session tokens", "session_id", session.ID, "error", err)
		}
	}
	_ = s.users.SetOnline(ctx, session.UserID, false)
	s.audits.Record(ctx, session.Name, session.InstitutionID, "auth.logout", session.Name, nil)
	// 后端登出：通知配置了 backchannel_logout_uri 的下游应用注销会话（异步、失败不阻断）
	if s.backchannel != nil {
		user, err := s.users.GetByName(ctx, session.InstitutionID, session.Name)
		if err == nil {
			go s.backchannel.NotifySessionEnd(context.WithoutCancel(ctx), session.InstitutionID, session.UserID, session.Name, user.UID, session.ID)
		}
	}
	return nil
}

// LogoutByToken 按会话 JWT 登出（Cookie/Bearer 的值均为会话 JWT）。
func (s *AuthService) LogoutByToken(ctx context.Context, token string) error {
	claims, err := s.jwt.Parse(token)
	if err != nil {
		return nil // 令牌无效视为已登出
	}
	return s.Logout(ctx, claims.SessionID)
}

// Status 返回当前会话状态。
func (s *AuthService) Status(ctx context.Context, sessionID string) *auth.AuthStatus {
	if sessionID == "" {
		return &auth.AuthStatus{Authenticated: false}
	}
	session, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return &auth.AuthStatus{Authenticated: false}
	}
	status := &auth.AuthStatus{
		Authenticated: true,
		MfaPending:    session.MfaPending,
		InstitutionID: session.InstitutionID,
		Name:          session.Name,
		DisplayName:   session.DisplayName,
		Email:         session.Email,
		IsAdmin:       session.IsAdmin,
		Roles:         session.Roles,
		Permissions:   session.Permissions,
		ExpiresAt:     session.ExpiresAt,
	}
	// 顶栏与个人资料需要组织展示名与头像：会话里只存登录快照，头像可能在登录后才上传，
	// 因此按会话里的组织 + 账号实时回填（/auth/status 只在应用启动与显式 refresh 时调用）。
	if s.orgs != nil {
		if org, orgErr := s.orgs.GetByID(ctx, session.InstitutionID); orgErr == nil && org != nil {
			status.InstitutionName = firstNonEmptyStr(org.DisplayName, org.Name)
			status.InstitutionLogo = org.Logo
			status.InstitutionFavicon = org.Favicon
		}
	}
	if s.users != nil {
		if user, userErr := s.users.GetByName(ctx, session.InstitutionID, session.Name); userErr == nil && user != nil {
			status.Avatar = user.Avatar
		}
	}
	return status
}

// StatusByToken 按会话 JWT 返回当前会话状态（令牌无效返回未认证）。
func (s *AuthService) StatusByToken(ctx context.Context, token string) *auth.AuthStatus {
	if token == "" {
		return &auth.AuthStatus{Authenticated: false}
	}
	claims, err := s.jwt.Parse(token)
	if err != nil {
		return &auth.AuthStatus{Authenticated: false}
	}
	return s.Status(ctx, claims.SessionID)
}

// IssueJWT 为会话签发 JWT（仅含 session id）。
func (s *AuthService) IssueJWT(sessionID string) (string, error) {
	return s.jwt.Generate(sessionID)
}

// SessionTTL 返回会话有效期。
func (s *AuthService) SessionTTL() time.Duration { return s.sessionTTL }

// CookieTTL resolves the session cookie lifetime for an application-scoped session.
func (s *AuthService) CookieTTL(ctx context.Context, sessionID string) time.Duration {
	session, err := s.sessions.Load(ctx, sessionID)
	if err != nil || session.Application == "" {
		return s.sessionTTL
	}
	app, err := s.apps.GetByCode(ctx, session.InstitutionID, session.Application)
	if err != nil {
		return s.sessionTTL
	}
	return cookieTTL(app, s.sessionTTL)
}

// CookieSecure 是否要求 HTTPS Cookie
func (s *AuthService) CookieSecure() bool { return s.oauthCfg.CookieSecure }

// ── SSO（外部 IdP）───────────────────────────────────────────

// ssoState SSO state 暂存载荷
