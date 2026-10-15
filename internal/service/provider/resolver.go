package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"polaris/internal/auth/oauth"
	"polaris/internal/config"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// DefaultConnectorCacheTTL 连接器实例缓存时长。
//
// 缓存的是"构造结果"（OIDC discovery 是一次真实外呼），缓存键包含全部影响构造的字段，
// 因此**改配置即换键即重建**（配置变更立即生效）；TTL 只用于回收长期未用的实例。
const DefaultConnectorCacheTTL = 5 * time.Minute

// Resolver 把「提供商」行构造为可用的外部身份提供方连接器（SP 角色）。
//
// 与 oauth.Manager 的关系：Manager 是启动期静态注册表（第一至第三阶段的做法，
// 配置只能来自 configs/app.yaml 且改一行要重启）；Resolver 改为**按行按需构造**，
// 因此控制台里的任何修改都立即生效，无需重启进程。
type Resolver struct {
	runtime *Runtime
	client  *oauth.Client
	ttl     time.Duration

	mu    sync.RWMutex
	cache map[string]cachedConnector
}

type cachedConnector struct {
	connector oauth.Provider
	expiresAt time.Time
}

// NewResolver 创建连接器解析器
func NewResolver(runtime *Runtime, timeout time.Duration) *Resolver {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Resolver{
		runtime: runtime,
		client:  oauth.NewClient(timeout),
		ttl:     DefaultConnectorCacheTTL,
		cache:   map[string]cachedConnector{},
	}
}

// Get 由提供商行构造（或复用缓存的）连接器。item 需为 Runtime 解密后的行。
func (r *Resolver) Get(ctx context.Context, item *repository.Provider) (oauth.Provider, error) {
	if item == nil {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", "")
	}
	if NormalizeCategory(item.Category) != string(CategoryOAuth) {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "provider is not a sign-in provider").
			WithDetail("category", item.Category)
	}
	kind := Kind(item)
	key := cacheKey(item, kind)
	if connector, ok := r.lookup(key); ok {
		return connector, nil
	}
	connector, err := r.build(item, kind)
	if err != nil {
		return nil, err
	}
	r.store(key, connector)
	return connector, nil
}

// GetBySignin 按 (组织, 名称) 解析可用于登录的连接器（含启用校验）。
func (r *Resolver) GetBySignin(ctx context.Context, institutionID int64, name string) (oauth.Provider, *repository.Provider, error) {
	item, err := r.runtime.FindSignin(ctx, institutionID, name)
	if err != nil {
		return nil, nil, err
	}
	connector, err := r.Get(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	return connector, item, nil
}

// Invalidate 失效某提供商的缓存（更新/删除/启停后调用；本实例立即生效，跨实例由 TTL 兜底）。
func (r *Resolver) Invalidate(institutionID int64, name string) {
	prefix := fmt.Sprintf("%d:%s:", institutionID, name)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.cache {
		if strings.HasPrefix(key, prefix) {
			delete(r.cache, key)
		}
	}
}

func (r *Resolver) build(item *repository.Provider, kind string) (oauth.Provider, error) {
	cfg := config.OAuthProviderConfig{
		Type: kind, Enabled: true, DisplayName: item.DisplayName,
		Issuer: item.IssuerURL, ClientID: item.ClientID, ClientSecret: item.ClientSecret,
		Scopes: item.Scopes, RedirectURI: item.RedirectURI,
		AuthURL: item.CustomAuthURL, TokenURL: item.CustomTokenURL, UserInfoURL: item.CustomUserInfoURL,
		EnablePKCE: item.EnablePKCE,
	}
	switch kind {
	case TypeOIDC:
		connector, err := oauth.NewOIDCProvider(item.Name, cfg)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeOAuthProviderError,
				"identity provider is not reachable", err).WithDetail("provider", item.Name)
		}
		return connector, nil
	case TypeDingTalk:
		return oauth.NewDingTalkProvider(item.Name, cfg, r.client), nil
	case TypeWeChat:
		return oauth.NewWeChatProvider(item.Name, cfg, r.client), nil
	case TypeFeishu:
		return oauth.NewFeishuProvider(item.Name, cfg, r.client), nil
	case TypeLark:
		cfg.Type = TypeFeishu
		return oauth.NewFeishuProvider(item.Name, cfg, r.client), nil
	default:
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported provider type").
			WithDetail("type", kind)
	}
}

func (r *Resolver) lookup(key string) (oauth.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.connector, true
}

func (r *Resolver) store(key string, connector oauth.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = cachedConnector{connector: connector, expiresAt: time.Now().Add(r.ttl)}
}

// cacheKey 缓存键 = 组织:名称:配置指纹（任何影响构造的字段变化都会换键 → 立即重建）
func cacheKey(item *repository.Provider, kind string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		kind, item.ClientID, item.ClientSecret, item.IssuerURL,
		item.CustomAuthURL, item.CustomTokenURL, item.CustomUserInfoURL,
		strings.Join(item.Scopes, " "), fmt.Sprintf("%t", item.EnablePKCE), item.DisplayName,
	}, "\x00")))
	return fmt.Sprintf("%d:%s:%s", item.InstitutionID, item.Name, hex.EncodeToString(sum[:8]))
}
