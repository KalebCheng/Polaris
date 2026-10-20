// Package config 负责应用配置的加载与环境变量覆盖。
//
// 安全约定（见 AGENTS.md §3.5）：configs/app.yaml 只允许占位符，
// 真实密钥/环境相关配置必须通过 POLARIS_* 环境变量注入。
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构
type Config struct {
	App      AppConfig      `yaml:"app"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Auth     AuthConfig     `yaml:"auth"`
	Security SecurityConfig `yaml:"security"`
	CORS     CORSConfig     `yaml:"cors"`
}

// AppConfig 应用配置
type AppConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Env     string `yaml:"env"`
	Port    int    `yaml:"port"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// DatabaseConfig PostgreSQL 配置
type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Name            string `yaml:"name"`
	SSLMode         string `yaml:"sslmode"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
	AutoMigrate     bool   `yaml:"auto_migrate"`
	MigrationsDir   string `yaml:"migrations_dir"`
	Seed            bool   `yaml:"seed"` // 启动时写入种子数据（仅开发环境建议开启）
}

// DSN 生成 PostgreSQL 连接字符串
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
}

// Addr 生成 Redis 地址
func (c *RedisConfig) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

// AuthConfig 认证配置
type AuthConfig struct {
	JWTSecret  string          `yaml:"jwt_secret"`
	SessionTTL int             `yaml:"session_ttl"`
	RefreshTTL int             `yaml:"refresh_ttl"`
	Issuer     string          `yaml:"issuer"` // JWT issuer（默认回退到 app.name）
	RateLimit  RateLimitConfig `yaml:"rate_limit"`
	OIDC       OIDCConfig      `yaml:"oidc"`  // 作为 OIDC Provider（IdP）对外提供的能力
	OAuth      OAuthConfig     `yaml:"oauth"` // 作为 SP 接入外部 IdP 的能力
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	LoginMaxAttempts   int `yaml:"login_max_attempts"`
	LoginWindowSeconds int `yaml:"login_window_seconds"`
}

// OIDCConfig 作为 OIDC Provider 的配置
type OIDCConfig struct {
	// Issuer 对外展示的 issuer（默认回退到 oauth.base_url）
	Issuer string `yaml:"issuer"`
	// CertName 用于签发 id_token 的证书名（certs 表；为空则使用内置自动生成的自签名证书）
	CertName string `yaml:"cert_name"`
	// AdditionalKeyCertNames 附加校验密钥的证书名（除 CertName 外；用于 JWKS 多密钥平滑轮换）。
	// 这些证书的公钥会一并发布到 JWKS 并用于验签既有令牌，但不用于签发新令牌。
	AdditionalKeyCertNames []string `yaml:"additional_key_cert_names"`
	// AccessTokenExpireHours access_token 默认有效期（小时）
	AccessTokenExpireHours int `yaml:"access_token_expire_hours"`
	// RefreshTokenExpireHours refresh_token 默认有效期（小时）
	RefreshTokenExpireHours int `yaml:"refresh_token_expire_hours"`
	// AuthorizationCodeExpireSeconds 授权码有效期（秒）
	AuthorizationCodeExpireSeconds int `yaml:"authorization_code_expire_seconds"`
}

// OAuthConfig 接入外部 IdP（SP 角色）配置
type OAuthConfig struct {
	BaseURL         string `yaml:"base_url"`
	SuccessRedirect string `yaml:"success_redirect"`
	RedirectURI     string `yaml:"redirect_uri"`
	StateTTL        int    `yaml:"state_ttl"`
	CookieSecure    bool   `yaml:"cookie_secure"`
	// DefaultInstitution SSO 自动建号/关联的落地组织（Provider 未指定时的兜底）
	DefaultInstitution string                         `yaml:"default_institution"`
	Providers          map[string]OAuthProviderConfig `yaml:"providers"`
}

// ProvisioningConfig 账号关联与建号策略（挂在具体 Provider 上配置）。
type ProvisioningConfig struct {
	AutoCreate     bool     `yaml:"auto_create"`
	AllowedDomains []string `yaml:"allowed_domains"`
	LinkByEmail    bool     `yaml:"link_by_email"`
}

// DefaultProvisioning 返回代码级默认供给策略。
func DefaultProvisioning() ProvisioningConfig {
	return ProvisioningConfig{AutoCreate: true, LinkByEmail: true}
}

// OAuthProviderConfig 单个外部身份提供方（SP 角色）配置。
//
// 注意：自第四阶段起，外部身份提供方的**权威来源是 providers 表**（控制台「提供商」），
// 本结构仅作为两种用途：(1) configs/app.yaml 的迁移期种子；(2) 运行时把表行转换为连接器构造参数的载体。
type OAuthProviderConfig struct {
	Type         string              `yaml:"type"` // oidc | dingtalk | wechat | feishu | lark
	Enabled      bool                `yaml:"enabled"`
	DisplayName  string              `yaml:"display_name"`
	Issuer       string              `yaml:"issuer"`
	ClientID     string              `yaml:"client_id"`
	ClientSecret string              `yaml:"client_secret"`
	Scopes       []string            `yaml:"scopes"`
	RedirectURI  string              `yaml:"redirect_uri"`
	Provisioning *ProvisioningConfig `yaml:"provisioning"`
	// 自定义端点（非标准 OIDC）：授权/令牌/用户信息三项齐全时不再执行 discovery
	AuthURL     string `yaml:"auth_url"`
	TokenURL    string `yaml:"token_url"`
	UserInfoURL string `yaml:"user_info_url"`
	// EnablePKCE 启用授权码 + PKCE（S256）
	EnablePKCE bool `yaml:"enable_pkce"`
}

// CORSConfig 跨域配置
type CORSConfig struct {
	AllowOrigins []string `yaml:"allow_origins"`
	AllowMethods []string `yaml:"allow_methods"`
	AllowHeaders []string `yaml:"allow_headers"`
}

// SecurityConfig 安全配置。
// 注意：CSRF 白名单与 CORS 白名单语义不同、相互独立——
//   - cors.allow_origins 控制"谁可以读取跨域响应"（浏览器强制）；
//   - security.csrf_allowed_origins 控制"谁可以携带 Cookie 环境凭证执行写操作"（服务端强制）。
//
// 空 = 跳过 Origin 校验（仅限开发）；生产必须显式配置。
type SecurityConfig struct {
	CSRFAllowedOrigins          []string `yaml:"csrf_allowed_origins"`
	CredentialEncKey            string   `yaml:"credential_enc_key"`
	AccessKeyRateLimitMax       int64    `yaml:"access_key_rate_limit_max"`
	AccessKeyRateLimitWindowSec int64    `yaml:"access_key_rate_limit_window_seconds"`
}

// Manager 配置管理器
type Manager struct {
	cfg      *Config
	mu       sync.RWMutex
	watchers []func(*Config)
}

// NewManager 创建配置管理器
func NewManager() *Manager { return &Manager{cfg: &Config{}, watchers: make([]func(*Config), 0)} }

// LoadFromFile 从本地 YAML 文件加载配置
func (m *Manager) LoadFromFile(path string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return nil
}

// Load 独立加载配置：读取 YAML 并应用 POLARIS_* 环境变量覆盖。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

// Get 获取当前配置（只读）
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update 更新配置
func (m *Manager) Update(cfg *Config) {
	m.mu.Lock()
	m.cfg = cfg
	watchers := make([]func(*Config), len(m.watchers))
	copy(watchers, m.watchers)
	m.mu.Unlock()
	for _, w := range watchers {
		w(cfg)
	}
}

// Watch 注册配置变更监听器
func (m *Manager) Watch(fn func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchers = append(m.watchers, fn)
}

// ── POLARIS_* 环境变量常量 ────────────────────────────────────
const (
	EnvConfigPath         = "POLARIS_CONFIG"
	EnvAppEnv             = "POLARIS_ENV"
	EnvAppPort            = "POLARIS_PORT"
	EnvDBHost             = "POLARIS_DB_HOST"
	EnvDBPort             = "POLARIS_DB_PORT"
	EnvDBUser             = "POLARIS_DB_USER"
	EnvDBPassword         = "POLARIS_DB_PASSWORD"
	EnvDBName             = "POLARIS_DB_NAME"
	EnvDBSSLMode          = "POLARIS_DB_SSLMODE"
	EnvRedisHost          = "POLARIS_REDIS_HOST"
	EnvRedisPort          = "POLARIS_REDIS_PORT"
	EnvRedisDB            = "POLARIS_REDIS_DB"
	EnvRedisPassword      = "POLARIS_REDIS_PASSWORD"
	EnvJWTSecret          = "POLARIS_JWT_SECRET"
	EnvCookieSecure       = "POLARIS_COOKIE_SECURE"
	EnvLoginMaxAttempts   = "POLARIS_LOGIN_MAX_ATTEMPTS"
	EnvLoginWindow        = "POLARIS_LOGIN_WINDOW_SECONDS"
	EnvDBAutoMigrate      = "POLARIS_DB_AUTO_MIGRATE"
	EnvMigrationsDir      = "POLARIS_MIGRATIONS_DIR"
	EnvSeed               = "POLARIS_DB_SEED"
	EnvCSRFAllowed        = "POLARIS_CSRF_ALLOWED_ORIGINS"
	EnvCredentialEncKey   = "POLARIS_CREDENTIAL_ENC_KEY"
	EnvIssuer             = "POLARIS_ISSUER"
	EnvDefaultInstitution = "POLARIS_DEFAULT_INSTITUTION"
	EnvOIDCAdditionalKeys = "POLARIS_OIDC_ADDITIONAL_KEY_CERT_NAMES"

	// OAuth（作为 SP 接入外部 IdP）的对外地址。生产环境必须覆盖为公网地址；
	// 只改 issuer 不够：开发信息里的回调地址读取的是 redirect_uri。
	EnvOAuthBaseURL         = "POLARIS_OAUTH_BASE_URL"
	EnvOAuthRedirectURI     = "POLARIS_OAUTH_REDIRECT_URI"
	EnvOAuthSuccessRedirect = "POLARIS_OAUTH_SUCCESS_REDIRECT"

	EnvOAuthSecretPrefix = "POLARIS_OAUTH_"
	EnvOAuthSecretSuffix = "_CLIENT_SECRET"
)

func applyEnvOverrides(cfg *Config) {
	cfg.App.Env = envOr(EnvAppEnv, cfg.App.Env)
	cfg.App.Port = envIntOr(EnvAppPort, cfg.App.Port)

	cfg.Database.Host = envOr(EnvDBHost, cfg.Database.Host)
	cfg.Database.Port = envIntOr(EnvDBPort, cfg.Database.Port)
	cfg.Database.User = envOr(EnvDBUser, cfg.Database.User)
	cfg.Database.Password = envOrAllowEmpty(EnvDBPassword, cfg.Database.Password)
	cfg.Database.Name = envOr(EnvDBName, cfg.Database.Name)
	cfg.Database.SSLMode = envOr(EnvDBSSLMode, cfg.Database.SSLMode)
	cfg.Database.AutoMigrate = envBoolOr(EnvDBAutoMigrate, cfg.Database.AutoMigrate)
	cfg.Database.MigrationsDir = envOr(EnvMigrationsDir, cfg.Database.MigrationsDir)
	cfg.Database.Seed = envBoolOr(EnvSeed, cfg.Database.Seed)

	cfg.Redis.Host = envOr(EnvRedisHost, cfg.Redis.Host)
	cfg.Redis.Port = envIntOr(EnvRedisPort, cfg.Redis.Port)
	cfg.Redis.DB = envIntOr(EnvRedisDB, cfg.Redis.DB)
	cfg.Redis.Password = envOrAllowEmpty(EnvRedisPassword, cfg.Redis.Password)

	if v := os.Getenv(EnvCSRFAllowed); v != "" {
		parts := strings.Split(v, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				list = append(list, p)
			}
		}
		cfg.Security.CSRFAllowedOrigins = list
	}
	cfg.Security.CredentialEncKey = envOr(EnvCredentialEncKey, cfg.Security.CredentialEncKey)

	cfg.Auth.JWTSecret = envOr(EnvJWTSecret, cfg.Auth.JWTSecret)

	// OAuth 对外地址：显式环境变量优先级最高（即使显式写成 127.0.0.1 也保留，便于本地联调）。
	oauthBaseSet := false
	if v := os.Getenv(EnvOAuthBaseURL); v != "" {
		cfg.Auth.OAuth.BaseURL = v
		oauthBaseSet = true
	}
	oauthRedirectSet := false
	if v := os.Getenv(EnvOAuthRedirectURI); v != "" {
		cfg.Auth.OAuth.RedirectURI = v
		oauthRedirectSet = true
	}
	oauthSuccessSet := false
	if v := os.Getenv(EnvOAuthSuccessRedirect); v != "" {
		cfg.Auth.OAuth.SuccessRedirect = v
		oauthSuccessSet = true
	}
	// POLARIS_ISSUER 是对外可见的 OIDC issuer（discovery / ID Token / 登录回跳），
	// 必须同时写入 auth.oidc.issuer；否则 discovery 会回退到 auth.oauth.base_url 的本地地址，
	// 导致外部 IdP（如 NetBird）校验 issuer 失败。
	//
	// 同时，如果 YAML 里仍是示例的 127.0.0.1 / localhost，而部署已经配置了公网 issuer，
	// 则用 issuer 的 origin 纠正 base_url / redirect_uri / success_redirect——
	// 否则控制台「开发信息」会继续展示本地地址，复制到 IdP 后台必然登不回来。
	if v := os.Getenv(EnvIssuer); v != "" {
		cfg.Auth.Issuer = v
		cfg.Auth.OIDC.Issuer = v
	}
	// 公网 issuer 可能来自环境变量，也可能直接写在 YAML 的 auth.oidc.issuer；
	// 两种来源都要参与 loopback 纠正。
	if origin := publicOrigin(cfg.Auth.OIDC.Issuer); origin != "" {
		if !oauthBaseSet && (cfg.Auth.OAuth.BaseURL == "" || isLoopbackURL(cfg.Auth.OAuth.BaseURL)) {
			cfg.Auth.OAuth.BaseURL = origin
		}
		if !oauthRedirectSet && (cfg.Auth.OAuth.RedirectURI == "" || isLoopbackURL(cfg.Auth.OAuth.RedirectURI)) {
			cfg.Auth.OAuth.RedirectURI = origin + "/sso/callback"
		}
		if !oauthSuccessSet && (cfg.Auth.OAuth.SuccessRedirect == "" || isLoopbackURL(cfg.Auth.OAuth.SuccessRedirect)) {
			cfg.Auth.OAuth.SuccessRedirect = origin + "/"
		}
	}
	cfg.Auth.OAuth.CookieSecure = envBoolOr(EnvCookieSecure, cfg.Auth.OAuth.CookieSecure)
	cfg.Auth.OAuth.DefaultInstitution = envOr(EnvDefaultInstitution, cfg.Auth.OAuth.DefaultInstitution)
	if v := os.Getenv(EnvOIDCAdditionalKeys); v != "" {
		parts := strings.Split(v, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				list = append(list, p)
			}
		}
		cfg.Auth.OIDC.AdditionalKeyCertNames = list
	}
	cfg.Auth.RateLimit.LoginMaxAttempts = envIntOr(EnvLoginMaxAttempts, cfg.Auth.RateLimit.LoginMaxAttempts)
	cfg.Auth.RateLimit.LoginWindowSeconds = envIntOr(EnvLoginWindow, cfg.Auth.RateLimit.LoginWindowSeconds)

	for name, prov := range cfg.Auth.OAuth.Providers {
		key := EnvOAuthSecretPrefix + normalizeEnvName(name) + EnvOAuthSecretSuffix
		if v := os.Getenv(key); v != "" {
			prov.ClientSecret = v
			cfg.Auth.OAuth.Providers[name] = prov
		}
	}
}

// publicOrigin 从 URL 中提取 scheme://host（丢弃路径），用于把公网 issuer 回填到 OAuth 配置。
func publicOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// isLoopbackURL 判断地址是否仍指向本机（示例配置 / 本地开发）。
func isLoopbackURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func normalizeEnvName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func envOr(key, current string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return current
}

func envIntOr(key string, current int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return current
}

// envOrAllowEmpty 与 envOr 的区别：环境变量存在但值为空时也覆盖，
// 用于"密码为空"本身有意义的配置（如无认证 Redis）。
func envOrAllowEmpty(key, current string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return current
}

func envBoolOr(key string, current bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1"
	}
	return current
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{Name: "polaris", Version: "0.1.0", Env: "development", Port: 8100},
		Log: LogConfig{Level: "info", Format: "json", Output: "stdout"},
		Database: DatabaseConfig{
			Host: "localhost", Port: 5432, User: "polaris", Password: "",
			Name: "polaris", SSLMode: "disable", MaxOpenConns: 25, MaxIdleConns: 5,
			ConnMaxLifetime: 300, AutoMigrate: true, MigrationsDir: "migrations", Seed: true,
		},
		Redis: RedisConfig{Host: "localhost", Port: 6379, Password: "", DB: 0, PoolSize: 10, MinIdleConns: 2},
		Auth: AuthConfig{
			JWTSecret:  "",
			SessionTTL: 3600,
			RefreshTTL: 7200,
			Issuer:     "polaris",
			RateLimit:  RateLimitConfig{LoginMaxAttempts: 5, LoginWindowSeconds: 300},
			OIDC: OIDCConfig{
				AccessTokenExpireHours:         1,
				RefreshTokenExpireHours:        24,
				AuthorizationCodeExpireSeconds: 300,
			},
			OAuth: OAuthConfig{StateTTL: 600, Providers: make(map[string]OAuthProviderConfig)},
		},
		CORS: CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders: []string{"Authorization", "Content-Type", "X-Request-ID"},
		},
		Security: SecurityConfig{CSRFAllowedOrigins: []string{}, AccessKeyRateLimitMax: 10, AccessKeyRateLimitWindowSec: 900},
	}
}

// IssuerURL 返回当前服务作为 IdP 的 issuer 地址。
func (c *Config) IssuerURL() string {
	if c.Auth.OIDC.Issuer != "" {
		return c.Auth.OIDC.Issuer
	}
	if c.Auth.OAuth.BaseURL != "" {
		return c.Auth.OAuth.BaseURL
	}
	return fmt.Sprintf("http://localhost:%d", c.App.Port)
}
