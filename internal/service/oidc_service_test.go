package service

import (
	"context"
	"testing"
	"time"

	"polaris/internal/auth/tokenhash"
	"polaris/internal/repository"
)

func TestAppAllowsRedirect(t *testing.T) {
	app := &repository.Application{
		RedirectURIs: []string{
			"http://localhost:5173/sso/callback",
			"https://app.example.com/*",
		},
	}
	cases := []struct {
		uri  string
		want bool
	}{
		{"http://localhost:5173/sso/callback", true},
		{"https://app.example.com/oauth/callback", true},
		{"https://app.example.com/anything/deep", true},
		{"http://localhost:5173/evil", false},
		{"https://evil.example.com/callback", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := appAllowsRedirect(app, tc.uri); got != tc.want {
			t.Errorf("appAllowsRedirect(%q) = %v, 期望 %v", tc.uri, got, tc.want)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "openid profile email"},
		{"profile", "openid profile"},
		{"openid email", "openid email"},
	}
	for _, tc := range cases {
		if got := normalizeScope(tc.in); got != tc.want {
			t.Errorf("normalizeScope(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

// TestShouldIssueRefresh 验证 refresh_token 仅在 scope 含 offline_access 且应用允许刷新时下发。
func TestShouldIssueRefresh(t *testing.T) {
	app := &repository.Application{RefreshExpireInHours: 24}
	noRefresh := &repository.Application{RefreshExpireInHours: 0}

	cases := []struct {
		name  string
		app   *repository.Application
		scope string
		want  bool
	}{
		{"含 offline_access 且允许刷新", app, "openid offline_access", true},
		{"无 offline_access", app, "openid profile email", false},
		{"子串不应误判", app, "openid offline_access_x", false},
		{"应用不允许刷新", noRefresh, "openid offline_access", false},
		{"nil 应用", nil, "openid offline_access", false},
	}
	for _, tc := range cases {
		if got := shouldIssueRefresh(tc.app, tc.scope); got != tc.want {
			t.Errorf("%s: shouldIssueRefresh = %v, 期望 %v", tc.name, got, tc.want)
		}
	}
}

// TestHasScope 验证按空格切分的 scope 匹配（避免子串误判）。
func TestHasScope(t *testing.T) {
	scope := normalizeScope("openid profile email")
	if !hasScope(scope, "openid") || !hasScope(scope, "profile") || !hasScope(scope, "email") {
		t.Fatal("应匹配已声明的 scope")
	}
	if hasScope(scope, "phone") || hasScope(scope, "offline_access") {
		t.Fatal("不应匹配未声明的 scope")
	}
	if hasScope("openid offline_access_x", "offline_access") {
		t.Fatal("不应把 offline_access_x 误判为 offline_access")
	}
}

func TestVerifyPKCE(t *testing.T) {
	// RFC 7636 附录 B 的示例向量
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if !verifyPKCE(challenge, "S256", verifier) {
		t.Fatal("S256 校验应通过")
	}
	if verifyPKCE(challenge, "S256", "wrong-verifier") {
		t.Fatal("错误的 code_verifier 不应通过")
	}
	if !verifyPKCE("plain-value", "plain", "plain-value") {
		t.Fatal("plain 方式应逐字比对")
	}
	if !verifyPKCE("plain-value", "", "plain-value") {
		t.Fatal("method 为空时按 plain 处理")
	}
	if verifyPKCE(challenge, "S256", "") {
		t.Fatal("空 verifier 不应通过")
	}
	if verifyPKCE(challenge, "unknown-method", verifier) {
		t.Fatal("未知 method 不应通过")
	}
}

func TestSanitizeFrom(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/users", "/users"},
		{"", ""},
		{"https://evil.com", ""},
		{"//evil.com", ""},
		{"/login", ""},
		{"/login?from=x", ""},
		{"/oauth/authorize?client_id=netbird&redirect_uri=https%3A%2F%2Fnetbird.example.com%2Fcallback&state=s", "/oauth/authorize?client_id=netbird&redirect_uri=https%3A%2F%2Fnetbird.example.com%2Fcallback&state=s"},
		{"users", ""},
	}
	for _, tc := range cases {
		if got := sanitizeFrom(tc.in); got != tc.want {
			t.Errorf("sanitizeFrom(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

type tokenUserDeptRepo struct {
	repository.UserDepartmentRepository
	ids []int64
}

func (r *tokenUserDeptRepo) DepartmentIDsForUser(context.Context, int64) ([]int64, error) {
	return r.ids, nil
}

type tokenDeptRepo struct {
	repository.DepartmentRepository
	items []*repository.Department
}

func (r *tokenDeptRepo) List(context.Context, repository.ListQuery) ([]*repository.Department, error) {
	return r.items, nil
}

type tokenRoleRepo struct {
	repository.RoleRepository
}

func (r *tokenRoleRepo) RolesForUser(context.Context, int64, string) ([]string, error) {
	return []string{"member"}, nil
}

type tokenJobTitleRepo struct {
	repository.JobTitleRepository
	titles []string
}

func (r *tokenJobTitleRepo) JobTitlesForUser(context.Context, int64, string) ([]string, error) {
	return r.titles, nil
}

// 回归：NetBird 的 JWT group sync 读 access_token 的 groups claim；
// 本系统的 groups 对应「部门」，取部门展示路径。
func TestTokenExtraGroupsComeFromDepartments(t *testing.T) {
	svc := &OIDCService{
		userDepts: &tokenUserDeptRepo{ids: []int64{2, 3}},
		depts: &tokenDeptRepo{items: []*repository.Department{
			{ID: 1, Name: "built-in", DisplayName: "Polaris", ParentIDChain: []int64{}},
			{ID: 2, Name: "fe", DisplayName: "前端组", ParentIDChain: []int64{1}},
			{ID: 3, Name: "tech", DisplayName: "技术部", ParentIDChain: []int64{1}},
		}},
		roles:  &tokenRoleRepo{},
		groups: &tokenJobTitleRepo{titles: []string{"CEO"}},
	}
	user := &repository.User{InstitutionID: 9, Name: "10002"}

	extra, err := svc.tokenExtra(context.Background(), &repository.Application{
		TokenAttributes: []repository.TokenAttribute{
			{Name: "username", Category: TokenAttributeCategoryExisting, Value: "username", Type: TokenAttributeTypeString},
			{Name: "institution", Category: TokenAttributeCategoryExisting, Value: "institution", Type: TokenAttributeTypeString},
			{Name: "groups", Category: TokenAttributeCategoryExisting, Value: "departments", Type: TokenAttributeTypeArray},
			{Name: "job_titles", Category: TokenAttributeCategoryExisting, Value: "job_titles", Type: TokenAttributeTypeArray},
		},
	}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}
	if got, ok := extra["groups"].([]string); !ok || len(got) != 2 || got[0] != "Polaris / 前端组" {
		t.Fatalf("groups = %#v, want 部门路径列表", extra["groups"])
	}
	if got, ok := extra["job_titles"].([]string); !ok || len(got) != 1 || got[0] != "CEO" {
		t.Fatalf("job_titles = %#v, want 职务列表", extra["job_titles"])
	}
	if extra["institution"] != int64(9) {
		t.Fatalf("institution = %#v, want 9", extra["institution"])
	}
	if _, exists := extra["org"]; exists {
		t.Fatal("不应再下发历史 claim org")
	}

	withoutGroups, err := svc.tokenExtra(context.Background(), &repository.Application{
		TokenAttributes: []repository.TokenAttribute{
			{Name: "username", Category: TokenAttributeCategoryExisting, Value: "username", Type: TokenAttributeTypeString},
		},
	}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}
	if _, exists := withoutGroups["groups"]; exists {
		t.Fatal("未选择 groups 字段时不应下发该 claim")
	}

	defaults, err := svc.tokenExtra(context.Background(), &repository.Application{}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}
	if defaults["institution"] != int64(9) {
		t.Fatalf("默认 institution = %#v, want 9", defaults["institution"])
	}
	if _, exists := defaults["org"]; exists {
		t.Fatal("默认字段不应再包含 org")
	}

	standard, err := svc.tokenExtra(context.Background(), &repository.Application{
		TokenFormat: TokenFormatStandard,
		TokenAttributes: []repository.TokenAttribute{
			{Name: "groups", Category: TokenAttributeCategoryExisting, Value: "departments", Type: TokenAttributeTypeArray},
		},
	}, user)
	if err != nil {
		t.Fatalf("tokenExtra: %v", err)
	}
	if _, exists := standard["groups"]; exists {
		t.Fatal("标准模式不应下发自定义 groups claim")
	}
	if len(standard) != 0 {
		t.Fatalf("标准模式只应保留注册 claim，自定义 extra 应为空，实际 %#v", standard)
	}

	idClaims, err := svc.idTokenClaims(context.Background(), &repository.Application{TokenFormat: TokenFormatCustom}, user, "openid profile")
	if err != nil {
		t.Fatalf("idTokenClaims: %v", err)
	}
	if got, ok := idClaims["groups"].([]string); !ok || len(got) != 2 || got[0] != "Polaris / 前端组" {
		t.Fatalf("id_token groups = %#v, want 部门路径列表", idClaims["groups"])
	}

	standardClaims, err := svc.idTokenClaims(context.Background(), &repository.Application{TokenFormat: TokenFormatStandard}, user, "openid profile")
	if err != nil {
		t.Fatalf("idTokenClaims: %v", err)
	}
	for _, key := range []string{"groups", "roles", "institution", "is_admin"} {
		if _, exists := standardClaims[key]; exists {
			t.Fatalf("标准模式 id_token 不应包含自定义 claim %s", key)
		}
	}

	// NetBird 连接器可能只申请 openid/email；自定义模式仍应下发 groups。
	noProfileClaims, err := svc.idTokenClaims(context.Background(), &repository.Application{
		TokenFormat: TokenFormatCustom,
	}, user, "openid email")
	if err != nil {
		t.Fatalf("idTokenClaims: %v", err)
	}
	if got, ok := noProfileClaims["groups"].([]string); !ok || len(got) != 2 {
		t.Fatalf("未申请 profile 时 groups = %#v, want 部门列表", noProfileClaims["groups"])
	}
	if got, ok := noProfileClaims["email"]; !ok || got != "" {
		t.Fatalf("无邮箱用户申请 email scope 时 id_token email = %#v, want 空字符串 claim", got)
	}

}

func TestStandardClaimsFollowScopes(t *testing.T) {
	user := &repository.User{
		Name: "zhangsan", DisplayName: "张三", FirstName: "三", LastName: "张",
		Email: "zhangsan@example.com", EmailVerified: true,
		Phone: "13800000000", PhoneVerified: true,
		Address: "世纪大道 1 号", Location: "上海", CountryCode: "CN",
		Language: "zh-CN", Homepage: "https://example.com/zhangsan",
		Gender: "male", Birthday: "1990-01-01",
		UpdatedAt: time.Unix(1700000000, 0),
	}

	claims := standardClaims(user, "openid profile email phone address")
	for _, key := range []string{
		"name", "given_name", "family_name", "preferred_username", "website",
		"gender", "birthdate", "locale", "updated_at", "email", "email_verified",
		"phone_number", "phone_number_verified", "address",
	} {
		if _, ok := claims[key]; !ok {
			t.Fatalf("缺少标准 claim %s: %#v", key, claims)
		}
	}
	for _, key := range []string{"institution", "groups", "roles", "is_admin", "departments"} {
		if _, ok := claims[key]; ok {
			t.Fatalf("标准 claims 不应包含自定义 %s", key)
		}
	}

	onlyOpenID := standardClaims(user, "openid")
	if len(onlyOpenID) != 0 {
		t.Fatalf("openid scope 不应下发 profile claim，实际 %#v", onlyOpenID)
	}
}

// 回归：NetBird 内置 Dex 在 scope 含 email 时要求 email claim 存在。
// 无邮箱用户也必须得到空字符串 claim（rc6 行为），否则会在 NetBird 回调阶段
// 报 missing email claim，页面显示 Authentication failed。
func TestStandardClaimsKeepEmptyEmailWhenScopeRequested(t *testing.T) {
	user := &repository.User{Name: "no-email-user", DisplayName: "无邮箱用户"}

	claims := standardClaims(user, "openid email")
	email, ok := claims["email"]
	if !ok {
		t.Fatal("申请 email scope 时必须下发 email claim，即使值为空")
	}
	if email != "" {
		t.Fatalf("email = %#v, want 空字符串", email)
	}
	verified, ok := claims["email_verified"]
	if !ok {
		t.Fatal("email_verified claim 缺失")
	}
	if verified != false {
		t.Fatalf("email_verified = %#v, want false", verified)
	}

	withoutEmailScope := standardClaims(user, "openid")
	if _, ok := withoutEmailScope["email"]; ok {
		t.Fatal("未申请 email scope 时不应下发 email claim")
	}
}

func TestHashTokenStable(t *testing.T) {
	a := tokenhash.Hash("token-value")
	b := tokenhash.Hash("token-value")
	if a != b {
		t.Fatal("同一令牌的哈希应稳定")
	}
	if a == tokenhash.Hash("token-value-2") {
		t.Fatal("不同令牌哈希不应相同")
	}
	if a == "token-value" {
		t.Fatal("哈希不应等于明文")
	}
}

// Regression: token records must persist hashes, never the original token
// values. The plaintext is only used in memory while building the response.
func TestTokenRecordStoresHashesOnly(t *testing.T) {
	app := &repository.Application{ID: 7, InstitutionID: 1, ClientID: "client"}
	user := &repository.User{ID: 9, InstitutionID: 1, Name: "alice", UID: "uid-alice"}
	exp := time.Now().Add(time.Hour)
	refreshExp := exp.Add(time.Hour)

	rec := tokenRecord(app, user, "token-test", "openid offline_access",
		"access-plain", "refresh-plain", "id-plain", "session-1", exp, &refreshExp, "authorization_code")

	if rec.AccessTokenHash != tokenhash.Hash("access-plain") {
		t.Fatalf("access token hash = %q", rec.AccessTokenHash)
	}
	if rec.RefreshTokenHash != tokenhash.Hash("refresh-plain") {
		t.Fatalf("refresh token hash = %q", rec.RefreshTokenHash)
	}
	if rec.IDTokenHash != tokenhash.Hash("id-plain") {
		t.Fatalf("id token hash = %q", rec.IDTokenHash)
	}
	if rec.SessionID != "session-1" {
		t.Fatalf("session id = %q, want session-1", rec.SessionID)
	}
	if rec.AccessTokenHash == "access-plain" || rec.RefreshTokenHash == "refresh-plain" || rec.IDTokenHash == "id-plain" {
		t.Fatal("token record must not contain plaintext tokens")
	}
}

type clientAuthAppRepo struct {
	repository.ApplicationRepository
	app *repository.Application
}

func (r *clientAuthAppRepo) GetByClientID(context.Context, string) (*repository.Application, error) {
	return r.app, nil
}

type clientAuthOrgRepo struct {
	repository.InstitutionRepository
}

func (clientAuthOrgRepo) GetByID(context.Context, int64) (*repository.Institution, error) {
	return &repository.Institution{ID: 1}, nil
}

func TestAuthenticateClientRequiresConfiguredSecret(t *testing.T) {
	app := &repository.Application{
		ID: 1, InstitutionID: 1, ClientID: "client-1",
		ClientSecretHash: tokenhash.Hash("correct-secret"), State: "active",
	}
	svc := &OIDCService{
		apps: &clientAuthAppRepo{app: app},
		orgs: clientAuthOrgRepo{},
	}

	if _, err := svc.authenticateClient(context.Background(), "client-1", ""); err == nil {
		t.Fatal("configured client_secret must reject an empty secret")
	}
	if _, err := svc.authenticateClient(context.Background(), "client-1", "wrong-secret"); err == nil {
		t.Fatal("configured client_secret must reject a wrong secret")
	}
	if _, err := svc.authenticateClient(context.Background(), "client-1", "correct-secret"); err != nil {
		t.Fatalf("correct client_secret rejected: %v", err)
	}

	publicApp := &repository.Application{ID: 2, InstitutionID: 1, ClientID: "public", State: "active"}
	svc.apps = &clientAuthAppRepo{app: publicApp}
	if _, err := svc.authenticateClient(context.Background(), "public", ""); err != nil {
		t.Fatalf("public client without a secret rejected: %v", err)
	}
}

func TestGrantClientCredentialsRejectsPublicClient(t *testing.T) {
	svc := &OIDCService{}
	_, err := svc.grantClientCredentials(context.Background(), &repository.Application{
		GrantTypes: []string{"client_credentials"},
	}, TokenRequest{})
	if err == nil {
		t.Fatal("client_credentials must reject a public client without a secret")
	}
}

type createAppRepo struct {
	stubAppRepo
	created *repository.Application
}

func (r *createAppRepo) Create(_ context.Context, app *repository.Application) (int64, error) {
	app.ID = 1
	r.created = app
	return 1, nil
}

func TestApplicationCreateStoresClientSecretHashAndReturnsPlainOnce(t *testing.T) {
	apps := &createAppRepo{}
	orgs := &stubOrgRepo{byName: map[string]*repository.Institution{"built-in": {ID: 1, Name: "built-in"}}}
	svc := NewApplicationService(apps, orgs, nil)

	app, err := svc.Create(context.Background(), "admin", &repository.Application{
		InstitutionID: 1,
		Code:          "test-app",
		AccessMode:    repository.AccessModeAll,
		RedirectURIs:  []string{"https://example.com/cb"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if app.ClientSecretPlain == "" {
		t.Fatal("create must return the plaintext client_secret exactly once")
	}
	if app.ClientSecretHash != tokenhash.Hash(app.ClientSecretPlain) {
		t.Fatal("stored client_secret hash does not match the returned plaintext")
	}
	if apps.created == nil || apps.created.ClientSecretHash != app.ClientSecretHash {
		t.Fatal("client_secret hash was not persisted to the application record")
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"zhangsan", "zhangsan"},
		{"zhang san", "zhang-san"},
		{"a@b.com", "a-b.com"},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeName(tc.in)
		if tc.in == "" {
			if got == "" {
				t.Errorf("空输入应回退为随机用户名，实际为空")
			}
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeName(%q) = %q, 期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestDomainAllowed(t *testing.T) {
	if !domainAllowed("a@example.com", []string{"example.com"}) {
		t.Fatal("域名应匹配")
	}
	if !domainAllowed("a@EXAMPLE.com", []string{"example.com"}) {
		t.Fatal("域名匹配应忽略大小写")
	}
	if domainAllowed("a@other.com", []string{"example.com"}) {
		t.Fatal("不匹配的域名不应通过")
	}
	if domainAllowed("no-at-sign", []string{"example.com"}) {
		t.Fatal("非法邮箱不应通过")
	}
}

func TestPage(t *testing.T) {
	cases := []struct {
		page, size            int
		wantOffset, wantLimit int
	}{
		{1, 20, 0, 20},
		{3, 10, 20, 10},
		{0, 20, 0, 20},
		{1, 0, 0, 20},
		{1, 1000, 0, 20},
	}
	for _, tc := range cases {
		offset, limit := Page(tc.page, tc.size)
		if offset != tc.wantOffset || limit != tc.wantLimit {
			t.Errorf("Page(%d,%d) = (%d,%d), 期望 (%d,%d)", tc.page, tc.size, offset, limit, tc.wantOffset, tc.wantLimit)
		}
	}
}
