package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"polaris/internal/repository"
)

type fakeFeishuOpts struct {
	// allStaff 应用通讯录权限范围是否为「全员」
	allStaff bool
	// scopeEmpty 受限范围内没有任何部门（实测形态：范围只勾了成员）
	scopeEmpty bool
}

// feishuUserPayload 构造一条"文档全集"的用户记录（每个字段都有值，含嵌套的 avatar/status）。
// 所有布尔字段都取 true、数值都取非 0，便于断言"每个候选键都能取到值"。
func feishuUserPayload(openID, userID, name string, departmentIDs []string) string {
	item := map[string]interface{}{
		"open_id": openID, "user_id": userID, "union_id": "on_" + openID,
		"name": name, "en_name": "San Zhang", "nickname": "Alex Zhang",
		"email": "personal@example.com", "enterprise_email": "corp@example.com",
		"mobile": "+8613519157516", "mobile_visible": true, "gender": 1,
		"avatar":         map[string]string{"avatar_72": "https://cdn/" + openID + ".png"},
		"status":         map[string]bool{"is_frozen": true, "is_resigned": true, "is_activated": true, "is_exited": true},
		"department_ids": departmentIDs, "leader_user_id": "ou_boss",
		"dotted_line_leader_user_ids": []string{"ou_plus"},
		"city":                        "杭州", "country": "CN", "work_station": "3F-08", "join_time": 1600000000,
		"is_tenant_manager": true, "employee_no": "E1001", "employee_type": 1, "job_title": "工程师",
		"job_level_id": "jl_1", "job_family_id": "jf_1", "description": "负责…",
	}
	payload, _ := json.Marshal(item)
	return string(payload)
}

