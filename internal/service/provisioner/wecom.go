package provisioner

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"polaris/internal/service/syncer"
)

// wecomBase 企业微信接口地址（测试可覆盖）
var wecomBase = "https://qyapi.weixin.qq.com"

// WeComProvisioner 企业微信下游连接器：把本地账号推送到企业微信通讯录。
//
// 凭据：CorpID（app_id）+ 通讯录 Secret（app_secret），与入方向一致。
//
// 能力（依据官方文档「创建成员 / 更新成员 / 读取成员 / 删除成员」核对）：
//   - **不支持设置口令**（企业微信账号在企微侧管理，无独立密码）；
//   - **支持停用/启用**：`user/update` 的 `enable` 字段（1 启用、0 禁用）——这是官方字段，
//     因此"禁用即停用下游账号"的策略在企微上可以真正生效；
//   - 建号必须指定所属部门，未配置 `dept_id` 时归入根部门（1）；
//   - **注意**：删除成员接口官方标注"仅通讯录同步助手或第三方通讯录应用可调用"，
//     普通自建应用可能无权限，届时会如实报错（不会被静默忽略）。
type WeComProvisioner struct{}

// Type 连接器类型标识
func (w *WeComProvisioner) Type() string { return "wecom" }

// Capabilities 企业微信能力：建号 / 更新 / 启用停用 / 删除；无口令
func (w *WeComProvisioner) Capabilities(_ Config) Capabilities {
	return Capabilities{Password: false, Update: true, Disable: true, Delete: true}
}

// accessToken 取通讯录 access_token
func (w *WeComProvisioner) accessToken(ctx context.Context, cfg Config) (string, error) {
	corpID := cfg.Value("app_id")
	secret := cfg.Value("app_secret")
	if corpID == "" || secret == "" {
		return "", fmt.Errorf("企业微信连接器缺少 CorpID / Secret")
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		wecomBase, url.QueryEscape(corpID), url.QueryEscape(secret))
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := syncer.RequestJSON(ctx, http.MethodGet, endpoint, nil, nil, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 || resp.AccessToken == "" {
		return "", fmt.Errorf("企业微信获取 token 失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken, nil
}

// call 调用通讯录接口；query 会被追加 access_token
func (w *WeComProvisioner) call(ctx context.Context, cfg Config, method, path string, query map[string]string, body interface{}) error {
	token, err := w.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	values := url.Values{}
	values.Set("access_token", token)
	for key, value := range query {
		values.Set(key, value)
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	endpoint := fmt.Sprintf("%s%s?%s", wecomBase, path, values.Encode())
	if err := syncer.RequestJSON(ctx, method, endpoint, nil, body, &resp); err != nil {
		return err
	}
	if resp.ErrCode != 0 {
		return fmt.Errorf("企业微信接口返回错误: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return nil
}

// department 账号所属部门（建号必填，缺省归入根部门）
func wecomDepartment(cfg Config) []int {
	if raw := cfg.Value("dept_id"); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil && id > 0 {
			return []int{id}
		}
	}
	return []int{1}
}

// Provision 建号：以本地账号名作为 userid
func (w *WeComProvisioner) Provision(ctx context.Context, cfg Config, account *Account) (string, error) {
	body := map[string]interface{}{
		"userid":     account.Username,
		"name":       firstNonEmpty(account.DisplayName, account.Username),
		"department": wecomDepartment(cfg),
		"enable":     1, // 官方字段：1 启用、0 禁用（新建成员显式启用）
	}
	if account.Phone != "" {
		body["mobile"] = account.Phone
	}
	if account.Email != "" {
		body["email"] = account.Email
	}
	// 目标端允许的字段（官方文档「创建成员」）：position / telephone / address / alias / email
	applyAttributes(body, account, "position", "telephone", "address", "alias")
	if err := w.call(ctx, cfg, http.MethodPost, "/cgi-bin/user/create", nil, body); err != nil {
		return "", err
	}
	return account.Username, nil
}

// Update 更新账号属性（不含口令）
func (w *WeComProvisioner) Update(ctx context.Context, cfg Config, account *Account) error {
	body := map[string]interface{}{"userid": account.Username}
	if account.DisplayName != "" {
		body["name"] = account.DisplayName
	}
	if account.Phone != "" {
		body["mobile"] = account.Phone
	}
	if account.Email != "" {
		body["email"] = account.Email
	}
	applyAttributes(body, account, "position", "telephone", "address", "alias")
	return w.call(ctx, cfg, http.MethodPost, "/cgi-bin/user/update", nil, body)
}

// Enable 启用成员（官方 `user/update` 的 enable 字段：1 启用）
func (w *WeComProvisioner) Enable(ctx context.Context, cfg Config, account *Account) error {
	return w.setEnabled(ctx, cfg, account, 1)
}

// Disable 停用成员（官方 `user/update` 的 enable 字段：0 禁用）
func (w *WeComProvisioner) Disable(ctx context.Context, cfg Config, account *Account) error {
	return w.setEnabled(ctx, cfg, account, 0)
}

// setEnabled 通过更新成员接口设置启用状态（官方文档：enable 1 启用 / 0 禁用）
func (w *WeComProvisioner) setEnabled(ctx context.Context, cfg Config, account *Account, enable int) error {
	return w.call(ctx, cfg, http.MethodPost, "/cgi-bin/user/update", nil, map[string]interface{}{
		"userid": account.Username, "enable": enable,
	})
}

// Delete 删除成员（企业微信该接口为 GET + userid 查询参数）
func (w *WeComProvisioner) Delete(ctx context.Context, cfg Config, account *Account) error {
	return w.call(ctx, cfg, http.MethodGet, "/cgi-bin/user/delete",
		map[string]string{"userid": account.Username}, nil)
}

// ExistsUser 查询企业微信成员是否存在（user/get：60111 = 成员不存在）。
func (w *WeComProvisioner) ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error) {
	token, err := w.accessToken(ctx, cfg)
	if err != nil {
		return ExistsResult{}, err
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	endpoint := fmt.Sprintf("%s/cgi-bin/user/get?access_token=%s&userid=%s",
		wecomBase, url.QueryEscape(token), url.QueryEscape(username))
	if err := syncer.RequestJSON(ctx, http.MethodGet, endpoint, nil, nil, &resp); err != nil {
		return ExistsResult{}, err
	}
	switch resp.ErrCode {
	case 0:
		return ExistsResult{State: ExistsYes}, nil
	case 60111: // 企业微信：成员不存在
		return ExistsResult{State: ExistsNo}, nil
	default:
		return ExistsResult{State: ExistsUnknown, Detail: fmt.Sprintf("errcode=%d %s", resp.ErrCode, resp.ErrMsg)}, nil
	}
}

// Ping 连通性自检：能取到 access_token 即视为可用
func (w *WeComProvisioner) Ping(ctx context.Context, cfg Config) error {
	_, err := w.accessToken(ctx, cfg)
	return err
}
