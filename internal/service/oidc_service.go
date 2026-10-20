package service

import (
	"context"
	"crypto/subtle"
	"net/url"
	"strings"
	"time"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	"polaris/internal/auth/tokenhash"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// OIDCService 作为 OIDC Provider 的核心服务。
type OIDCService struct {
	apps   repository.ApplicationRepository
	users  repository.UserRepository
	orgs   repository.InstitutionRepository
	roles  repository.RoleRepository
	groups repository.JobTitleRepository
	// userDepts / depts：access_token 的 department_ids / departments 字段（多值部门归属）
	userDepts repository.UserDepartmentRepository
	depts     repository.DepartmentRepository
	tokens    repository.TokenRepository
	issuer    *oidc.Issuer
	audits    *AuditRecorder
	sessions  *auth.SessionManager
	// access 应用访问授权（可选；装配阶段注入）：作为 IdP 的链路一律带应用上下文，因此恒判定。
	access *ApplicationAccessService
}

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
	groups repository.JobTitleRepository,
	userDepts repository.UserDepartmentRepository,
	depts repository.DepartmentRepository,
	tokens repository.TokenRepository,
	issuer *oidc.Issuer,
	audits *AuditRecorder,
	sessions *auth.SessionManager,
	access *ApplicationAccessService,
) *OIDCService {
	return &OIDCService{
		apps: apps, users: users, orgs: orgs, roles: roles, groups: groups,
		userDepts: userDepts, depts: depts,
		tokens: tokens, issuer: issuer, audits: audits, sessions: sessions, access: access,
	}
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
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "auth_time", "sid",
			"name", "given_name", "family_name", "preferred_username", "picture", "website", "gender", "birthdate", "locale", "updated_at",
			"email", "email_verified", "phone_number", "phone_number_verified", "address",
			"institution", "groups", "job_titles", "roles",
		},
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
	SessionID           string
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
		ApplicationID:       &app.ID,
		UserID:              &user.ID,
		Name:                "code-" + randomHex(6),
		CodeHash:            tokenhash.Hash(code),
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: p.CodeChallengeMethod,
		CodeExpireIn:        time.Now().Add(s.issuer.CodeExpire()).Unix(),
		CodeScope:           scope,
		CodeRedirectURI:     p.RedirectURI,
		CodeNonce:           p.Nonce,
		SessionID:           p.SessionID,
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
	if app.ClientSecretHash != "" {
		if clientSecret == "" {
			return nil, bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "missing_secret")
		}
		presented := tokenhash.Hash(clientSecret)
		if subtle.ConstantTimeCompare([]byte(presented), []byte(app.ClientSecretHash)) != 1 {
			return nil, bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "bad_secret")
		}
	}
	return app, nil
}

func (s *OIDCService) grantAuthorizationCode(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	rec, err := s.tokens.GetByCode(ctx, app.ID, req.Code)
	if err != nil {
		return nil, bizerrors.ErrOAuthInvalidGrant
	}
	if rec.CodeIsUsed {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "code_used")
	}
	if rec.CodeExpireIn > 0 && rec.CodeExpireIn < time.Now().Unix() {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "code_expired")
	}
	if rec.ApplicationID == nil || *rec.ApplicationID != app.ID {
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
	consumed, err := s.tokens.ConsumeCode(ctx, app.ID, req.Code)
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
	return s.issueTokens(ctx, app, user, rec.CodeScope, rec.CodeNonce, rec.SessionID)
}

func (s *OIDCService) grantRefreshToken(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	rec, err := s.tokens.GetByRefreshToken(ctx, app.ID, req.RefreshToken)
	if err != nil || rec.Revoked {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "refresh_invalid")
	}
	if rec.ApplicationID == nil || *rec.ApplicationID != app.ID {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "client_mismatch")
	}
	if rec.ExpireTime != nil && rec.ExpireTime.Before(time.Now()) {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "refresh_expired")
	}
	if rec.SessionID != "" && s.sessions != nil {
		if _, err := s.sessions.Load(ctx, rec.SessionID); err != nil {
			return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "session_invalid")
		}
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
		// Rotation disabled: update the existing record in place and omit a new
		// refresh_token. The client keeps using the refresh token it already has.
		return s.issueTokensWithRefresh(ctx, app, user, rec.Scope, "", rec.SessionID, rec, false)
	}
	return s.issueTokensWithRefresh(ctx, app, user, rec.Scope, "", rec.SessionID, rec, true)
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
	return s.issueTokens(ctx, app, user, normalizeScope(req.Scope), "", "")
}

