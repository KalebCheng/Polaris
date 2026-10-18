package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"polaris/internal/logger"
	"polaris/internal/repository"
)

// feishuBase 飞书开放平台地址（Lark 国际版为 https://open.larksuite.com）。
// 包级变量只为测试能指向假下游；连接器本身无状态（AGENTS.md §3.1-1 的红线针对有状态组件）。
var feishuBase = "https://open.feishu.cn"

// feishuPathPrefix 飞书开放平台 API 前缀
const feishuPathPrefix = "/open-apis"

// feishuPageSize 分页大小（官方上限即为 50）
const feishuPageSize = 50

// feishuRootDepartmentID 根部门 ID（部门 ID 类型为 open_department_id 时同样是 "0"）
const feishuRootDepartmentID = "0"

// feishuNoDeptAuthCode 飞书"无部门通讯录权限"业务码（应用被限定通讯录范围时从根部门递归会返回它）。
const feishuNoDeptAuthCode = 40004

// errNoDeptInScope 应用通讯录权限范围里没有任何部门。
// 此时既同步不了组织，"按部门枚举成员"也无从下手（用户同步会退回历史接口兜底）。
var errNoDeptInScope = errors.New("应用的通讯录权限范围里没有任何部门：" +
	"请在飞书开放平台把该应用的数据权限（通讯录范围）设为「全部员工」，" +
	"并开通「获取部门基础信息」「获取部门组织架构信息」权限后发布新版本")

// FeishuConnector 飞书（Lark）通讯录连接器
type FeishuConnector struct{}

// Type 连接器类型
func (c *FeishuConnector) Type() string { return "lark" }

func (c *FeishuConnector) tenantToken(ctx context.Context, s *repository.Syncer) (string, error) {
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	body := map[string]string{"app_id": s.AppID, "app_secret": s.AppSecret}
	if err := postJSON(ctx, feishuBase+feishuPathPrefix+"/auth/v3/tenant_access_token/internal", nil, body, &resp); err != nil {
		return "", fmt.Errorf("飞书获取 tenant_access_token 失败: %w", err)
	}
	if resp.Code != 0 || resp.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书获取 tenant_access_token 失败: %d %s", resp.Code, resp.Msg)
	}
	return resp.TenantAccessToken, nil
}

// ── 用户对象与"可映射源字段" ────────────────────────────────

// feishuUser 飞书通讯录用户对象（字段名与官方文档一一对应）。
// 未开通对应**字段权限**的字段会返回空值（如 email 需「获取用户邮箱信息」），这不影响解析。
type feishuUser struct {
	UserID          string `json:"user_id"`
	OpenID          string `json:"open_id"`
	UnionID         string `json:"union_id"`
	Name            string `json:"name"`
	EnName          string `json:"en_name"`
	Nickname        string `json:"nickname"` // 别名（官方用户对象字段）
	Email           string `json:"email"`
	EnterpriseEmail string `json:"enterprise_email"`
	Mobile          string `json:"mobile"`
	MobileVisible   bool   `json:"mobile_visible"`
	Gender          int    `json:"gender"`
	Avatar          struct {
		Avatar72 string `json:"avatar_72"`
	} `json:"avatar"`
	Status struct {
		IsFrozen    bool `json:"is_frozen"`
		IsResigned  bool `json:"is_resigned"`
		IsActivated bool `json:"is_activated"`
		IsExited    bool `json:"is_exited"`
	} `json:"status"`
	DepartmentIDs           []string `json:"department_ids"`
	LeaderUserID            string   `json:"leader_user_id"`
	DottedLineLeaderUserIDs []string `json:"dotted_line_leader_user_ids"`
	City                    string   `json:"city"`
	Country                 string   `json:"country"`
	WorkStation             string   `json:"work_station"`
	JoinTime                int64    `json:"join_time"`
	IsTenantManager         bool     `json:"is_tenant_manager"`
	EmployeeNo              string   `json:"employee_no"`
	EmployeeType            int      `json:"employee_type"`
	JobTitle                string   `json:"job_title"`
	JobLevelID              string   `json:"job_level_id"`
	JobFamilyID             string   `json:"job_family_id"`
	Description             string   `json:"description"`
}

