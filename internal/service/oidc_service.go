package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// OIDCService 作为 OIDC Provider 的核心服务。
type OIDCService struct {
	apps   repository.ApplicationRepository
	users  repository.UserRepository
	orgs   repository.InstitutionRepository
	roles  repository.RoleRepository
	groups repository.GroupRepository
	tokens repository.TokenRepository
	issuer *oidc.Issuer
	audits *AuditRecorder
	// access 应用访问授权（可选；装配阶段注入）：作为 IdP 的链路一律带应用上下文，因此恒判定。
	access *ApplicationAccessService
}

// SetApplicationAccess 注入应用访问授权服务（装配阶段调用）。
func (s *OIDCService) SetApplicationAccess(a *ApplicationAccessService) { s.access = a }

// enforceAccess 应用准入判定（令牌链路：应用上下文恒存在，故无"是否判定"的分支）。
func (s *OIDCService) enforceAccess(ctx context.Context, app *repository.Application, user *repository.User) error {
	if s.access == nil {
		return nil
	}
	return s.access.Enforce(ctx, app, user)
}

// NewOIDCService 创建 OIDC 服务
func NewOIDCService(
	apps repository.ApplicationRepository,
	users repository.UserRepository,
	orgs repository.InstitutionRepository,
	roles repository.RoleRepository,
	groups repository.GroupRepository,
	tokens repository.TokenRepository,
	issuer *oidc.Issuer,
	audits *AuditRecorder,
) *OIDCService {
	return &OIDCService{apps: apps, users: users, orgs: orgs, roles: roles, groups: groups, tokens: tokens, issuer: issuer, audits: audits}
}

// Issuer 返回签发器
func (s *OIDCService) Issuer() *oidc.Issuer { return s.issuer }

// Discovery 返回 OIDC discovery 文档。
func (s *OIDCService) Discovery() map[string]interface{} {
	issuer := strings.TrimSuffix(s.issuer.IssuerURL(), "/")
	return map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"userinfo_endpoint":                     issuer + "/oauth/userinfo",
		"jwks_uri":                              issuer + "/.well-known/jwks",
		"end_session_endpoint":                  issuer + "/oauth/logout",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "password", "client_credentials"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "phone", "address", "offline_access"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "name", "preferred_username", "email", "email_verified", "picture", "phone_number"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"backchannel_logout_supported":          true,
		"backchannel_logout_session_supported":  true,
	}
}

// JWKS 返回 JSON Web Key Set。
func (s *OIDCService) JWKS() map[string]interface{} { return s.issuer.Certs().JWKS() }

// AuthorizeParams 授权请求参数
type AuthorizeParams struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	Institution         string
	UserName            string
}

