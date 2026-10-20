package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	content := `
app:
  name: polaris
  env: development
  port: 8100
database:
  host: yaml-db
  port: 5432
  user: polaris
  password: yaml-db-pass
  name: polaris
  sslmode: disable
redis:
  host: yaml-redis
  port: 6379
  db: 0
  password: yaml-placeholder
auth:
  jwt_secret: change-me
  session_ttl: 3600
  oauth:
    providers:
      dingtalk:
        type: dingtalk
        client_id: from-yaml
        client_secret: yaml-placeholder
security:
  csrf_allowed_origins: []
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	t.Setenv(EnvAppEnv, "production")
	t.Setenv(EnvAppPort, "9100")
	t.Setenv(EnvJWTSecret, "injected-secret")
	t.Setenv(EnvDBSSLMode, "require")
	t.Setenv(EnvRedisHost, "injected-redis")
	t.Setenv(EnvRedisPort, "6380")
	t.Setenv(EnvRedisDB, "3")
	t.Setenv(EnvRedisPassword, "injected-redis-pass")
	t.Setenv(EnvCSRFAllowed, "https://a.example.com, https://b.example.com")
	// Provider 级密钥：POLARIS_OAUTH_DINGTALK_CLIENT_SECRET
	t.Setenv(EnvOAuthSecretPrefix+"DINGTALK"+EnvOAuthSecretSuffix, "injected-client-secret")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.App.Env != "production" {
		t.Errorf("app.env = %q, 期望 production", cfg.App.Env)
	}
	if cfg.App.Port != 9100 {
		t.Errorf("app.port = %d, 期望 9100", cfg.App.Port)
	}
	if cfg.Auth.JWTSecret != "injected-secret" {
		t.Errorf("jwt_secret 未注入: %q", cfg.Auth.JWTSecret)
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("database.sslmode 未注入: %q", cfg.Database.SSLMode)
	}
	if cfg.Redis.Host != "injected-redis" || cfg.Redis.Port != 6380 || cfg.Redis.DB != 3 {
		t.Errorf("redis host/port/db 未注入: %+v", cfg.Redis)
	}
	if cfg.Redis.Password != "injected-redis-pass" {
		t.Errorf("redis 密码未注入: %q", cfg.Redis.Password)
	}
	if len(cfg.Security.CSRFAllowedOrigins) != 2 ||
		cfg.Security.CSRFAllowedOrigins[0] != "https://a.example.com" ||
		cfg.Security.CSRFAllowedOrigins[1] != "https://b.example.com" {
		t.Errorf("CSRF 白名单解析错误: %v", cfg.Security.CSRFAllowedOrigins)
	}
	prov, ok := cfg.Auth.OAuth.Providers["dingtalk"]
	if !ok {
		t.Fatal("未解析出 dingtalk provider")
	}
	if prov.ClientSecret != "injected-client-secret" {
		t.Errorf("provider client_secret 未注入: %q", prov.ClientSecret)
	}
	if prov.ClientID != "from-yaml" {
		t.Errorf("provider client_id 不应被覆盖: %q", prov.ClientID)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("配置文件不存在应当报错")
	}
}

// POLARIS_ISSUER 必须覆盖 OIDC discovery / ID Token 使用的 auth.oidc.issuer，
// 而不是只改内部 JWT 的 iss：否则外部 IdP 会读到 auth.oauth.base_url 的本地地址。
func TestLoadAppliesIssuerEnvToOIDCDiscovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	content := `
auth:
  oauth:
    base_url: http://127.0.0.1:8100
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	t.Setenv(EnvIssuer, "https://polaris.insuresmart.tech")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Auth.OIDC.Issuer != "https://polaris.insuresmart.tech" {
		t.Fatalf("auth.oidc.issuer = %q，期望被 POLARIS_ISSUER 覆盖", cfg.Auth.OIDC.Issuer)
	}
	if got := cfg.IssuerURL(); got != "https://polaris.insuresmart.tech" {
		t.Fatalf("IssuerURL = %q，期望使用 POLARIS_ISSUER 而不是 base_url", got)
	}
}