func (s *OIDCService) grantClientCredentials(ctx context.Context, app *repository.Application, req TokenRequest) (*TokenResponse, error) {
	if !contains(app.GrantTypes, "client_credentials") {
		return nil, bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "grant_not_allowed")
	}
	if app.ClientSecretHash == "" {
		return nil, bizerrors.ErrOAuthInvalidClient.WithDetail("reason", "client_credentials_requires_secret")
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
	if err := s.storeToken(ctx, app, nil, "", scope, accessToken, "", "", exp, "client_credentials"); err != nil {
		return nil, err
	}
	return &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(time.Until(exp).Seconds()),
		Scope:       scope,
	}, nil
}

func (s *OIDCService) issueTokens(ctx context.Context, app *repository.Application, user *repository.User, scope, nonce, sessionID string) (*TokenResponse, error) {
	return s.issueTokensWithRefresh(ctx, app, user, scope, nonce, sessionID, nil, false)
}

// issueTokensWithRefresh issues a token pair. A nil existing record creates a
// new record. When rotate is true, the existing record is revoked and replaced
// with a new one carrying a fresh refresh token. When rotate is false, the
// existing record is updated in place and the current refresh token is kept.
func (s *OIDCService) issueTokensWithRefresh(
	ctx context.Context,
	app *repository.Application,
	user *repository.User,
	scope, nonce, sessionID string,
	existing *repository.Token,
	rotate bool,
) (*TokenResponse, error) {
	scope = normalizeScope(scope)
	authTime := time.Now()
	extra, err := s.tokenExtra(ctx, app, user)
	if err != nil {
		return nil, err
	}
	accessToken, exp, err := s.issuer.IssueAccessTokenTTL(oidc.TokenClaims{
		Subject:   user.UID,
		Audience:  app.ClientID,
		Scope:     scope,
		SessionID: sessionID,
		Extra:     extra,
	}, accessTokenTTL(app, s.issuer.AccessTokenExpire()))
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "签发 access_token 失败", err)
	}
	refreshToken := ""
	var refreshExp *time.Time
	if existing == nil || rotate {
		if shouldIssueRefresh(app, scope) {
			// Offline access：仅在请求 scope 含 offline_access 且应用允许刷新时下发 refresh_token
			// （OIDC 语义：offline_access 用于申请长期离线访问的 refresh_token）。
			refreshToken = s.issuer.NewRefreshToken()
			t := time.Now().Add(refreshTokenTTL(app, s.issuer.RefreshTokenExpire()))
			refreshExp = &t
		}
	}
	idToken := ""
	if strings.Contains(scope, "openid") {
		claims, err := s.idTokenClaims(ctx, app, user, scope)
		if err != nil {
			return nil, err
		}
		token, _, err := s.issuer.IssueIDToken(oidc.IDTokenParams{
			Subject: user.UID, Audience: app.ClientID, Nonce: nonce, AuthTime: authTime,
			SessionID: sessionID, Claims: claims,
		})
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "签发 id_token 失败", err)
		}
		idToken = token
	}
	switch {
	case existing == nil:
		record := tokenRecord(app, user, "token-"+randomHex(6), scope, accessToken, refreshToken, idToken, sessionID, exp, refreshExp, "authorization_code")
		if _, err := s.tokens.Create(ctx, record); err != nil {
			return nil, err
		}
	case rotate:
		record := tokenRecord(app, user, "token-rotated-"+randomHex(6), scope, accessToken, refreshToken, idToken, sessionID, exp, refreshExp, "authorization_code")
		if err := s.tokens.RotateRefreshToken(ctx, existing.ID, record); err != nil {
			return nil, err
		}
	default:
		existing.SessionID = sessionID
		existing.AccessTokenHash = tokenhash.Hash(accessToken)
		existing.IDTokenHash = ""
		if idToken != "" {
			existing.IDTokenHash = tokenhash.Hash(idToken)
		}
		existing.ExpiresIn = int(time.Until(exp).Seconds())
		existing.Scope = scope
		if err := s.tokens.Update(ctx, existing); err != nil {
			return nil, err
		}
	}
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

// tokenExtra 生成 access_token 的自定义 claim。
// 部门字段按需查询 user_departments（唯一真相），部门展示名用完整路径；两者都是数组。
