package provisioner

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"polaris/internal/service/syncer"
)

// HTTPProvisioner 通用 RESTful 下游连接器（默认实现）。
// 契约见包注释；账号名经 URL 转义后拼入路径。
type HTTPProvisioner struct{}

// Type 连接器类型标识。
func (h *HTTPProvisioner) Type() string { return "http" }

// Capabilities 通用 RESTful 契约支持全量动作（建号/改密/启用/禁用/删除）
func (h *HTTPProvisioner) Capabilities(_ Config) Capabilities {
	return Capabilities{Password: true, Update: true, Disable: true, Delete: true}
}

func accountsURL(cfg Config) string {
	return strings.TrimRight(cfg.Value("provider_url"), "/") + "/accounts"
}
func accountURL(cfg Config, name string) string {
	return accountsURL(cfg) + "/" + pathEscape(name)
}
func actionURL(cfg Config, name, action string) string {
	return accountURL(cfg, name) + "/" + action
}

// Provision 建号；下游返回 {"username": ...} 时以其为准。
func (h *HTTPProvisioner) Provision(ctx context.Context, cfg Config, account *Account) (string, error) {
	if cfg.Value("provider_url") == "" {
		return "", fmt.Errorf("下游连接器未配置 provider_url")
	}
	var out struct {
		Username string `json:"username"`
	}
	if err := request(ctx, http.MethodPost, accountsURL(cfg), cfg, account, &out); err != nil {
		return "", err
	}
	if out.Username != "" {
		return out.Username, nil
	}
	return account.Username, nil
}

// Update 更新账号属性 / 改密。
func (h *HTTPProvisioner) Update(ctx context.Context, cfg Config, account *Account) error {
	return request(ctx, http.MethodPut, accountURL(cfg, account.Username), cfg, account, nil)
}

// Enable 启用账号。
func (h *HTTPProvisioner) Enable(ctx context.Context, cfg Config, account *Account) error {
	return request(ctx, http.MethodPost, actionURL(cfg, account.Username, "enable"), cfg, nil, nil)
}

// Disable 禁用账号。
func (h *HTTPProvisioner) Disable(ctx context.Context, cfg Config, account *Account) error {
	return request(ctx, http.MethodPost, actionURL(cfg, account.Username, "disable"), cfg, nil, nil)
}

// Delete 删除账号。
func (h *HTTPProvisioner) Delete(ctx context.Context, cfg Config, account *Account) error {
	return request(ctx, http.MethodDelete, accountURL(cfg, account.Username), cfg, nil, nil)
}

// ExistsUser 查询下游是否存在该账号：
// 契约上 `GET {base}/accounts/{username}` 返回 2xx = 存在；404 = 不存在；其余状态码归为"无法判定"。
func (h *HTTPProvisioner) ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error) {
	if cfg.Value("provider_url") == "" {
		return ExistsResult{}, fmt.Errorf("下游连接器未配置 provider_url")
	}
	err := syncer.RequestJSON(ctx, http.MethodGet, accountURL(cfg, username), authHeader(cfg), nil, nil)
	if err == nil {
		return ExistsResult{State: ExistsYes}, nil
	}
	// 请求层把非 2xx 归一成错误：这里识别"404 → 不存在"，其余（含网络错误）都算无法判定
	if strings.Contains(err.Error(), "404") {
		return ExistsResult{State: ExistsNo}, nil
	}
	return ExistsResult{State: ExistsUnknown, Detail: err.Error()}, nil
}

// Ping 连通性自检。
func (h *HTTPProvisioner) Ping(ctx context.Context, cfg Config) error {
	if cfg.Value("provider_url") == "" {
		return fmt.Errorf("下游连接器未配置 provider_url")
	}
	return request(ctx, http.MethodGet, strings.TrimRight(cfg.ProviderURL, "/")+"/health", cfg, nil, nil)
}

// pathEscape 对账号名做最小转义（保留 @ . 等常用字符，仅转义 / 与空格）。
func pathEscape(name string) string {
	replacer := strings.NewReplacer("/", "%2F", " ", "%20", "?", "%3F", "#", "%23")
	return replacer.Replace(name)
}
