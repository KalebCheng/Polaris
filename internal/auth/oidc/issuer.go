package oidc

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer 令牌签发器（access_token / id_token / refresh_token / authorization code）。
type Issuer struct {
	certs              *CertManager
	issuerURL          string
	accessTokenExpire  time.Duration
	refreshTokenExpire time.Duration
	codeExpire         time.Duration
}

// Config 签发器配置
type Config struct {
	CertManager          *CertManager
	IssuerURL            string
	AccessTokenExpire    time.Duration
	RefreshTokenExpire   time.Duration
	AuthorizationCodeTTL time.Duration
}

// NewIssuer 创建签发器
func NewIssuer(cfg Config) *Issuer {
	if cfg.AccessTokenExpire <= 0 {
		cfg.AccessTokenExpire = time.Hour
	}
	if cfg.RefreshTokenExpire <= 0 {
		cfg.RefreshTokenExpire = 24 * time.Hour
	}
	if cfg.AuthorizationCodeTTL <= 0 {
		cfg.AuthorizationCodeTTL = 5 * time.Minute
	}
	return &Issuer{
		certs:              cfg.CertManager,
		issuerURL:          cfg.IssuerURL,
		accessTokenExpire:  cfg.AccessTokenExpire,
		refreshTokenExpire: cfg.RefreshTokenExpire,
		codeExpire:         cfg.AuthorizationCodeTTL,
	}
}

// IssuerURL 返回 issuer 地址
func (i *Issuer) IssuerURL() string { return i.issuerURL }

// Certs 返回证书管理器
func (i *Issuer) Certs() *CertManager { return i.certs }

// AccessTokenExpire 返回 access_token 默认有效期
func (i *Issuer) AccessTokenExpire() time.Duration { return i.accessTokenExpire }

// RefreshTokenExpire 返回 refresh_token 默认有效期
func (i *Issuer) RefreshTokenExpire() time.Duration { return i.refreshTokenExpire }

// CodeExpire 返回授权码有效期
func (i *Issuer) CodeExpire() time.Duration { return i.codeExpire }

// TokenClaims access_token / id_token 的声明参数
type TokenClaims struct {
	Subject   string
	Audience  string
	Scope     string
	SessionID string
	Extra     map[string]interface{}
}

// IssueAccessToken 签发 access_token（使用签发器默认有效期）
func (i *Issuer) IssueAccessToken(c TokenClaims) (string, time.Time, error) {
	return i.IssueAccessTokenTTL(c, 0)
}

// IssueAccessTokenTTL 签发 access_token，ttl <= 0 时使用默认有效期。
// 应用可配置自身的令牌有效期（application.expire_in_hours），因此此处允许覆盖。
func (i *Issuer) IssueAccessTokenTTL(c TokenClaims, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = i.accessTokenExpire
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := jwt.MapClaims{
		"iss": i.issuerURL,
		"sub": c.Subject,
		"aud": c.Audience,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": exp.Unix(),
		"jti": randomID(),
		"typ": "access",
	}
	if c.Scope != "" {
		claims["scope"] = c.Scope
	}
	if c.SessionID != "" {
		claims["sid"] = c.SessionID
	}
	for k, v := range c.Extra {
		claims[k] = v
	}
	signed, err := i.certs.Sign(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// IDTokenParams id_token 的声明参数
type IDTokenParams struct {
	Subject  string
	Audience string
	Nonce    string
	AuthTime time.Time
	Claims   map[string]interface{}
}

// IssueIDToken 签发 id_token（JWT）
func (i *Issuer) IssueIDToken(p IDTokenParams) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(i.accessTokenExpire)
	claims := jwt.MapClaims{
		"iss": i.issuerURL,
		"sub": p.Subject,
		"aud": p.Audience,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	if p.Nonce != "" {
		claims["nonce"] = p.Nonce
	}
	if !p.AuthTime.IsZero() {
		claims["auth_time"] = p.AuthTime.Unix()
	}
	for k, v := range p.Claims {
		claims[k] = v
	}
	signed, err := i.certs.Sign(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// LogoutTokenParams 登出令牌（logout_token）的声明参数。
type LogoutTokenParams struct {
	Subject   string
	Audience  string // 目标 client_id
	SessionID string // 可选：本地会话 id
}

// IssueLogoutToken 签发 OpenID Connect Back-Channel Logout 的 logout_token。
// 依据 OIDC Back-Channel Logout 1.0：typ=logout+jwt，携带 events 声明，
// 且必须包含 sub 与 sid 之一（此处两者都带）。
func (i *Issuer) IssueLogoutToken(p LogoutTokenParams) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(2 * time.Minute) // 登出令牌有效期短
	claims := jwt.MapClaims{
		"iss": i.issuerURL,
		"aud": p.Audience,
		"iat": now.Unix(),
		"exp": exp.Unix(),
		"jti": randomID(),
		"events": map[string]interface{}{
			"http://schemas.openid.net/event/backchannel-logout": map[string]interface{}{},
		},
	}
	if p.Subject != "" {
		claims["sub"] = p.Subject
	}
	if p.SessionID != "" {
		claims["sid"] = p.SessionID
	}
	token, err := i.certs.SignTyp(claims, "logout+jwt")
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

// VerifyToken 校验并解析由本签发器签发的 JWT。
func (i *Issuer) VerifyToken(token string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	if err := i.certs.Verify(token, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// NewCode 生成授权码（一次性随机串）
func (i *Issuer) NewCode() string { return randomID() }

// NewRefreshToken 生成 refresh_token（一次性随机串）
func (i *Issuer) NewRefreshToken() string { return randomID() + randomID() }

func randomID() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
