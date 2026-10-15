package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"polaris/internal/auth"
	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// 回归：用户响应绝不能包含密码哈希 / TOTP 密钥 / 恢复码（曾出现 /me 直接返回实体导致泄漏）。
func TestUserDTONeverExposesSecrets(t *testing.T) {
	u := &repository.User{
		ID: 1, InstitutionID: 1, Name: "admin",
		PasswordHash:     "$2a$10$verysecret",
		MfaSecret:        "ENCRYPTED-TOTP-SECRET",
		MfaRecoveryCodes: []string{"abcdef1234"},
	}
	b, err := json.Marshal(toUserDTO(u))
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	body := string(b)
	for _, forbidden := range []string{"verysecret", "ENCRYPTED-TOTP-SECRET", "abcdef1234", "password_hash", "mfa_secret", "mfa_recovery_codes"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("用户响应泄漏敏感字段 %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"has_password":true`) {
		t.Fatalf("应以 has_password 表达是否设置密码: %s", body)
	}
	if !strings.Contains(body, `"mfa_enabled"`) {
		t.Fatalf("应包含 mfa_enabled: %s", body)
	}
}

// 回归：组织响应绝不能包含密码盐明文（以 has_password_salt 表达是否已配置）。
func TestInstitutionDTONeverExposesPasswordSalt(t *testing.T) {
	o := &repository.Institution{ID: 1, Name: "built-in", PasswordSalt: "super-secret-pepper"}
	b, _ := json.Marshal(toInstitutionDTO(o))
	body := string(b)
	if strings.Contains(body, "super-secret-pepper") || strings.Contains(body, `"password_salt"`) {
		t.Fatalf("组织响应泄漏密码盐: %s", body)
	}
	if !strings.Contains(body, `"has_password_salt":true`) {
		t.Fatalf("应以 has_password_salt 表达是否配置密码盐: %s", body)
	}
}

// 回归：提供方与证书响应绝不能包含 client_secret / 私钥。
func TestProviderAndCertDTONeverExposeSecrets(t *testing.T) {
	p := &repository.Provider{
		ID: 1, Name: "dingtalk", Category: "oauth", Type: "dingtalk",
		ClientID: "ding-abc", ClientSecret: "super-secret-value", HasSecret: true,
	}
	pb, _ := json.Marshal(toProviderDTO(p))
	if strings.Contains(string(pb), "super-secret-value") {
		t.Fatalf("提供方响应泄漏密钥: %s", string(pb))
	}
	if !strings.Contains(string(pb), `"has_secret":true`) {
		t.Fatalf("应以 has_secret 表达密钥是否配置: %s", string(pb))
	}
	// 目录声明的非敏感字段必须按类型下发（前端向导据此回填）
	if !strings.Contains(string(pb), `"config":{"client_id":"ding-abc"`) {
		t.Fatalf("config 应包含该类型的非敏感字段: %s", string(pb))
	}

	c := &repository.Cert{ID: 1, Name: "cert-built-in", PrivateKey: "-----BEGIN RSA PRIVATE KEY-----"}
	cb, _ := json.Marshal(toCertDTO(c))
	if strings.Contains(string(cb), "BEGIN RSA PRIVATE KEY") || strings.Contains(string(cb), `"private_key":`) {
		t.Fatalf("证书响应泄漏私钥: %s", string(cb))
	}
	if !strings.Contains(string(cb), `"has_private_key":true`) {
		t.Fatalf("应以 has_private_key 表达私钥是否存在: %s", string(cb))
	}
}

// 回归：令牌响应不得下发令牌明文。
func TestTokenDTONeverExposesTokenValue(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	tk := &repository.Token{
		ID: 1, Name: "api-key-1", AccessToken: "plain-access-token",
		RefreshToken: "plain-refresh-token", IDToken: "plain-id-token", ExpireTime: &exp,
	}
	b, _ := json.Marshal(toTokenDTO(tk))
	body := string(b)
	for _, forbidden := range []string{
		"plain-access-token", "plain-refresh-token", "plain-id-token",
		`"access_token":`, `"refresh_token":`, `"id_token":`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("令牌响应泄漏令牌内容 %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"has_access_token":true`) {
		t.Fatalf("应以 has_access_token 表达存在性: %s", body)
	}
}

func TestSelfServiceResponsesHideAdminAndAuditDetails(t *testing.T) {
	session := &auth.Session{
		ID: "session-1", Name: "alice", InstitutionID: 1,
		Roles: []string{"admin"}, Permissions: []auth.PermissionRule{{Resource: "*", Action: "*"}},
		UserAgent: "browser",
	}
	sessionJSON, err := json.Marshal(toSelfSessionDTO(session, session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sessionJSON), "permissions") || strings.Contains(string(sessionJSON), "roles") ||
		!strings.Contains(string(sessionJSON), `"is_current":true`) {
		t.Fatalf("个人会话 DTO 应只暴露会话展示字段: %s", sessionJSON)
	}

	recordJSON, err := json.Marshal(personalRecordDTO{
		ID: 1, Action: "auth.login", Object: "alice", StatusCode: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"response", "detail", "trace_id", "password"} {
		if strings.Contains(string(recordJSON), forbidden) {
			t.Fatalf("个人审计 DTO 不应暴露管理审计字段 %q: %s", forbidden, recordJSON)
		}
	}
}

// 回归：同步器响应不得下发口令明文。
func TestSyncerDTONeverExposesSecrets(t *testing.T) {
	s := &repository.Syncer{ID: 1, Name: "ldap-1", Password: "enc:ldap-password", AppSecret: "enc:app-secret"}
	b, _ := json.Marshal(toSyncerDTO(s))
	body := string(b)
	if strings.Contains(body, "ldap-password") || strings.Contains(body, "app-secret") {
		t.Fatalf("同步器响应泄漏口令: %s", body)
	}
	if !strings.Contains(body, `"has_secret":true`) || !strings.Contains(body, `"has_app_secret":true`) {
		t.Fatalf("应以 has_secret/has_app_secret 表达存在性: %s", body)
	}
}

// DTO 应把 nil 集合规范化为空集合，避免前端拿到 null。
func TestApplicationDTOEmptyCollections(t *testing.T) {
	a := &repository.Application{ID: 1, Code: "app", Name: "App", ClientSecret: "keep-me"}
	b, _ := json.Marshal(toApplicationDTO(a))
	body := string(b)
	for _, want := range []string{`"grant_types":[]`, `"redirect_uris":[]`, `"providers":[]`, `"scopes":[]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("期望包含 %s，实际 %s", want, body)
		}
	}
}

// 回归：应用列表/详情响应绝不下发客户端密钥明文（仅创建与重置密钥时一次性返回）。
func TestApplicationDTONeverExposesClientSecret(t *testing.T) {
	a := &repository.Application{ID: 1, Code: "portal", Name: "门户", ClientID: "cid-1", ClientSecret: "super-secret-value"}
	b, _ := json.Marshal(toApplicationDTO(a))
	body := string(b)
	if strings.Contains(body, "super-secret-value") || strings.Contains(body, `"client_secret":`) {
		t.Fatalf("应用响应泄漏客户端密钥: %s", body)
	}
	if !strings.Contains(body, `"has_client_secret":true`) {
		t.Fatalf("应以 has_client_secret 表达密钥存在性: %s", body)
	}
	if !strings.Contains(body, `"client_id":"cid-1"`) {
		t.Fatalf("client_id 应正常下发: %s", body)
	}
}

// 回归：编辑页依赖的字段必须全部回显，否则提交时会用空值覆盖（"编辑即重置"）。
func TestApplicationDTOIncludesEditableFields(t *testing.T) {
	a := &repository.Application{
		ID: 1, Code: "portal", Name: "门户",
		SignupItems:   []repository.SignupItem{{Name: "Email", Visible: true, Required: true}},
		SigninMethods: []repository.SigninMethod{{Name: "MagicLink"}},
		ThemeData:     &repository.ThemeData{ColorPrimary: "#5B53E8"},
		IPWhitelist:   "10.0.0.0/8", TokenFields: []string{"email"},
		ForcedRedirectOrigin:   "https://app.example.com",
		BackchannelLogoutURI:   "https://app.example.com/logout",
		DisableRefreshRotation: true,
		// 登录失败限制：编辑页必须回显，否则提交时会被 0 覆盖（"编辑即重置"）
		SigninErrorLimit: 3, SigninRetryWaitMinutes: 20,
	}
	b, _ := json.Marshal(toApplicationDTO(a))
	body := string(b)
	for _, want := range []string{
		`"signup_items":[`, `"signin_methods":[`, `"theme_data":{`, `"ip_whitelist":"10.0.0.0/8"`,
		`"token_fields":["email"]`, `"forced_redirect_origin":"https://app.example.com"`,
		`"backchannel_logout_uri":"https://app.example.com/logout"`, `"disable_refresh_rotation":true`,
		`"signin_error_limit":3`, `"signin_retry_wait_minutes":20`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("编辑页字段缺失 %s，实际 %s", want, body)
		}
	}
}

// 回归：连接器响应只下发非敏感配置与 has_secrets，绝不下发明文凭据。
func TestConnectorDTONeverExposesCredentials(t *testing.T) {
	c := &repository.Connector{
		ID: 1, Name: "conn1",
		Values:     map[string]string{"provider_url": "https://a", "credentials": "enc:secret-value"},
		HasSecrets: map[string]bool{"credentials": true},
	}
	b, _ := json.Marshal(toConnectorDTO(c))
	body := string(b)
	if strings.Contains(body, "secret-value") || strings.Contains(body, "enc:secret-value") {
		t.Fatalf("连接器响应泄漏凭证: %s", body)
	}
	if !strings.Contains(body, `"has_secrets":{"credentials":true}`) {
		t.Fatalf("应以 has_secrets 表达存在性: %s", body)
	}
	if !strings.Contains(body, `"config":{"provider_url":"https://a"}`) {
		t.Fatalf("非敏感配置应下发: %s", body)
	}
	// 迁移 022 后连接器类型必须下发（前端据此选择图标与表单），且类型缺省为通用 HTTP
	if !strings.Contains(body, `"type":"http"`) {
		t.Fatalf("应下发连接器类型（缺省 http）: %s", body)
	}
}

// 回归：任务目录响应必须收敛为「界面用得到的字段」——
// 不下发定义表的 payload（实现细节），且 nil 集合**归一化为空集合**（前端不必判空）。
func TestAsyncdJobDTOShape(t *testing.T) {
	definition := &repository.AsyncdJob{
		ID: 7, Name: "syncer.schedule", TaskType: "syncer.schedule",
		Kind: repository.AsyncdJobKindScheduled, ScheduleKind: repository.ScheduleKindDaily,
		DailyTimes: nil, WindowMinutes: 30, Enabled: true,
		LastStatus: repository.AsyncdJobStatusSkipped,
	}
	body, err := json.Marshal(toAsyncdJobDTO(asyncd.JobCatalog{Definition: definition}))
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		`"name":"syncer.schedule"`, `"kind":"scheduled"`, `"daily_times":[]`,
		`"last_status":"skipped"`, `"by_status":{}`, `"id":7`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("任务目录响应缺少 %s: %s", want, got)
		}
	}
	if strings.Contains(got, "payload") {
		t.Fatalf("任务目录响应不应下发定义表的 payload: %s", got)
	}

	// 异步任务：没有调度段（前端据此不渲染启停开关）
	async := &repository.AsyncdJob{ID: 8, Name: "syncer.run", TaskType: "syncer.run", Kind: repository.AsyncdJobKindAsync}
	b, _ := json.Marshal(toAsyncdJobDTO(asyncd.JobCatalog{Definition: async, Stats: asyncd.JobStats{
		Total: 2, ByStatus: map[string]int{repository.AsyncdJobRunSucceeded: 2},
	}}))
	if strings.Contains(string(b), `"schedule"`) {
		t.Fatalf("异步任务不应带调度段: %s", string(b))
	}
	if !strings.Contains(string(b), `"total":2`) {
		t.Fatalf("异步任务应带执行统计: %s", string(b))
	}
}

// 回归：执行器实例的在线状态是**算出**的（心跳新鲜度 / 显式停止），不是库里的列。
func TestAsyncdWorkerDTOState(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		stopped *time.Time
		want    string
	}{
		{name: "在线", want: asyncd.WorkerStateOnline},
		{name: "已停止", stopped: &now, want: asyncd.WorkerStateStopped},
	} {
		worker := asyncd.WorkerInfo{ID: "host-1#0", State: tc.want, Running: 1}
		body, _ := json.Marshal(&asyncdWorkerDTO{ID: worker.ID, State: worker.State, Running: worker.Running})
		if !strings.Contains(string(body), `"state":"`+tc.want+`"`) {
			t.Fatalf("%s：状态未下发: %s", tc.name, string(body))
		}
	}
}
