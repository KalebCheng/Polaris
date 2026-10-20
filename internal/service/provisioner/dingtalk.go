package provisioner

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"polaris/internal/service/syncer"
)

// dingTalkBase 开放平台地址（测试可覆盖，指向 httptest 假下游）
var dingTalkBase = "https://oapi.dingtalk.com"

// DingTalkProvisioner 钉钉（企业内部应用）下游连接器：把本地账号推送到钉钉通讯录。
//
// 与入方向复用同一套应用凭据（AppKey / AppSecret）：先用凭据换 access_token，再调用通讯录接口。
// 能力差异（与"通用 HTTP"不同，配置与界面都按此收敛）：
//   - **不支持设置口令**：钉钉是 SSO 场景，账号没有独立密码 → 服务层不生成、不保存下游口令；
//   - **不支持"禁用但保留"**：钉钉通讯录没有停用态，只能删除 → 由服务层提前跳过并如实记录；
//   - 建号时钉钉要求由调用方指定 userid（我们沿用本地账号名）。
type DingTalkProvisioner struct{}

// Type 连接器类型标识
func (d *DingTalkProvisioner) Type() string { return "dingtalk" }

// Capabilities 钉钉能力：建号 / 更新 / 删除；无口令、无禁用
func (d *DingTalkProvisioner) Capabilities(_ Config) Capabilities {
	return Capabilities{Password: false, Update: true, Disable: false, Delete: true}
}

// accessToken 用 AppKey/AppSecret 换取 access_token
func (d *DingTalkProvisioner) accessToken(ctx context.Context, cfg Config) (string, error) {
	appKey := cfg.Value("app_id")
	appSecret := cfg.Value("app_secret")
	if appKey == "" || appSecret == "" {
		return "", fmt.Errorf("钉钉连接器缺少 AppKey / AppSecret")
	}
	endpoint := fmt.Sprintf("%s/gettoken?appkey=%s&appsecret=%s",
		dingTalkBase, url.QueryEscape(appKey), url.QueryEscape(appSecret))
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := syncer.RequestJSON(ctx, http.MethodGet, endpoint, nil, nil, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 || resp.AccessToken == "" {
		return "", fmt.Errorf("钉钉获取 token 失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken, nil
}

// call 调用需要 access_token 的接口，并把钉钉的 errcode 归一为错误
func (d *DingTalkProvisioner) call(ctx context.Context, cfg Config, path string, body map[string]interface{}) error {
	token, err := d.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	endpoint := fmt.Sprintf("%s%s?access_token=%s", dingTalkBase, path, url.QueryEscape(token))
	if err := syncer.RequestJSON(ctx, http.MethodPost, endpoint, nil, body, &resp); err != nil {
		return err
	}
	if resp.ErrCode != 0 {
		return fmt.Errorf("钉钉接口返回错误: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return nil
}

// deptIDs 取账号应归属的钉钉部门列表（留空时由钉钉归入默认部门）
func deptIDs(cfg Config) []int64 {
	raw := cfg.Value("dept_id")
	if raw == "" {
		return nil
	}
	var id int64
	if _, err := fmt.Sscanf(raw, "%d", &id); err != nil || id <= 0 {
		return nil
	}
	return []int64{id}
}

// Provision 建号：钉钉要求由调用方指定 userid（沿用本地账号名）
func (d *DingTalkProvisioner) Provision(ctx context.Context, cfg Config, account *Account) (string, error) {
	body := map[string]interface{}{
		"userid": account.Username,
		"name":   firstNonEmpty(account.DisplayName, account.Username),
	}
	if account.Phone != "" {
		body["mobile"] = account.Phone
	}
	if account.Email != "" {
		body["email"] = account.Email
	}
	if depts := deptIDs(cfg); len(depts) > 0 {
		body["dept_id_list"] = depts
	}
	applyAttributes(body, account, "title", "work_place", "job_number")
	if err := d.call(ctx, cfg, "/topapi/v2/user/create", body); err != nil {
		return "", err
	}
	return account.Username, nil
}

// Update 更新账号属性（改密不适用：钉钉为 SSO 场景，口令由钉钉侧管理）
func (d *DingTalkProvisioner) Update(ctx context.Context, cfg Config, account *Account) error {
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
	if depts := deptIDs(cfg); len(depts) > 0 {
		body["dept_id_list"] = depts
	}
	applyAttributes(body, account, "title", "work_place", "job_number")
	return d.call(ctx, cfg, "/topapi/v2/user/update", body)
}

// Enable 钉钉通讯录没有"停用态"，无法重新启用一个已停用的账号
func (d *DingTalkProvisioner) Enable(ctx context.Context, cfg Config, account *Account) error {
	return ErrUnsupported
}

// Disable 钉钉通讯录没有"停用态"（只能删除）→ 明确不支持，由服务层跳过并如实记录
func (d *DingTalkProvisioner) Disable(ctx context.Context, cfg Config, account *Account) error {
	return ErrUnsupported
}

// Delete 删除账号
func (d *DingTalkProvisioner) Delete(ctx context.Context, cfg Config, account *Account) error {
	return d.call(ctx, cfg, "/topapi/v2/user/delete", map[string]interface{}{"userid": account.Username})
}

// ExistsUser 查询钉钉账号是否存在。
// 只有明确返回"用户不存在"（errcode 60121）才判定为不存在；其余错误码归为"无法判定"并带上原始错误码，
// **避免把权限不足/接口变更误判成"账号不存在"而重复建号**。
func (d *DingTalkProvisioner) ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error) {
	token, err := d.accessToken(ctx, cfg)
	if err != nil {
		return ExistsResult{}, err
	}
	var resp struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	endpoint := fmt.Sprintf("%s/topapi/v2/user/get?access_token=%s", dingTalkBase, url.QueryEscape(token))
	if err := syncer.RequestJSON(ctx, http.MethodPost, endpoint, nil,
		map[string]interface{}{"userid": username}, &resp); err != nil {
		return ExistsResult{}, err
	}
	switch resp.ErrCode {
	case 0:
		return ExistsResult{State: ExistsYes}, nil
	case 60121: // 钉钉文档：用户不存在
		return ExistsResult{State: ExistsNo}, nil
	default:
		return ExistsResult{State: ExistsUnknown, Detail: fmt.Sprintf("errcode=%d %s", resp.ErrCode, resp.ErrMsg)}, nil
	}
}

// Ping 连通性自检：能取到 token 即视为可用（同时校验 AppKey/AppSecret）
func (d *DingTalkProvisioner) Ping(ctx context.Context, cfg Config) error {
	_, err := d.accessToken(ctx, cfg)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