// feishuUserSourceField 一条"可映射源字段"的声明：键名（取自官方文档字段名）+ 取值方式
type feishuUserSourceField struct {
	Key   string
	Value func(u feishuUser) string
}

// feishuUserSourceFields 飞书用户同步的**全部**可映射源字段（口径：按官方"用户对象"文档列出）。
//
// 是否真的有值取决于该应用的**字段权限**——权限没开时对应键为空串，界面上照样可选：
//   - email ←「获取用户邮箱信息」；
//   - mobile ←「获取用户手机号」；
//   - city / country / work_station / join_time / employee_no / employee_type / job_title /
//     enterprise_email / is_tenant_manager ←「获取用户受雇信息」（或以应用身份读取通讯录等任一）；
//   - leader_user_id / department_ids / department_path ←「获取用户组织架构信息」；
//   - job_level_id / job_family_id ←「查询用户职级」「查询用户所属的工作序列」；
//   - gender ←「获取用户性别」。
//
// 未列入的只有两类：① 嵌套集合（orders / custom_attrs —— 映射规则一条只能取单值）；
// ② 平台不返回的字段（department_path 需 user_access_token；assign_info 席位本接口不返回）。
var feishuUserSourceFields = []feishuUserSourceField{
	{Key: "user_id", Value: func(u feishuUser) string { return u.UserID }},
	{Key: "open_id", Value: func(u feishuUser) string { return u.OpenID }},
	{Key: "union_id", Value: func(u feishuUser) string { return u.UnionID }},
	{Key: "name", Value: func(u feishuUser) string { return u.Name }},
	{Key: "en_name", Value: func(u feishuUser) string { return u.EnName }},
	{Key: "nickname", Value: func(u feishuUser) string { return u.Nickname }},
	{Key: "email", Value: func(u feishuUser) string { return u.Email }},
	{Key: "enterprise_email", Value: func(u feishuUser) string { return u.EnterpriseEmail }},
	{Key: "mobile", Value: func(u feishuUser) string { return u.Mobile }},
	{Key: "mobile_visible", Value: func(u feishuUser) string { return boolValue(u.MobileVisible) }},
	{Key: "gender", Value: func(u feishuUser) string { return intValue(int64(u.Gender)) }},
	{Key: "avatar", Value: func(u feishuUser) string { return u.Avatar.Avatar72 }},
	{Key: "department_ids", Value: func(u feishuUser) string { return strings.Join(u.DepartmentIDs, ",") }},
	{Key: "leader_user_id", Value: func(u feishuUser) string { return u.LeaderUserID }},
	{Key: "dotted_line_leader_user_ids", Value: func(u feishuUser) string {
		return strings.Join(u.DottedLineLeaderUserIDs, ",")
	}},
	{Key: "city", Value: func(u feishuUser) string { return u.City }},
	{Key: "country", Value: func(u feishuUser) string { return u.Country }},
	{Key: "work_station", Value: func(u feishuUser) string { return u.WorkStation }},
	{Key: "join_time", Value: func(u feishuUser) string { return intValue(u.JoinTime) }},
	{Key: "is_tenant_manager", Value: func(u feishuUser) string { return boolValue(u.IsTenantManager) }},
	{Key: "employee_no", Value: func(u feishuUser) string { return u.EmployeeNo }},
	{Key: "employee_type", Value: func(u feishuUser) string { return intValue(int64(u.EmployeeType)) }},
	{Key: "job_title", Value: func(u feishuUser) string { return u.JobTitle }},
	{Key: "job_level_id", Value: func(u feishuUser) string { return u.JobLevelID }},
	{Key: "job_family_id", Value: func(u feishuUser) string { return u.JobFamilyID }},
	{Key: "description", Value: func(u feishuUser) string { return u.Description }},
	{Key: "is_frozen", Value: func(u feishuUser) string { return boolValue(u.Status.IsFrozen) }},
	{Key: "is_resigned", Value: func(u feishuUser) string { return boolValue(u.Status.IsResigned) }},
	{Key: "is_activated", Value: func(u feishuUser) string { return boolValue(u.Status.IsActivated) }},
	{Key: "is_exited", Value: func(u feishuUser) string { return boolValue(u.Status.IsExited) }},
}

