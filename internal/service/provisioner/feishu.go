package provisioner

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"polaris/internal/service/syncer"
)

// feishuBase 飞书开放平台地址（Lark 国际版为 https://open.larksuite.com；可由配置覆盖，测试也用它指向假下游）
var feishuBase = "https://open.feishu.cn"

// feishuPathPrefix 飞书开放平台 API 前缀
const feishuPathPrefix = "/open-apis"

// FeishuProvisioner 飞书 / Lark 下游连接器：把本地账号推送到飞书通讯录。
//
// 与入方向复用同一套应用凭据（App ID / App Secret）：先取 tenant_access_token，再调用通讯录接口。
// 能力差异：
//   - **不支持设置口令**（企业 IM 账号由飞书侧管理，无独立密码）；
//   - **不支持"禁用但保留"**：飞书通讯录没有停用态，只能删除 → 由服务层提前跳过并如实记录；
//   - 国际版 Lark 通过 `base_url` 指向 open.larksuite.com（其余流程一致）。
type FeishuProvisioner struct{}

// Type 连接器类型标识（与入方向保持一致：lark 覆盖飞书与 Lark）
func (f *FeishuProvisioner) Type() string { return "lark" }

// Capabilities 飞书能力：建号 / 更新 / 删除；无口令、无禁用
func (f *FeishuProvisioner) Capabilities(_ Config) Capabilities {
	return Capabilities{Password: false, Update: true, Disable: false, Delete: true}
}

// base 解析开放平台地址（默认飞书；Lark 国际版经 base_url 覆盖）
func (f *FeishuProvisioner) base(cfg Config) string {
	if custom := cfg.Value("base_url"); custom != "" {
		return strings.TrimRight(custom, "/")
	}
	return feishuBase
}

// tenantToken 取 tenant_access_token
func (f *FeishuProvisioner) tenantToken(ctx context.Context, cfg Config) (string, error) {
	appID := cfg.Value("app_id")
	appSecret := cfg.Value("app_secret")
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("飞书连接器缺少 App ID / App Secret")
	}
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	endpoint := f.base(cfg) + feishuPathPrefix + "/auth/v3/tenant_access_token/internal"
	if err := syncer.RequestJSON(ctx, http.MethodPost, endpoint, nil,
		map[string]string{"app_id": appID, "app_secret": appSecret}, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 || resp.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书获取 tenant_access_token 失败: %d %s", resp.Code, resp.Msg)
	}
	return resp.TenantAccessToken, nil
}

// call 调用通讯录接口（统一带 Bearer 鉴权并把 code 归一为错误）
func (f *FeishuProvisioner) call(ctx context.Context, cfg Config, method, path string, body interface{}) error {
	token, err := f.tenantToken(ctx, cfg)
	if err != nil {
		return err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	endpoint := f.base(cfg) + feishuPathPrefix + path
	if err := syncer.RequestJSON(ctx, method, endpoint,
		map[string]string{"Authorization": "Bearer " + token}, body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("飞书接口返回错误: %d %s", resp.Code, resp.Msg)
	}
	return nil
}

// userPath 用户接口路径（统一用 user_id 作为账号标识，即本地账号名）
func feishuUserPath(cfg Config, userID string) string {
	path := "/contact/v3/users/" + url.PathEscape(userID) + "?user_id_type=user_id"
	if cfg.Value("dept_id") != "" {
		path += "&department_id_type=open_department_id"
	}
	return path
}

// departmentIDs 取账号归属部门（可选）
func feishuDepartmentIDs(cfg Config) []string {
	if dept := cfg.Value("dept_id"); dept != "" {
		return []string{dept}
	}
	return nil
}

// Provision 建号：以本地账号名作为 user_id（便于后续更新/删除精确定位）
func (f *FeishuProvisioner) Provision(ctx context.Context, cfg Config, account *Account) (string, error) {
	body := map[string]interface{}{
		"user_id": account.Username,
		"name":    firstNonEmpty(account.DisplayName, account.Username),
	}
	if account.Phone != "" {
		body["mobile"] = account.Phone
	}
	if account.Email != "" {
		body["email"] = account.Email
	}
	if depts := feishuDepartmentIDs(cfg); len(depts) > 0 {
		body["department_ids"] = depts
	}
	// 目标端允许的字段（官方 SDK：contact v3 的 User 结构体）：job_title / city / employee_no
	applyAttributes(body, account, "job_title", "city", "employee_no")
	path := "/contact/v3/users?user_id_type=user_id"
	if len(feishuDepartmentIDs(cfg)) > 0 {
		path += "&department_id_type=open_department_id"
	}
	if err := f.call(ctx, cfg, http.MethodPost, path, body); err != nil {
		return "", err
	}
	return account.Username, nil
}

// Update 更新账号属性（不含口令：飞书账号无独立密码）
func (f *FeishuProvisioner) Update(ctx context.Context, cfg Config, account *Account) error {
	body := map[string]interface{}{}
	if account.DisplayName != "" {
		body["name"] = account.DisplayName
	}
	if account.Phone != "" {
		body["mobile"] = account.Phone
	}
	if account.Email != "" {
		body["email"] = account.Email
	}
	if depts := feishuDepartmentIDs(cfg); len(depts) > 0 {
		body["department_ids"] = depts
	}
	applyAttributes(body, account, "job_title", "city", "employee_no")
	return f.call(ctx, cfg, http.MethodPut, feishuUserPath(cfg, account.Username), body)
}

// Enable 飞书通讯录没有停用态，无法重新启用
func (f *FeishuProvisioner) Enable(ctx context.Context, cfg Config, account *Account) error {
	return ErrUnsupported
}

// Disable 飞书通讯录没有停用态（只能删除）→ 明确不支持，由服务层跳过并如实记录
func (f *FeishuProvisioner) Disable(ctx context.Context, cfg Config, account *Account) error {
	return ErrUnsupported
}

// Delete 删除账号
func (f *FeishuProvisioner) Delete(ctx context.Context, cfg Config, account *Account) error {
	return f.call(ctx, cfg, http.MethodDelete, feishuUserPath(cfg, account.Username), nil)
}

// ExistsUser 查询飞书账号是否存在。
// 飞书的错误码随版本变化较大，因此除了"明确的不存在码"以外一律归为"无法判定"（带原始码），
// 避免误判导致重复建号。
func (f *FeishuProvisioner) ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error) {
	token, err := f.tenantToken(ctx, cfg)
	if err != nil {
		return ExistsResult{}, err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	endpoint := f.base(cfg) + feishuPathPrefix + feishuUserPath(cfg, username)
	if err := syncer.RequestJSON(ctx, http.MethodGet, endpoint,
		map[string]string{"Authorization": "Bearer " + token}, nil, &resp); err != nil {
		return ExistsResult{}, err
	}
	switch resp.Code {
	case 0:
		return ExistsResult{State: ExistsYes}, nil
	case 43004: // 飞书：用户不存在（contact 接口常见返回码）
		return ExistsResult{State: ExistsNo}, nil
	default:
		return ExistsResult{State: ExistsUnknown, Detail: fmt.Sprintf("code=%d %s", resp.Code, resp.Msg)}, nil
	}
}

// Ping 连通性自检：能取到 tenant_access_token 即视为可用
func (f *FeishuProvisioner) Ping(ctx context.Context, cfg Config) error {
	_, err := f.tenantToken(ctx, cfg)
	return err
}
