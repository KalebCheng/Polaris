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
	if p.Mode != "jit" || !p.AutoCreate || !p.LinkByEmail {
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