// boolValue 布尔字段的映射取值：false 记为空串。
// 理由：飞书对"未开通字段权限"的布尔字段同样回 false，若原样下发 "false"，
// 映射到本地字段就是"看起来同步了一个否定值"，比留空更容易误导。
func boolValue(b bool) string {
	if !b {
		return ""
	}
	return strconv.FormatBool(b)
}

// intValue 数值字段的映射取值：0 记为空串（同上：0 通常意味着"没返回"，而不是"值为 0"）。
func intValue(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

// FeishuUserSourceFieldKeys 返回可映射源字段键全集。
// 目录声明的候选与本清单同源（catalog 侧测试锁定），因此两者不可能漂移。
func FeishuUserSourceFieldKeys() []string {
	keys := make([]string, 0, len(feishuUserSourceFields))
	for _, field := range feishuUserSourceFields {
		keys = append(keys, field.Key)
	}
	return keys
}

// feishuUserRaw 把一条用户记录展开为映射取值的 raw 集合：**每个候选键都存在**，
// 飞书没返回（缺字段权限）时为空串——界面可选、值可能为空，正是"要用就去飞书补权限"的语义。
func feishuUserRaw(u feishuUser) map[string]string {
	raw := make(map[string]string, len(feishuUserSourceFields)+len(feishuUserLegacyAliases))
	for _, field := range feishuUserSourceFields {
		raw[field.Key] = field.Value(u)
	}
	// 历史键名别名：只参与取值，不再作为界面候选（见 feishuUserLegacyAliases）
	for legacy, canonical := range feishuUserLegacyAliases {
		raw[legacy] = raw[canonical]
	}
	return raw
}

// feishuUserLegacyAliases 历史键名 → 官方字段名。
//
// 这些键曾是我们自己起的名字（job_number/title/location），现已统一为官方字段名。
// 已保存的映射规则里可能还写着旧键名（如 title/location），若直接删键，那几行会**静默失效**
// （取值落空 → 回退内置默认），因此保留别名的取值能力：目录不再列出它们，旧配置继续生效。
// 取向与 mapping.go 的"规则串向后兼容历史数据"一致。
var feishuUserLegacyAliases = map[string]string{
	"job_number": "employee_no",
	"title":      "job_title",
	"location":   "city",
}

// feishuUserIdentity 去重与命名用的稳定标识（一个成员可属于多个部门，按部门枚举会重复出现）
func feishuUserIdentity(u feishuUser) string {
	return firstNonEmpty(u.OpenID, u.UserID, u.UnionID)
}

// ── 部门对象 ────────────────────────────────────────────

// feishuDepartment 部门对象（本连接器用到的字段）
type feishuDepartment struct {
	OpenDepartmentID   string `json:"open_department_id"`
	ParentDepartmentID string `json:"parent_department_id"`
	Name               string `json:"name"`
}

// feishuDepartmentPage 部门列表的一页
type feishuDepartmentPage struct {
	HasMore   bool               `json:"has_more"`
	PageToken string             `json:"page_token"`
	Items     []feishuDepartment `json:"items"`
}

// fetchDepartments 取应用可见的部门清单，并返回"是否从根部门递归"这一请求形态。
//
// 飞书对"从根部门递归"与"按应用通讯录范围"是**两套权限校验**（官方文档）：
//   - fromRoot=true  → 带 parent_department_id=0（根部门），会校验应用通讯录权限是否为「全员」；
//     非全员 → 40004 no dept authority error；
//   - fromRoot=false → 不带该参数，飞书按**应用自身的通讯录范围**返回部门及其子部门。
//
// 因此先按全树取、遇 40004 再退回按范围取：两种权限模型都能同步出正确的组织结构。
// 反例（历史缺陷）：只带 root 会让受限应用整体失败；只不带参数则"全员"应用只拿到根部门一条，
// 且在权限范围内没有部门时飞书返回 200 + code 0 + 空列表——参数写错就是"静默丢光部门、还显示成功"。
func (c *FeishuConnector) fetchDepartments(ctx context.Context, headers map[string]string) ([]feishuDepartment, bool, error) {
	fromRoot, scoped := true, false
	var out []feishuDepartment
	pageToken := ""
	for {
		page, code, err := c.fetchDepartmentPage(ctx, headers, fromRoot, pageToken)
		if err != nil {
			// 仅允许在"一条都还没拿到"的起点降级：翻页中途出错如实上报，不重新开始
			if fromRoot && len(out) == 0 && code == feishuNoDeptAuthCode {
				fromRoot, scoped, pageToken = false, true, ""
				continue
			}
			return nil, false, err
		}
		out = append(out, page.Items...)
		if !page.HasMore || page.PageToken == "" {
			break
		}
		pageToken = page.PageToken
	}
	// 降级后一条部门都没有：应用被限定在"只有成员、没有部门"的通讯录范围里。
	// 静默返回空列表会让管理员以为"同步成功、只是没有组织"（2026-10-16 实测踩到），因此必须报错。
	if scoped && len(out) == 0 {
		return nil, false, errNoDeptInScope
	}
	return out, fromRoot, nil
}

// fetchDepartmentPage 取一页部门。返回值 code 是飞书业务码（非 0 时同时返回 error），
// 供调用方决定是否降级重试。
func (c *FeishuConnector) fetchDepartmentPage(
	ctx context.Context, headers map[string]string, fromRoot bool, pageToken string,
) (*feishuDepartmentPage, int, error) {
	u := feishuBase + feishuPathPrefix + "/contact/v3/departments?page_size=" + strconv.Itoa(feishuPageSize) +
		"&department_id_type=open_department_id&fetch_child=true"
	// 请求形态必须跨分页保持一致：飞书按"是否带 parent_department_id"决定走哪套权限校验与返回
	if fromRoot {
		u += "&parent_department_id=" + feishuRootDepartmentID
	}
	if pageToken != "" {
		u += "&page_token=" + url.QueryEscape(pageToken)
	}
	// 飞书把业务错误码放在响应体里（无部门权限时是 HTTP 403 + body code=40004），
	// 所以这里用 RequestJSONRaw 自行解析，避免把可诊断的业务码压成一句"外部数据源返回 403"。
	status, data, err := RequestJSONRaw(ctx, http.MethodGet, u, headers, nil)
	if err != nil {
		return nil, 0, err
	}
	var resp struct {
		Code int                  `json:"code"`
		Msg  string               `json:"msg"`
		Data feishuDepartmentPage `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, 0, fmt.Errorf("飞书拉取部门失败: 外部数据源返回 %d: %s", status, truncate(string(data), 300))
	}
	if resp.Code != 0 {
		return nil, resp.Code, fmt.Errorf("飞书拉取部门失败: %d %s", resp.Code, resp.Msg)
	}
	return &resp.Data, 0, nil
}

// feishuScanDepartmentIDs 返回"要枚举直属成员的部门 ID 清单"。
// 根部门只在能从根递归时加入：受限范围的应用查根部门会被拒（40004）。
func feishuScanDepartmentIDs(departments []feishuDepartment, fromRoot bool) []string {
	ids := make([]string, 0, len(departments)+1)
	seen := map[string]bool{}
	if fromRoot {
		ids = append(ids, feishuRootDepartmentID)
		seen[feishuRootDepartmentID] = true
	}
	for _, department := range departments {
		id := department.OpenDepartmentID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// FetchGroups 拉取飞书部门
func (c *FeishuConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	token, err := c.tenantToken(ctx, s)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	departments, _, err := c.fetchDepartments(ctx, headers)
	if err != nil {
		return nil, fmt.Errorf("飞书拉取部门失败: %w", err)
	}
	groups := make([]ExternalGroup, 0, len(departments))
	for _, department := range departments {
		// 飞书根部门（open_department_id = "0" 或自引用）是"企业本身"，不落库为部门（Spec Q4.5）
		groups = append(groups, ExternalGroup{
			ID: department.OpenDepartmentID, Name: department.Name, ParentID: department.ParentDepartmentID,
			IsRoot: department.OpenDepartmentID == "0" || department.ParentDepartmentID == department.OpenDepartmentID,
			Raw:    map[string]string{"open_department_id": department.OpenDepartmentID, "name": department.Name},
		})
	}
	return groups, nil
}

// ── 用户同步 ────────────────────────────────────────────

// FetchUsers 拉取飞书成员。
//
// 走官方推荐路径 **按部门取直属用户**（`GET /contact/v3/users/find_by_department`）：
// 历史接口 `GET /contact/v3/users`（获取用户列表）官方已标注"不再维护更新，不推荐使用"，
// 而本接口与组织同步同源（先有部门、再取该部门下的直属成员），逐部门分页后按成员去重。
//
// 能枚举到谁由应用的通讯录权限范围决定（官方注意事项）：
//   - 请求部门 0（根部门）会校验应用是否具备「全员」通讯录权限；
//   - 请求非 0 部门则校验该部门是否在应用通讯录权限范围内，无权限时返回 40004。
//
// 唯一例外：应用的范围里连部门都没有时（范围只勾了成员），按部门枚举无从下手，
// 此时退回历史列表接口兜底，保证用户同步不中断；应用补齐部门权限/范围后自动回到官方路径。
func (c *FeishuConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	token, err := c.tenantToken(ctx, s)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Authorization": "Bearer " + token}

	departments, fromRoot, err := c.fetchDepartments(ctx, headers)
	if err != nil {
		if !errors.Is(err, errNoDeptInScope) {
			return nil, err
		}
		logger.Warn("飞书应用的通讯录范围里没有部门，按部门枚举成员不可用，退回历史列表接口",
			"hint", "请在飞书开放平台把该应用的数据权限（通讯录范围）设为「全部员工」")
		return c.fetchUsersByLegacyList(ctx, headers, s.UserMapping)
	}

	// 一个成员可属于多个部门 → 按部门枚举会重复，这里按稳定标识去重，并保持首次出现的顺序
	byIdentity := map[string]feishuUser{}
	identities := make([]string, 0, len(departments))
	var authorityErr error
	for _, departmentID := range feishuScanDepartmentIDs(departments, fromRoot) {
		items, err := c.fetchDepartmentUsers(ctx, headers, departmentID)
		if err != nil {
			// 单个部门读不到不阻断其余部门（受限范围下的常见形态）；但若最终一个成员都没拿到，
			// 必须把错误抛出去——"0 个成员"会被同步保护当成"源端已无成员"，软删会把本地账号全停用。
			if authorityErr == nil {
				authorityErr = err
			}
			logger.Warn("飞书按部门拉取成员失败", "department", departmentID, "error", err)
			continue
		}
		for _, item := range items {
			identity := feishuUserIdentity(item)
			if identity == "" {
				continue
			}
			if _, ok := byIdentity[identity]; !ok {
				identities = append(identities, identity)
			}
			byIdentity[identity] = item
		}
	}
	if len(identities) == 0 && authorityErr != nil {
		return nil, authorityErr
	}

	mapping := s.UserMapping
	users := make([]ExternalUser, 0, len(identities))
	for _, identity := range identities {
		item := byIdentity[identity]
		raw := feishuUserRaw(item)
		users = append(users, ExternalUser{
			ID:            PickMapped(mapping, raw, "id", "user_id"),
			Username:      PickMapped(mapping, raw, "name", "user_id"),
			Name:          PickMapped(mapping, raw, "displayName", "name"),
			Email:         PickMapped(mapping, raw, "email", "email"),
			Phone:         PickMapped(mapping, raw, "phone", "mobile"),
			Avatar:        PickMapped(mapping, raw, "avatar", "avatar"),
			Title:         PickMapped(mapping, raw, "title", "title"),
			Location:      PickMapped(mapping, raw, "location", "location"),
			DepartmentIDs: item.DepartmentIDs,
		})
	}
	return users, nil
}

// fetchDepartmentUsers 取某部门下的**直属**成员（官方 find_by_department，逐页取）
func (c *FeishuConnector) fetchDepartmentUsers(
	ctx context.Context, headers map[string]string, departmentID string,
) ([]feishuUser, error) {
	var users []feishuUser
	pageToken := ""
	for {
		u := feishuBase + feishuPathPrefix + "/contact/v3/users/find_by_department?page_size=" + strconv.Itoa(feishuPageSize) +
			"&department_id_type=open_department_id&user_id_type=open_id" +
			"&department_id=" + url.QueryEscape(departmentID)
		if pageToken != "" {
			u += "&page_token=" + url.QueryEscape(pageToken)
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				HasMore   bool         `json:"has_more"`
				PageToken string       `json:"page_token"`
				Items     []feishuUser `json:"items"`
			} `json:"data"`
		}
		if err := getJSON(ctx, u, headers, &resp); err != nil {
			return nil, fmt.Errorf("飞书拉取部门成员失败: %w", err)
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("飞书拉取部门成员失败: %d %s", resp.Code, resp.Msg)
		}
		users = append(users, resp.Data.Items...)
		if !resp.Data.HasMore || resp.Data.PageToken == "" {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return users, nil
}

// fetchUsersByLegacyList 兜底路径：历史接口 `GET /contact/v3/users`（官方已标注不推荐使用）。
//
// 只在"应用通讯录范围里没有任何部门"时兜底：官方推荐接口必须按部门枚举成员，
// 而这类应用查任何部门都会被拒（40004），旧接口会按应用可见范围直接返回成员，能保住存量同步。
func (c *FeishuConnector) fetchUsersByLegacyList(
	ctx context.Context, headers map[string]string, mapping map[string]string,
) ([]ExternalUser, error) {
	var users []ExternalUser
	pageToken := ""
	for {
		u := feishuBase + feishuPathPrefix + "/contact/v3/users?page_size=" + strconv.Itoa(feishuPageSize) +
			"&department_id_type=open_department_id&user_id_type=open_id"
		if pageToken != "" {
			u += "&page_token=" + url.QueryEscape(pageToken)
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				HasMore   bool         `json:"has_more"`
				PageToken string       `json:"page_token"`
				Items     []feishuUser `json:"items"`
			} `json:"data"`
		}
		if err := getJSON(ctx, u, headers, &resp); err != nil {
			return nil, fmt.Errorf("飞书拉取用户失败: %w", err)
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("飞书拉取用户失败: %d %s", resp.Code, resp.Msg)
		}
		for _, item := range resp.Data.Items {
			raw := feishuUserRaw(item)
			users = append(users, ExternalUser{
				ID:            PickMapped(mapping, raw, "id", "user_id"),
				Username:      PickMapped(mapping, raw, "name", "user_id"),
				Name:          PickMapped(mapping, raw, "displayName", "name"),
				Email:         PickMapped(mapping, raw, "email", "email"),
				Phone:         PickMapped(mapping, raw, "phone", "mobile"),
				Avatar:        PickMapped(mapping, raw, "avatar", "avatar"),
				Title:         PickMapped(mapping, raw, "title", "title"),
				Location:      PickMapped(mapping, raw, "location", "location"),
				DepartmentIDs: item.DepartmentIDs,
			})
		}
		if !resp.Data.HasMore || resp.Data.PageToken == "" {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return users, nil
}
