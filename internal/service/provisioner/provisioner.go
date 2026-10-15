// Package provisioner 实现"推送型"下游应用连接器（账号供给 / 生命周期）。
//
// 与 service/syncer（从外部目录"拉取"用户）互补：provisioner 把本地账号"推送"到下游应用，
// 统一暴露 建号（Provision）/ 改密与更新（Update）/ 启用（Enable）/ 禁用（Disable）/ 删除（Delete）
// 契约，供 ProvisioningService 的供给任务调用。
//
// 契约（RESTful，base = connector.provider_url；鉴权为 HTTP Basic principal:credentials）：
//
//	POST   {base}/accounts                     建号  → 响应可含 {"username": "<下游账号名>"}
//	PUT    {base}/accounts/{username}          更新/改密
//	POST   {base}/accounts/{username}/enable   启用
//	POST   {base}/accounts/{username}/disable  禁用
//	DELETE {base}/accounts/{username}          删除
//	GET    {base}/health                       连通性自检
//
// 复用 service/syncer 的 HTTP 客户端基础设施（超时、响应上限、错误归一）。
package provisioner

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"polaris/internal/service/syncer"
)

// Account 推送到下游的账号视图（中立载荷；下游按 mapping 取用）。
type Account struct {
	Username    string            `json:"username"`
	DisplayName string            `json:"display_name,omitempty"`
	Email       string            `json:"email,omitempty"`
	Phone       string            `json:"phone,omitempty"`
	Password    string            `json:"password,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// Config 连接器的运行时配置（敏感字段已由上层解密）。
//
// Values 是"类型专属配置 + 已解密凭据"的合并视图（键与目录里声明的字段一致），
// 多类型连接器（钉钉/飞书/企微/LDAP…）统一从这里取值；ProviderURL/Principal/Credentials
// 保留给通用 HTTP 契约使用（等价于 Values 里的 provider_url/principal/credentials）。
type Config struct {
	Type        string
	Values      map[string]string
	ProviderURL string
	Principal   string
	Credentials string
}

// Value 读取配置项（Values 优先，其次兼容字段）
func (c Config) Value(key string) string {
	if c.Values != nil {
		if v, ok := c.Values[key]; ok && v != "" {
			return v
		}
	}
	switch key {
	case "provider_url":
		return c.ProviderURL
	case "principal":
		return c.Principal
	case "credentials":
		return c.Credentials
	}
	return ""
}

// Capabilities 下游连接器的能力声明。
// 服务层据此**提前拒绝**不支持的动作（而不是发请求后失败），并决定是否生成/保存下游口令。
type Capabilities struct {
	// Password 是否支持设置口令（钉钉等 SSO 场景不需要）
	Password bool
	// Update 是否支持更新账号属性
	Update bool
	// Disable 是否支持"禁用但保留账号"
	Disable bool
	// Delete 是否支持删除账号
	Delete bool
}

// applyAttributes 把账号策略映射出的下游属性**按目标端允许的字段名**原样带上。
//
// 约定：策略的 mapping 形如 {"本地属性": "下游字段名"}，因此 Attributes 的键就是下游字段名。
// 这里只透传目标端确实存在的字段（白名单），避免把任意键塞进请求体导致接口报参数错误。
func applyAttributes(body map[string]interface{}, account *Account, allowed ...string) {
	for _, key := range allowed {
		if value := account.Attributes[key]; value != "" {
			body[key] = value
		}
	}
}

// ErrUnsupported 下游明确不支持该动作（服务层据此跳过并记录，而不是无限重试）
var ErrUnsupported = errors.New("下游连接器不支持该动作")

// ExistsState 账号在下游的存在性判定结果
type ExistsState string

const (
	// ExistsYes 确认存在
	ExistsYes ExistsState = "exists"
	// ExistsNo 确认不存在（可安全补建号）
	ExistsNo ExistsState = "missing"
	// ExistsUnknown 无法判定（下游查询能力受限或返回了未映射的错误码）——
	// 这种情况**绝不能当作"不存在"**，否则对齐检查会重复建号。
	ExistsUnknown ExistsState = "unknown"
)

// ExistsResult 存在性判定结果；Unknown 时 Detail 携带可读原因（含下游错误码，便于逐步补齐映射）
type ExistsResult struct {
	State  ExistsState
	Detail string
}

// ExistsReporter 可选能力：下游是否支持"查询某个账号是否存在"（用于出方向对齐检查）。
// 未实现该接口的连接器一律视为"无法判定"。
type ExistsReporter interface {
	ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error)
}

// Provisioner 下游应用连接器的统一契约。
type Provisioner interface {
	Type() string
	// Provision 建号，返回下游账号名（下游未回传则回退为 account.Username）。
	Provision(ctx context.Context, cfg Config, account *Account) (string, error)
	// Update 更新账号属性 / 改密。
	Update(ctx context.Context, cfg Config, account *Account) error
	// Enable 启用账号。
	Enable(ctx context.Context, cfg Config, account *Account) error
	// Disable 禁用账号。
	Disable(ctx context.Context, cfg Config, account *Account) error
	// Delete 删除账号。
	Delete(ctx context.Context, cfg Config, account *Account) error
	// Ping 连通性自检。
	Ping(ctx context.Context, cfg Config) error
	// Capabilities 能力声明（服务层据此提前拒绝不支持的动作）。
	// 传入 cfg 是因为能力可能依赖配置：例如同一 LDAP 连接器，AD 目录支持禁用账号
	// （userAccountControl 位），而 OpenLDAP 没有等价的停用语义。
	Capabilities(cfg Config) Capabilities
}

var registry = map[string]Provisioner{}

// Register 注册连接器实现。
func Register(p Provisioner) { registry[p.Type()] = p }

// Get 按类型获取连接器。
func Get(provisionerType string) (Provisioner, error) {
	p, ok := registry[provisionerType]
	if !ok {
		return nil, fmt.Errorf("不支持的下游连接器类型: %s", provisionerType)
	}
	return p, nil
}

// Types 返回已注册的连接器类型。
func Types() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}

// Default 返回默认（通用 HTTP RESTful）连接器。
func Default() Provisioner { return registry["http"] }

// authHeader 构造 HTTP Basic 鉴权头（无 principal/credentials 时为空）。
func authHeader(cfg Config) map[string]string {
	principal := cfg.Value("principal")
	credentials := cfg.Value("credentials")
	if principal == "" && credentials == "" {
		return nil
	}
	token := base64.StdEncoding.EncodeToString([]byte(principal + ":" + credentials))
	return map[string]string{"Authorization": "Basic " + token}
}

// request 统一发请求（复用 syncer 的 JSON HTTP 基础设施）。
func request(ctx context.Context, method, url string, cfg Config, body, out interface{}) error {
	return syncer.RequestJSON(ctx, method, url, authHeader(cfg), body, out)
}

// init 注册内置连接器（与 syncer 一致：连接器无状态，注册表用 init 承载）。
func init() {
	Register(&HTTPProvisioner{})
	Register(&DingTalkProvisioner{})
	Register(&FeishuProvisioner{})
	Register(&WeComProvisioner{})
	Register(&LDAPProvisioner{})
}