// fakeFeishu 假飞书开放平台，按官方文档的两套权限校验语义回包：
//
//   - 部门列表带 parent_department_id=0（根部门）：校验应用是否为「全员」——allStaff=false 时
//     回 HTTP 403 + code 40004；不带该参数时按应用可见范围返回（范围内没有部门则回空列表）；
//   - 按部门取成员：department_id=0 会校验「全员」权限，非 0 部门校验该部门是否在范围内。
func fakeFeishu(t *testing.T, opts fakeFeishuOpts) *[]string {
	t.Helper()
	calls := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		writeUsers := func(items ...string) {
			body := `{"code":0,"msg":"success","data":{"has_more":false`
			if len(items) > 0 {
				body += `,"items":[` + strings.Join(items, ",") + `]`
			}
			_, _ = w.Write([]byte(body + `}}`))
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["app_id"] != "cli-app" || body["app_secret"] != "cli-secret" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":10003,"msg":"invalid param"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-1"}`))
		case strings.HasSuffix(r.URL.Path, "/contact/v3/users/find_by_department"):
			switch r.URL.Query().Get("department_id") {
			case "0": // 根部门：直属成员只有张三
				if !opts.allStaff {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"code":40004,"msg":"no dept authority error"}`))
					return
				}
				writeUsers(feishuUserPayload("ou_1", "u_1", "张三", []string{"0"}))
			case "od_tech": // 技术部：张三同时也在（用于验证按成员去重）+ 李四
				writeUsers(
					feishuUserPayload("ou_1", "u_1", "张三", []string{"0", "od_tech"}),
					feishuUserPayload("ou_2", "u_2", "李四", []string{"od_tech"}))
			default:
				writeUsers()
			}
		case strings.HasSuffix(r.URL.Path, "/contact/v3/users"): // 历史列表接口（兜底路径）
			writeUsers(feishuUserPayload("ou_1", "u_1", "张三", []string{"0"}))
		case strings.HasSuffix(r.URL.Path, "/contact/v3/departments"):
			fromRoot := r.URL.Query().Get("parent_department_id") != ""
			if fromRoot && !opts.allStaff {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"code":40004,"msg":"no dept authority error"}`))
				return
			}
			switch {
			case fromRoot && r.URL.Query().Get("page_token") == "": // 全树：第 1 页
				_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"has_more":true,"page_token":"p2","items":[
					{"open_department_id":"od_root","parent_department_id":"","name":"示例科技"},
					{"open_department_id":"od_tech","parent_department_id":"od_root","name":"技术部"}]}}`))
			case fromRoot: // 全树：第 2 页（飞书要求翻页仍带 parent_department_id）
				_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"has_more":false,"items":[
					{"open_department_id":"od_mkt","parent_department_id":"od_root","name":"市场部"}]}}`))
			case opts.scopeEmpty: // 受限范围且范围内没有部门 → 飞书 200 + 空列表
				_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"has_more":false}}`))
			default: // 受限范围 → 范围内的部门
				_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"has_more":false,"items":[
					{"open_department_id":"od_tech","parent_department_id":"od_root","name":"技术部"}]}}`))
			}
		default:
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		}
	}))
	t.Cleanup(srv.Close)
	prev := feishuBase
	feishuBase = srv.URL
	t.Cleanup(func() { feishuBase = prev })
	return calls
}

func feishuSyncer() *repository.Syncer {
	return &repository.Syncer{Type: "lark", AppID: "cli-app", AppSecret: "cli-secret"}
}

// calledUserScan 判断是否按指定部门枚举过成员（find_by_department + 该部门）
func calledUserScan(calls []string, departmentID string) bool {
	needle := "&department_id=" + departmentID
	for _, call := range calls {
		if strings.Contains(call, "find_by_department") && strings.Contains(call, needle) {
			return true
		}
	}
	return false
}

func calledWith(calls []string, substr string) bool {
	for _, call := range calls {
		if strings.Contains(call, substr) {
			return true
		}
	}
	return false
}

// ── 用户同步（官方推荐路径：按部门取直属用户）────────────────

// 回归：用户同步必须走官方推荐的 find_by_department，逐部门枚举并按成员去重；
// 根部门（0）也要扫一遍（直属在根部门下的成员不在任何子部门里）。
func TestFeishuFetchUsersByDepartmentWithDedupe(t *testing.T) {
	calls := fakeFeishu(t, fakeFeishuOpts{allStaff: true})
	users, err := (&FeishuConnector{}).FetchUsers(context.Background(), feishuSyncer())
	if err != nil {
		t.Fatalf("拉取用户失败: %v", err)
	}
	if len(users) != 2 || users[0].Username != "u_1" || users[1].Username != "u_2" {
		t.Fatalf("应按成员去重后得到 2 人（张三在根部门与技术部各出现一次），实际 %+v", users)
	}
	if !calledWith(*calls, "find_by_department") {
		t.Fatalf("用户同步应走官方推荐接口 find_by_department，实际调用 %v", *calls)
	}
	if !calledUserScan(*calls, "0") || !calledUserScan(*calls, "od_tech") {
		t.Fatalf("应逐部门枚举（含根部门 0），实际调用 %v", *calls)
	}
	if calledWith(*calls, "/contact/v3/users?") {
		t.Fatalf("全员权限下不应回退历史列表接口，实际调用 %v", *calls)
	}
}

// 回归：应用通讯录范围不是「全员」时，根部门不可读——应只枚举范围内的部门，而不是整体失败。
func TestFeishuFetchUsersFallsBackToScopedDepartments(t *testing.T) {
	calls := fakeFeishu(t, fakeFeishuOpts{})
	users, err := (&FeishuConnector{}).FetchUsers(context.Background(), feishuSyncer())
	if err != nil {
		t.Fatalf("受限范围应降级成功，实际报错: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("应取到范围内的 2 个成员，实际 %d", len(users))
	}
	if calledUserScan(*calls, "0") {
		t.Fatalf("受限范围不得请求根部门（会被拒），实际调用 %v", *calls)
	}
	if !calledUserScan(*calls, "od_tech") {
		t.Fatalf("应枚举范围内部门，实际调用 %v", *calls)
	}
}

// 回归：应用范围里连部门都没有时（范围只勾了成员），按部门枚举无从下手，
// 退回历史列表接口兜底，保证用户同步不中断（否则会同步出"0 个成员"，软删会把本地账号全停用）。
func TestFeishuFetchUsersFallsBackToLegacyListWhenNoDepartmentInScope(t *testing.T) {
	calls := fakeFeishu(t, fakeFeishuOpts{scopeEmpty: true})
	users, err := (&FeishuConnector{}).FetchUsers(context.Background(), feishuSyncer())
	if err != nil {
		t.Fatalf("应兜底成功，实际报错: %v", err)
	}
	if len(users) != 1 || users[0].Username != "u_1" {
		t.Fatalf("兜底应取到 1 个成员，实际 %+v", users)
	}
	if !calledWith(*calls, "/contact/v3/users?") {
		t.Fatalf("应回退历史列表接口，实际调用 %v", *calls)
	}
}

// 回归：目录声明的候选字段必须在 raw 集合里都有键（飞书没返回时为空值），
// 且完整记录上每个候选都能取到值——候选与解析不允许脱节。
func TestFeishuUserRawCoversDocumentedSourceFields(t *testing.T) {
	var full feishuUser
	if err := json.Unmarshal([]byte(feishuUserPayload("ou_1", "u_1", "张三", []string{"od_tech"})), &full); err != nil {
		t.Fatalf("解析完整记录失败: %v", err)
	}
	raw := feishuUserRaw(full)
	for _, key := range FeishuUserSourceFieldKeys() {
		if raw[key] == "" {
			t.Fatalf("源字段 %s 在完整记录上取不到值（候选与解析脱节）", key)
		}
	}
	empty := feishuUserRaw(feishuUser{})
	if len(empty) < len(FeishuUserSourceFieldKeys()) {
		t.Fatalf("空记录也应保留全部候选键（缺字段权限时留空即可），实际 %d 个", len(empty))
	}
	// 允许存在的额外键只有历史别名，且必须与目录候选一一对应
	declared := map[string]bool{}
	for _, key := range FeishuUserSourceFieldKeys() {
		declared[key] = true
	}
	for key, value := range empty {
		if declared[key] {
			if value != "" {
				t.Fatalf("空记录的 %s 应为空串，实际 %q", key, value)
			}
			continue
		}
		if canonical, ok := feishuUserLegacyAliases[key]; !ok || !declared[canonical] {
			t.Fatalf("raw 里出现了未声明的键 %s（候选与连接器脱节）", key)
		}
	}
}

// 回归：改键名为官方字段名后，**已保存的旧映射规则必须继续生效**（title/location/job_number），
// 否则用户升级后会发现"明明配过的映射变成空值"。
func TestFeishuUserRawKeepsLegacyAliases(t *testing.T) {
	var item feishuUser
	if err := json.Unmarshal([]byte(feishuUserPayload("ou_1", "u_1", "张三", []string{"od_tech"})), &item); err != nil {
		t.Fatalf("解析记录失败: %v", err)
	}
	raw := feishuUserRaw(item)
	for legacy, canonical := range feishuUserLegacyAliases {
		if raw[legacy] == "" || raw[legacy] != raw[canonical] {
			t.Fatalf("历史键名 %s 应继续取到 %s 的值，实际 %q", legacy, canonical, raw[legacy])
		}
	}
}

// 回归：把关键字段映射到本地字段时，取值必须真的生效（企业邮箱/union_id/工位）。
func TestFeishuFetchUsersSupportsDeclaredSourceFields(t *testing.T) {
	fakeFeishu(t, fakeFeishuOpts{allStaff: true})
	sy := feishuSyncer()
	sy.AccountMapping = map[string]string{
		"email":    "enterprise_email",
		"id":       "union_id",
		"location": "work_station",
		"name":     "user_id|employee_no",
	}
	users, err := (&FeishuConnector{}).FetchUsers(context.Background(), sy)
	if err != nil {
		t.Fatalf("拉取用户失败: %v", err)
	}
	u := users[0]
	if u.Email != "corp@example.com" {
		t.Fatalf("企业邮箱未生效: %q", u.Email)
	}
	if u.ID != "on_ou_1" {
		t.Fatalf("外部唯一标识应可取 union_id，实际 %q", u.ID)
	}
	if u.Location != "3F-08" {
		t.Fatalf("工位未生效: %q", u.Location)
	}
	if u.Name != "张三" || u.Username != "u_1" {
		t.Fatalf("默认映射（display_name←name / username←user_id）应保持不变: %+v", u)
	}
}

// ── 部门同步 ─────────────────────────────────────────────

// 回归（部门"静默丢光"）：应用为「全员」通讯录权限时，必须带 parent_department_id=0 递归取全树。
// 缺这个参数时飞书返回 200 + code 0 + 空列表——不报错、也永远拉不到部门。
func TestFeishuFetchGroupsFromRootWhenAllStaff(t *testing.T) {
	calls := fakeFeishu(t, fakeFeishuOpts{allStaff: true})
	groups, err := (&FeishuConnector{}).FetchGroups(context.Background(), feishuSyncer())
	if err != nil {
		t.Fatalf("拉取部门失败: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("应取到全树 3 个部门（两页、含根部门），实际 %d: %+v", len(groups), groups)
	}
	if groups[1].ParentID != "od_root" || groups[1].Name != "技术部" {
		t.Fatalf("部门层级未还原: %+v", groups[1])
	}
	if groups[2].ID != "od_mkt" {
		t.Fatalf("第 2 页部门未取到: %+v", groups[2])
	}
	// 每一页都必须带 parent_department_id=0：翻页时丢掉参数会切到另一套权限语义
	for _, call := range (*calls)[1:] {
		if !strings.Contains(call, "/contact/v3/departments") {
			continue
		}
		if !strings.Contains(call, "parent_department_id=0") || !strings.Contains(call, "fetch_child=true") {
			t.Fatalf("部门列表每页都必须带 parent_department_id=0 且递归，实际请求 %s", call)
		}
	}
}

// 回归：应用通讯录权限**不是全员**时（企业常把应用范围限定到个别部门），
// 从根部门递归会被飞书拒绝（40004）——必须退回"按应用可见范围"再取，而不是整体失败。
func TestFeishuFetchGroupsFallsBackToAppScopeWhenNotAllStaff(t *testing.T) {
	fakeFeishu(t, fakeFeishuOpts{})
	groups, err := (&FeishuConnector{}).FetchGroups(context.Background(), feishuSyncer())
	if err != nil {
		t.Fatalf("受限范围应降级成功，实际报错: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "od_tech" {
		t.Fatalf("应取到应用可见范围内的部门, 实际 %+v", groups)
	}
}

// 回归：应用被限定在"只有成员、没有部门"的通讯录范围时，必须以**可执行的报错**收场，
// 而不是 200 + 空列表（这就是"同步成功但一个部门都没有"的静默失败）。
func TestFeishuFetchGroupsReportsEmptyScope(t *testing.T) {
	fakeFeishu(t, fakeFeishuOpts{scopeEmpty: true})
	groups, err := (&FeishuConnector{}).FetchGroups(context.Background(), feishuSyncer())
	if err == nil {
		t.Fatalf("范围内没有部门时应报错并给出处置动作，实际返回 %d 个部门", len(groups))
	}
	for _, want := range []string{"全部员工", "获取部门基础信息", "飞书拉取部门失败"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("报错应包含 %q（管理员要能照着改），实际: %v", want, err)
		}
	}
}

// 回归：部门接口连续失败时必须如实报错（不得死循环重试），且保留飞书业务码。
func TestFeishuFetchGroupsReportsBusinessCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/auth/v3/tenant_access_token/internal") {
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-1"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":40004,"msg":"no dept authority error"}`))
	}))
	t.Cleanup(srv.Close)
	prev := feishuBase
	feishuBase = srv.URL
	t.Cleanup(func() { feishuBase = prev })

	_, err := (&FeishuConnector{}).FetchGroups(context.Background(), feishuSyncer())
	if err == nil {
		t.Fatal("两次都无部门权限时应报错，而不是静默返回空列表")
	}
	if !strings.Contains(err.Error(), "40004") || !strings.Contains(err.Error(), "飞书拉取部门失败") {
		t.Fatalf("错误信息应带飞书业务码与中文上下文，实际 %v", err)
	}
}