// Authorize 处理授权请求，返回需要重定向的地址（携带授权码）。
func (s *OIDCService) Authorize(ctx context.Context, p AuthorizeParams) (string, error) {
	app, err := s.apps.GetByClientID(ctx, p.ClientID)
	if err != nil {
		return "", bizerrors.ErrOAuthInvalidClient.WithDetail("client_id", p.ClientID)
	}
	if app.State != "active" {
		return "", bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "application_disabled")
	}
	if !appAllowsRedirect(app, p.RedirectURI) {
		return "", bizerrors.ErrOAuthInvalidRedirect.WithDetail("redirect_uri", p.RedirectURI)
	}
	if p.ResponseType != "" && p.ResponseType != "code" {
		return "", bizerrors.New(bizerrors.CodeOAuthUnsupportedGrant, "only response_type=code is supported")
	}
	// scope 必须落在应用允许列表内（未配置 scope 的应用不做限制）
	if err := validateAppScopes(app, p.Scope); err != nil {
		return "", err
	}
	user, err := s.users.GetByName(ctx, app.InstitutionID, p.UserName)
	if err != nil {
		return "", bizerrors.ErrUnauthorized
	}
	// 应用准入判定（Spec FR-3）：authorize 天然携带应用上下文（client_id），因此一律判定。
	// 拒绝时不返回错误信封，而是按 OAuth2 规范回跳 redirect_uri?error=access_denied&state=...，
	// 否则调用方（RP）看到的是 Polaris 的 JSON 错误页，而不是标准的"授权被拒"。
	if s.access != nil {
		allowed, aerr := s.access.Allowed(ctx, app, user)
		if aerr != nil {
			return "", aerr
		}
		if !allowed {
			redirect, perr := url.Parse(p.RedirectURI)
			if perr != nil {
				return "", bizerrors.ErrOAuthInvalidRedirect
			}
			q := redirect.Query()
			q.Set("error", "access_denied")
			q.Set("error_description", "user is not authorized for this application")
			if p.State != "" {
				q.Set("state", p.State)
			}
			redirect.RawQuery = q.Encode()
			s.audits.Record(ctx, user.Name, app.InstitutionID, "oauth.authorize_denied", app.Code, nil)
			return redirect.String(), nil
		}
	}
	scope := normalizeScope(p.Scope)
	code := s.issuer.NewCode()
	rec := &repository.Token{
		InstitutionID:       app.InstitutionID,
		Application:         app.Code,
		User:                user.Name,
		Name:                "code-" + randomHex(6),
		Code:                code,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		CodeExpireIn:        time.Now().Add(s.issuer.CodeExpire()).Unix(),
		CodeScope:           scope,
		CodeRedirectURI:     p.RedirectURI,
		Scope:               scope,
		GrantType:           "authorization_code",
	}
	if _, err := s.tokens.Create(ctx, rec); err != nil {
		return "", err
	}
	redirect, err := url.Parse(p.RedirectURI)
	if err != nil {
		return "", bizerrors.ErrOAuthInvalidRedirect
	}
	q := redirect.Query()
	q.Set("code", code)
	if p.State != "" {
		q.Set("state", p.State)
	}
	redirect.RawQuery = q.Encode()
	s.audits.Record(ctx, user.Name, app.InstitutionID, "oauth.authorize", app.Code, map[string]interface{}{"scope": scope})
	return redirect.String(), nil
}

// TokenRequest 令牌请求
type TokenRequest struct {
	GrantType    string
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
	CodeVerifier string
	RefreshToken string
	Username     string
	Password     string
	Scope        string
}

// TokenResponse 令牌响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// Token 处理令牌请求。
func (s *OIDCService) Token(ctx context.Context, req TokenRequest) (*TokenResponse, error) {
	app, err := s.authenticateClient(ctx, req.ClientID, req.ClientSecret)
	if err != nil {
		return nil, err
	}
	switch req.GrantType {
	case "authorization_code":
		return s.grantAuthorizationCode(ctx, app, req)
	case "refresh_token":
		return s.grantRefreshToken(ctx, app, req)
	case "password":
		return s.grantPassword(ctx, app, req)
	case "client_credentials":
		return s.grantClientCredentials(ctx, app, req)
	default:
		return nil, bizerrors.New(bizerrors.CodeOAuthUnsupportedGrant, "unsupported grant_type").WithDetail("grant_type", req.GrantType)
	}
}

func (s *OIDCService) authenticateClient(ctx context.Context, clientID, clientSecret string) (*repository.Application, error) {
	if clientID == "" {
		return nil, bizerrors.ErrOAuthInvalidClient
	}
	app, err := s.apps.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, bizerrors.ErrOAuthInvalidClient
	}
	if app.State != "active" {
		return nil, bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "application_disabled")
	}
	if clientSecret != "" && app.ClientSecret != "" && clientSecret != app.ClientSecret {
		return nil, bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "bad_secret")
	}
	return app, nil
}

func (s *OIDCService) grantAuthorizationCode(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	rec, err := s.tokens.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, bizerrors.ErrOAuthInvalidGrant
	}
	if rec.CodeIsUsed {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "code_used")
	}
	if rec.CodeExpireIn > 0 && rec.CodeExpireIn < time.Now().Unix() {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "code_expired")
	}
	if rec.Application != app.Code {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "client_mismatch")
	}
	if rec.CodeRedirectURI != "" && req.RedirectURI != "" && rec.CodeRedirectURI != req.RedirectURI {
		return nil, bizerrors.ErrOAuthInvalidRedirect
	}
	if rec.CodeChallenge != "" {
		if !verifyPKCE(rec.CodeChallenge, rec.CodeChallengeMethod, req.CodeVerifier) {
			return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "pkce_failed")
		}
	}
	consumed, err := s.tokens.ConsumeCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "code_replayed")
	}
	user, err := s.users.GetByName(ctx, rec.InstitutionID, rec.User)
	if err != nil {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "user_missing")
	}
	// 应用准入：authorize 与换码之间被移出名单时，授权码立即失效（收口窗口期）
	if err := s.enforceAccess(ctx, app, user); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, app, user, rec.CodeScope, rec.Name, false, 0)
}