// 回归（2026-10）：部署配置了公网 POLARIS_ISSUER，但 YAML 里仍是示例的 127.0.0.1 时，
// OAuth 对外地址必须跟随 issuer，否则控制台「开发信息」会把本地回调地址显示给管理员。
func TestLoadUsesIssuerOriginForLoopbackOAuthURLs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	content := `
auth:
  oauth:
    base_url: http://127.0.0.1:8100
    success_redirect: http://127.0.0.1:5173/
    redirect_uri: http://127.0.0.1:5173/sso/callback
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	t.Setenv(EnvOAuthBaseURL, "")
	t.Setenv(EnvOAuthRedirectURI, "")
	t.Setenv(EnvOAuthSuccessRedirect, "")
	t.Setenv(EnvIssuer, "https://polaris.insuresmart.tech/")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got := cfg.Auth.OAuth.BaseURL; got != "https://polaris.insuresmart.tech" {
		t.Fatalf("base_url 应跟随 issuer，实际 %q", got)
	}
	if got := cfg.Auth.OAuth.RedirectURI; got != "https://polaris.insuresmart.tech/sso/callback" {
		t.Fatalf("redirect_uri 应跟随 issuer，实际 %q", got)
	}
	if got := cfg.Auth.OAuth.SuccessRedirect; got != "https://polaris.insuresmart.tech/" {
		t.Fatalf("success_redirect 应跟随 issuer，实际 %q", got)
	}
}

// 显式 POLARIS_OAUTH_* 优先于 issuer 回退（便于前后端分域部署 / 本地联调）。
func TestLoadOAuthEnvOverridesIssuerFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	if err := os.WriteFile(path, []byte(`
auth:
  oauth:
    base_url: http://127.0.0.1:8100
    redirect_uri: http://127.0.0.1:5173/sso/callback
`), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	t.Setenv(EnvIssuer, "https://polaris.example.com")
	t.Setenv(EnvOAuthBaseURL, "https://api.example.com")
	t.Setenv(EnvOAuthRedirectURI, "https://login.example.com/sso/callback")
	t.Setenv(EnvOAuthSuccessRedirect, "https://login.example.com/")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Auth.OAuth.BaseURL != "https://api.example.com" {
		t.Fatalf("显式 base_url 不应被 issuer 覆盖，实际 %q", cfg.Auth.OAuth.BaseURL)
	}
	if cfg.Auth.OAuth.RedirectURI != "https://login.example.com/sso/callback" {
		t.Fatalf("显式 redirect_uri 不应被 issuer 覆盖，实际 %q", cfg.Auth.OAuth.RedirectURI)
	}
	if cfg.Auth.OAuth.SuccessRedirect != "https://login.example.com/" {
		t.Fatalf("显式 success_redirect 不应被 issuer 覆盖，实际 %q", cfg.Auth.OAuth.SuccessRedirect)
	}
}

func TestDefaultConfigAndIssuerURL(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.App.Name != "polaris" {
		t.Fatalf("默认应用名 = %q", cfg.App.Name)
	}
	if cfg.Database.MigrationsDir != "migrations" {
		t.Fatalf("默认迁移目录 = %q", cfg.Database.MigrationsDir)
	}
	// 未配置 issuer 且未配置 base_url 时回退到本地地址
	if got := cfg.IssuerURL(); got != "http://localhost:8100" {
		t.Fatalf("IssuerURL = %q", got)
	}
	cfg.Auth.OAuth.BaseURL = "https://sso.example.com"
	if got := cfg.IssuerURL(); got != "https://sso.example.com" {
		t.Fatalf("IssuerURL 应回退 base_url，实际 %q", got)
	}
	cfg.Auth.OIDC.Issuer = "https://idp.example.com"
	if got := cfg.IssuerURL(); got != "https://idp.example.com" {
		t.Fatalf("IssuerURL 应优先 oidc.issuer，实际 %q", got)
	}
}

func TestNormalizeEnvName(t *testing.T) {
	cases := map[string]string{
		"dingtalk": "DINGTALK",
		"we-chat":  "WE_CHAT",
		"a.b.c":    "A_B_C",
		"FeiShu":   "FEISHU",
		"my_prov1": "MY_PROV1",
	}
	for in, want := range cases {
		if got := normalizeEnvName(in); got != want {
			t.Errorf("normalizeEnvName(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestDefaultProvisioning(t *testing.T) {
	p := DefaultProvisioning()
	if !p.AutoCreate || !p.LinkByEmail {
		t.Fatalf("代码级默认供给策略错误: %+v", p)
	}
}

func TestLoadAllowsEmptyPasswordOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	content := `
database:
  password: yaml-db-placeholder
redis:
  password: yaml-redis-placeholder
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	t.Setenv(EnvDBPassword, "")
	t.Setenv(EnvRedisPassword, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Database.Password != "" {
		t.Errorf("database.password 应允许被空值覆盖，实际 %q", cfg.Database.Password)
	}
	if cfg.Redis.Password != "" {
		t.Errorf("redis.password 应允许被空值覆盖，实际 %q", cfg.Redis.Password)
	}
}
