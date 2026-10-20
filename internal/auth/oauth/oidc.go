package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"polaris/internal/config"
)

// defaultDiscoveryTimeout OIDC discovery 的超时（构造连接器时执行一次）
const defaultDiscoveryTimeout = 10 * time.Second

// OIDCProvider 通用 OIDC 连接器（标准 discovery + 授权码流程）。
//
// 两种模式：
//   - discovery 模式（默认）：由 issuer 发现全部端点与 JWKS，并校验 id_token；
//   - 静态端点模式：当配置同时给出授权端点与令牌端点时，不再访问 discovery
//     （用于"不暴露 /.well-known"的私有 IdP），用户信息以 userinfo 端点为准。
//
// PKCE：启用后 Authorize/Exchange 走 AuthCodeURLWithPKCE / ExchangeWithPKCE（S256），
// code_verifier 由调用方随 state 暂存（见 internal/service/auth_service.go 的 ssoState）。
type OIDCProvider struct {
	name         string
	displayName  string
	provider     *oidc.Provider // 静态端点模式下为 nil
	verifier     *oidc.IDTokenVerifier
	oauth2Config oauth2.Config
	userInfoURL  string
	httpClient   *http.Client
}

// NewOIDCProvider 创建 OIDC 连接器。
func NewOIDCProvider(name string, cfg config.OAuthProviderConfig) (*OIDCProvider, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("OIDC provider %s 缺少 client_id", name)
	}
	display := cfg.DisplayName
	if display == "" {
		display = name
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	base := &OIDCProvider{name: name, displayName: display, httpClient: http.DefaultClient}

	// 静态端点模式：授权 + 令牌端点齐全即可（issuer 可为空）
	if cfg.AuthURL != "" && cfg.TokenURL != "" {
		base.oauth2Config = oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL},
			Scopes:       scopes,
		}
		base.userInfoURL = cfg.UserInfoURL
		return base, nil
	}

	if cfg.Issuer == "" {
		return nil, fmt.Errorf("OIDC provider %s 缺少 issuer", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultDiscoveryTimeout)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery 失败: %w", err)
	}
	base.provider = provider
	base.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	base.oauth2Config = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}
	base.userInfoURL = cfg.UserInfoURL
	if base.userInfoURL == "" {
		if claims := struct {
			UserInfoURL string `json:"userinfo_endpoint"`
		}{}; provider.Claims(&claims) == nil {
			base.userInfoURL = claims.UserInfoURL
		}
	}
	return base, nil
}

// Name 连接器名
func (p *OIDCProvider) Name() string { return p.name }

// DisplayName 展示名
func (p *OIDCProvider) DisplayName() string { return p.displayName }

// AuthCodeURL 生成授权地址
func (p *OIDCProvider) AuthCodeURL(state, redirectURI string) string {
	cfg := p.oauth2Config
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// AuthCodeURLWithPKCE 生成带 PKCE（S256）挑战的授权地址
func (p *OIDCProvider) AuthCodeURLWithPKCE(state, redirectURI, challenge string) string {
	cfg := p.oauth2Config
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"))
}

// Exchange 用授权码换取用户信息
func (p *OIDCProvider) Exchange(ctx context.Context, code, redirectURI string) (*UserInfo, error) {
	return p.exchange(ctx, code, redirectURI)
}

// ExchangeWithPKCE 用授权码 + code_verifier 换取用户信息
func (p *OIDCProvider) ExchangeWithPKCE(ctx context.Context, code, redirectURI, verifier string) (*UserInfo, error) {
	return p.exchange(ctx, code, redirectURI, oauth2.SetAuthURLParam("code_verifier", verifier))
}

func (p *OIDCProvider) exchange(ctx context.Context, code, redirectURI string, opts ...oauth2.AuthCodeOption) (*UserInfo, error) {
	cfg := p.oauth2Config
	cfg.RedirectURL = redirectURI
	token, err := cfg.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, fmt.Errorf("OIDC 换取 token 失败: %w", err)
	}
	// discovery 模式下优先校验 id_token（能以签名背书身份来源）
	if p.verifier != nil {
		if raw, ok := token.Extra("id_token").(string); ok && raw != "" {
			return p.fromIDToken(ctx, raw)
		}
	}
	return p.fromUserInfo(ctx, token)
}

func (p *OIDCProvider) fromIDToken(ctx context.Context, rawIDToken string) (*UserInfo, error) {
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("OIDC id_token 校验失败: %w", err)
	}
	var claims struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		PreferredName string `json:"preferred_username"`
		Email         string `json:"email"`
		Picture       string `json:"picture"`
		Phone         string `json:"phone_number"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("解析 OIDC claims 失败: %w", err)
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredName
	}
	return &UserInfo{
		ProviderUserID: claims.Sub,
		Username:       claims.PreferredName,
		DisplayName:    name,
		Email:          claims.Email,
		Phone:          claims.Phone,
		Avatar:         claims.Picture,
	}, nil
}

// fromUserInfo 以 userinfo 端点作为主体来源（静态端点模式，或无 id_token 的 IdP）。
func (p *OIDCProvider) fromUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	if p.userInfoURL == "" {
		return nil, fmt.Errorf("OIDC 响应缺少 id_token，且未配置用户信息端点")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	token.SetAuthHeader(req)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC 用户信息请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OIDC 用户信息返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var claims struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		PreferredName string `json:"preferred_username"`
		Email         string `json:"email"`
		Picture       string `json:"picture"`
		Phone         string `json:"phone_number"`
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, fmt.Errorf("解析 OIDC 用户信息失败: %w", err)
	}
	if claims.Sub == "" {
		return nil, fmt.Errorf("OIDC 用户信息缺少 sub")
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredName
	}
	return &UserInfo{
		ProviderUserID: claims.Sub,
		Username:       claims.PreferredName,
		DisplayName:    name,
		Email:          claims.Email,
		Phone:          claims.Phone,
		Avatar:         claims.Picture,
	}, nil
}