func (s *OIDCService) grantRefreshToken(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	rec, err := s.tokens.GetByRefreshToken(ctx, req.RefreshToken)
	if err != nil || rec.Revoked {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "refresh_invalid")
	}
	if rec.Application != app.Code {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "client_mismatch")
	}
	if rec.ExpireTime != nil && rec.ExpireTime.Before(time.Now()) {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "refresh_expired")
	}
	user, err := s.users.GetByName(ctx, rec.InstitutionID, rec.User)
	if err != nil {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "user_missing")
	}
	// 「撤销授权立即生效」的关键点：换新令牌前**实时**判定，不依赖会话是否已被失效。
	if err := s.enforceAccess(ctx, app, user); err != nil {
		return nil, err
	}
	if app.DisableRefreshRotation {
		// 关闭轮换：保留原 refresh_token（记录不吊销），仅重新签发 access_token
		return s.issueTokensWithRefresh(ctx, app, user, rec.Scope, rec.RefreshToken, true)
	}
	// 默认轮换：旧令牌立即失效
	rec.RefreshToken = ""
	rec.RefreshTokenHash = ""
	rec.Revoked = true
	_ = s.tokens.Update(ctx, rec)
	return s.issueTokens(ctx, app, user, rec.Scope, rec.Name, false, 0)
}

func (s *OIDCService) grantPassword(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	if !contains(app.GrantTypes, "password") {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "grant_not_allowed")
	}
	if !app.HasSigninMethod(repository.SigninMethodPassword) {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "password_login_disabled")
	}
	if err := validateAppScopes(app, req.Scope); err != nil {
		return nil, err
	}
	user, err := s.users.GetByName(ctx, app.InstitutionID, req.Username)
	if err != nil {
		user, err = s.users.GetByEmail(ctx, app.InstitutionID, req.Username)
		if err != nil {
			return nil, bizerrors.ErrInvalidCredentials
		}
	}
	if user.IsForbidden || user.IsDeleted || !auth.VerifyPasswordWith(req.Password, user.PasswordHash, user.PasswordType, user.PasswordSalt) {
		return nil, bizerrors.ErrInvalidCredentials
	}
	if err := s.enforceAccess(ctx, app, user); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, app, user, normalizeScope(req.Scope), "", false, 0)
}

func (s *OIDCService) grantClientCredentials(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	if !contains(app.GrantTypes, "client_credentials") {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "grant_not_allowed")
	}
	scope := normalizeScope(req.Scope)
	accessToken, exp, err := s.issuer.IssueAccessToken(oidc.TokenClaims{
		Subject:  app.ClientID,
		Audience: app.ClientID,
		Scope:    scope,
	})
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "签发令牌失败", err)
	}
	_ = s.storeToken(ctx, app, nil, "", scope, accessToken, "", "", exp, "client_credentials")
	return &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(exp).Seconds()),
		Scope:       scope,
	}, nil
}

func (s *OIDCService) issueTokens(ctx context.Context, app *repository.Application, user *repository.User, scope, parentName string, _ bool, _ int) (*TokenResponse, error) {
	return s.issueTokensWithRefresh(ctx, app, user, scope, "", false)
}

// issueTokensWithRefresh 签发令牌；reuseRefresh 为 true 时复用既有 refresh_token（应用关闭轮换）。
func (s *OIDCService) issueTokensWithRefresh(ctx context.Context, app *repository.Application, user *repository.User, scope, reuseRefresh string, reuse bool) (*TokenResponse, error) {
	scope = normalizeScope(scope)
	authTime := time.Now()
	accessToken, exp, err := s.issuer.IssueAccessTokenTTL(oidc.TokenClaims{
		Subject:  user.UID,
		Audience: app.ClientID,
		Scope:    scope,
		Extra:    sanitizeTokenExtra(app, user),
	}, accessTokenTTL(app, s.issuer.AccessTokenExpire()))
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "签发 access_token 失败", err)
	}
	refreshToken := ""
	var refreshExp *time.Time
	if reuse && reuseRefresh != "" {
		refreshToken = reuseRefresh
		t := time.Now().Add(refreshTokenTTL(app, s.issuer.RefreshTokenExpire()))
		refreshExp = &t
	} else if shouldIssueRefresh(app, scope) {
		// Offline access：仅在请求 scope 含 offline_access 且应用允许刷新时下发 refresh_token
		// （OIDC 语义：offline_access 用于申请长期离线访问的 refresh_token）。
		refreshToken = s.issuer.NewRefreshToken()
		t := time.Now().Add(refreshTokenTTL(app, s.issuer.RefreshTokenExpire()))
		refreshExp = &t
	}
	idToken := ""
	if strings.Contains(scope, "openid") {
		claims, err := s.idTokenClaims(ctx, app, user, scope)
		if err != nil {
			return nil, err
		}
		token, _, err := s.issuer.IssueIDToken(oidc.IDTokenParams{
			Subject: user.UID, Audience: app.ClientID, AuthTime: authTime, Claims: claims,
		})
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "签发 id_token 失败", err)
		}
		idToken = token
	}
	name := "token-" + randomHex(6)
	if reuse && reuseRefresh != "" {
		name = "token-rotated-" + randomHex(6)
	}
	_ = s.storeTokenWithRefresh(ctx, app, user, name, scope, accessToken, refreshToken, idToken, exp, refreshExp, "authorization_code")
	s.audits.Record(ctx, user.Name, user.InstitutionID, "oauth.token", app.Code, map[string]interface{}{"scope": scope})
	return &TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(time.Until(exp).Seconds()),
		RefreshToken: refreshToken,
		IDToken:      idToken,
		Scope:        scope,
	}, nil
}

func (s *OIDCService) idTokenClaims(ctx context.Context, app *repository.Application, user *repository.User, scope string) (map[string]interface{}, error) {
	claims := map[string]interface{}{
		"preferred_username": user.Name,
	}
	if strings.Contains(scope, "profile") {
		claims["name"] = firstNonEmptyStr(user.DisplayName, user.Name)
		claims["picture"] = user.Avatar
		claims["locale"] = user.Language
		roles, _ := s.roles.RolesForUser(ctx, user.InstitutionID, user.Name)
		if len(roles) > 0 {
			claims["roles"] = roles
		}
		groups, _ := s.groups.GroupsForUser(ctx, user.InstitutionID, user.Name)
		if len(groups) > 0 {
			claims["groups"] = groups
		}
	}
	if strings.Contains(scope, "email") {
		claims["email"] = user.Email
		claims["email_verified"] = user.EmailVerified
	}
	if strings.Contains(scope, "phone") {
		claims["phone_number"] = user.Phone
	}
	claims["institutionID"] = user.InstitutionID
	claims["is_admin"] = user.IsAdmin
	// 应用适配器：custom 时按 claimMap 覆盖/新增 claim（default 忽略）
	applyAdapter(app, user, claims)
	return claims, nil
}

func (s *OIDCService) storeToken(ctx context.Context, app *repository.Application, user *repository.User, name, scope, accessToken, refreshToken, idToken string, exp time.Time, grantType string) error {
	return s.storeTokenWithRefresh(ctx, app, user, name, scope, accessToken, refreshToken, idToken, exp, nil, grantType)
}

func (s *OIDCService) storeTokenWithRefresh(ctx context.Context, app *repository.Application, user *repository.User, name, scope, accessToken, refreshToken, idToken string, exp time.Time, refreshExp *time.Time, grantType string) error {
	userName := ""
	org := app.InstitutionID
	if user != nil {
		userName = user.Name
		org = user.InstitutionID
	}
	rec := &repository.Token{
		InstitutionID:   org,
		Application:     app.Code,
		User:            userName,
		Name:            name,
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		IDToken:         idToken,
		AccessTokenHash: hashToken(accessToken),
		ExpiresIn:       int(time.Until(exp).Seconds()),
		Scope:           scope,
		TokenType:       "Bearer",
		GrantType:       grantType,
		ExpireTime:      refreshExp,
	}
	if refreshToken != "" {
		rec.RefreshTokenHash = hashToken(refreshToken)
	}
	_, err := s.tokens.Create(ctx, rec)
	return err
}

// UserInfo 返回 access_token 对应的用户信息。
func (s *OIDCService) UserInfo(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	claims, err := s.issuer.VerifyToken(accessToken)
	if err != nil {
		return nil, bizerrors.ErrInvalidToken
	}
	sub, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	// org claim 现在是组织主键 id（数字）；兼容旧令牌里可能是组织名
	orgID := int64(0)
	switch v := claims["org"].(type) {
	case float64:
		orgID = int64(v)
	case string:
		if v != "" {
			parsed, parseErr := strconv.ParseInt(v, 10, 64)
			if parseErr == nil {
				orgID = parsed
			} else if o, orgErr := s.orgs.GetByName(ctx, v); orgErr == nil {
				orgID = o.ID
			}
		}
	}
	if username == "" || orgID == 0 {
		return map[string]interface{}{"sub": sub}, nil
	}
	user, err := s.users.GetByName(ctx, orgID, username)
	if err != nil {
		return nil, bizerrors.ErrInvalidToken
	}
	scope, _ := claims["scope"].(string)
	result := map[string]interface{}{"sub": user.UID}
	if strings.Contains(scope, "profile") {
		result["name"] = firstNonEmptyStr(user.DisplayName, user.Name)
		result["preferred_username"] = user.Name
		result["picture"] = user.Avatar
		roles, _ := s.roles.RolesForUser(ctx, user.InstitutionID, user.Name)
		result["roles"] = roles
	}
	if strings.Contains(scope, "email") {
		result["email"] = user.Email
		result["email_verified"] = user.EmailVerified
	}
	if strings.Contains(scope, "phone") {
		result["phone_number"] = user.Phone
	}
	result["institutionID"] = user.InstitutionID
	// 应用适配器：按 access_token 的 aud（client_id）定位应用，custom 时覆盖 claim
	if aud, _ := claims["aud"].(string); aud != "" {
		if app, err := s.apps.GetByClientID(ctx, aud); err == nil {
			applyAdapter(app, user, result)
		}
	}
	return result, nil
}

// Introspect 令牌内省。
func (s *OIDCService) Introspect(ctx context.Context, token string) map[string]interface{} {
	claims, err := s.issuer.VerifyToken(token)
	if err != nil {
		return map[string]interface{}{"active": false}
	}
	claims["active"] = true
	return claims
}

// ── 辅助 ──────────────────────────────────────────────

func appAllowsRedirect(app *repository.Application, redirectURI string) bool {
	if redirectURI == "" {
		return false
	}
	for _, r := range app.RedirectURIs {
		if r == redirectURI {
			return true
		}
		// 支持通配子路径（如 https://app.example.com/*）
		if strings.HasSuffix(r, "/*") && strings.HasPrefix(redirectURI, strings.TrimSuffix(r, "*")) {
			return true
		}
	}
	return false
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "openid profile email"
	}
	if !contains(strings.Fields(scope), "openid") {
		scope = "openid " + scope
	}
	return scope
}

// hasScope 判断以空格分隔的 scope 串中是否包含指定 scope。
func hasScope(scope, target string) bool {
	return contains(strings.Fields(scope), target)
}

// shouldIssueRefresh 判定是否签发 refresh_token：
// 应用须允许刷新（RefreshExpireInHours > 0），且请求 scope 含 offline_access。
func shouldIssueRefresh(app *repository.Application, scope string) bool {
	return app != nil && app.RefreshExpireInHours > 0 && hasScope(scope, "offline_access")
}

func contains(items []string, target string) bool {
	for _, it := range items {
		if it == target {
			return true
		}
	}
	return false
}

func verifyPKCE(challenge, method, verifier string) bool {
	if verifier == "" {
		return false
	}
	switch method {
	case "", "plain":
		return challenge == verifier
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		return challenge == base64.RawURLEncoding.EncodeToString(sum[:])
	default:
		return false
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

var _ = jwt.MapClaims{}
