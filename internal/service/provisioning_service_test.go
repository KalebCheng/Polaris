package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// 回归：建号触发策略缺省应为 onCreate。
func TestPolicyCreateWhenDefaults(t *testing.T) {
	if got := policyCreateWhen(nil); got != CreateWhenOnCreate {
		t.Fatalf("nil 策略应缺省 onCreate，实际 %q", got)
	}
	if got := policyCreateWhen(&repository.AccountStrategy{}); got != CreateWhenOnCreate {
		t.Fatalf("空 policy 应缺省 onCreate，实际 %q", got)
	}
	s := &repository.AccountStrategy{Policy: map[string]string{"createWhen": CreateWhenJIT}}
	if got := policyCreateWhen(s); got != CreateWhenJIT {
		t.Fatalf("应读取到 jit，实际 %q", got)
	}
}

// 回归：回收触发策略缺省应为 disable。
func TestPolicyRevokeWhenDefaults(t *testing.T) {
	if got := policyRevokeWhen(nil); got != RevokeWhenDisable {
		t.Fatalf("nil 策略应缺省 disable，实际 %q", got)
	}
	s := &repository.AccountStrategy{Policy: map[string]string{"revokeWhen": RevokeWhenOffboard}}
	if got := policyRevokeWhen(s); got != RevokeWhenOffboard {
		t.Fatalf("应读取到 offboard，实际 %q", got)
	}
}

// 回归：角色授予触发只针对"本次新增"的用户，避免重复派发。
func TestNewlyAdded(t *testing.T) {
	got := newlyAdded([]string{"a", "b"}, []string{"b", "c", "c", "", "d"})
	want := []string{"c", "d"}
	if len(got) != len(want) {
		t.Fatalf("新增用户集合错误: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("新增用户集合错误: %v", got)
		}
	}
}

// 回归：本地属性映射应能取到常用用户字段。
func TestLocalAttribute(t *testing.T) {
	u := &repository.User{Name: "alice", DisplayName: "Alice", Email: "a@example.com", UID: "u-1"}
	cases := map[string]string{
		"name": "alice", "username": "alice", "displayName": "Alice",
		"email": "a@example.com", "uid": "u-1",
	}
	for field, want := range cases {
		got, ok := localAttribute(u, field)
		if !ok || got != want {
			t.Fatalf("字段 %s 应映射为 %q，实际 %q (ok=%v)", field, want, got, ok)
		}
	}
	if _, ok := localAttribute(u, "unknown"); ok {
		t.Fatal("未知字段应返回 ok=false")
	}
}

// 回归：供给任务的单飞键必须与 app_accounts 的幂等键一致（机构×应用×用户），
// 否则同一账号会被重复排队、下游重复建号。
func TestProvisionUniqueKey(t *testing.T) {
	key := ProvisionUniqueKey(7, "salesforce", "alice")
	if key != "provision:7:salesforce:alice" {
		t.Fatalf("单飞键格式错误: %s", key)
	}
	if ProvisionUniqueKey(7, "salesforce", "alice") == ProvisionUniqueKey(7, "salesforce", "bob") {
		t.Fatal("不同用户的单飞键不应相同")
	}
	if ProvisionUniqueKey(7, "a", "alice") == ProvisionUniqueKey(8, "a", "alice") {
		t.Fatal("不同组织的单飞键不应相同")
	}
}

// 回归：任务载荷解析（asyncd 传入的 JSON 数字是 float64）
func TestProvisionTaskFromPayload(t *testing.T) {
	task := ProvisionTaskFromPayload(map[string]any{
		"institution_id": float64(12), "application": "salesforce", "user": "alice", "action": "create",
	})
	if task.InstitutionID != 12 || task.Application != "salesforce" || task.User != "alice" || task.Action != "create" {
		t.Fatalf("载荷解析错误: %+v", task)
	}
	empty := ProvisionTaskFromPayload(nil)
	if empty.InstitutionID != 0 || empty.Application != "" {
		t.Fatalf("空载荷应得到零值: %+v", empty)
	}
}

// 回归：下游不支持口令时（如钉钉），不得生成也不得保存下游口令。
// 否则库里会存一个"下游根本用不到"的密钥，且界面上会显示 has_password=true（假信息）。
func TestBuildAccountSkipsPasswordWhenUnsupported(t *testing.T) {
	svc := &ProvisioningService{}
	user := &repository.User{Name: "alice", DisplayName: "Alice"}

	account, plain := svc.buildAccount(user, nil, nil, false)
	if plain != "" || account.Password != "" {
		t.Fatalf("下游不支持口令时不应生成口令: plain=%q account.Password=%q", plain, account.Password)
	}

	withPassword, plainWith := svc.buildAccount(user, nil, nil, true)
	if plainWith == "" || withPassword.Password != plainWith {
		t.Fatalf("支持口令时应生成并带上明文口令: %+v", withPassword)
	}
}

// stubStrategyAppRepo 只实现策略绑定需要的两个查询，其余方法由嵌入接口兜底。
type stubStrategyAppRepo struct {
	repository.ApplicationRepository
	byID   map[int64]*repository.Application
	byCode map[string]*repository.Application
}

func (s stubStrategyAppRepo) GetByID(_ context.Context, id int64) (*repository.Application, error) {
	if app, ok := s.byID[id]; ok {
		return app, nil
	}
	return nil, errors.New("not found")
}

func (s stubStrategyAppRepo) GetByCode(_ context.Context, _ int64, code string) (*repository.Application, error) {
	if app, ok := s.byCode[code]; ok {
		return app, nil
	}
	return nil, errors.New("not found")
}

// 回归：账号策略必须把应用引用落成 application_id，而不是只存一个 application code
// （否则运行时的 FindForApplication 永远命中不到该应用的策略）。
func TestBindStrategyApplicationByCode(t *testing.T) {
	app := &repository.Application{ID: 7, InstitutionID: 1, Code: "portal"}
	svc := &ProvisioningService{apps: stubStrategyAppRepo{
		byID:   map[int64]*repository.Application{7: app},
		byCode: map[string]*repository.Application{"portal": app},
	}}
	a := &repository.AccountStrategy{InstitutionID: 1, Application: "portal"}
	if err := svc.bindStrategyApplication(context.Background(), a); err != nil {
		t.Fatalf("绑定应用失败: %v", err)
	}
	if a.ApplicationID == nil || *a.ApplicationID != 7 {
		t.Fatalf("application_id 未解析: %+v", a)
	}
}

// 回归：跨组织传 application_id 不能绑定成功。
func TestBindStrategyApplicationRejectsCrossInstitution(t *testing.T) {
	id := int64(7)
	svc := &ProvisioningService{apps: stubStrategyAppRepo{
		byID: map[int64]*repository.Application{id: {ID: id, InstitutionID: 1, Code: "portal"}},
	}}
	a := &repository.AccountStrategy{InstitutionID: 2, ApplicationID: &id}
	if err := svc.bindStrategyApplication(context.Background(), a); err == nil {
		t.Fatal("跨组织应用不应绑定成功")
	}
}

// 回归：应用/连接器过滤要进入 ListQuery.Filters，供仓库层与关键字条件做 AND。
func TestProvisioningListFilters(t *testing.T) {
	q := withConnectorFilter(withApplicationFilter(repository.ListQuery{Keyword: "alice"}, 7), "dingtalk")
	if len(q.Filters) != 2 {
		t.Fatalf("过滤条件数量 = %d, 期望 2", len(q.Filters))
	}
	if q.Filters[0].Field != "application_id" || q.Filters[0].Value != int64(7) {
		t.Fatalf("应用过滤错误: %+v", q.Filters[0])
	}
	if q.Filters[1].Field != "connector" || q.Filters[1].Value != "dingtalk" {
		t.Fatalf("连接器过滤错误: %+v", q.Filters[1])
	}
}

type fakeTaskEnqueuer struct {
	job       string
	payload   map[string]interface{}
	uniqueKey string
	calls     int
}

func (f *fakeTaskEnqueuer) Enqueue(_ context.Context, jobName string, payload any, uniqueKey string, _ time.Time, _ ...asyncd.EnqueueOption) (int64, bool, error) {
	f.calls++
	f.job = jobName
	f.uniqueKey = uniqueKey
	if m, ok := payload.(map[string]interface{}); ok {
		f.payload = m
	}
	return 1, false, nil
}

type stubJITStrategyRepo struct {
	repository.AccountStrategyRepository
	strategy *repository.AccountStrategy
}

func (r stubJITStrategyRepo) FindForApplication(context.Context, int64, int64) (*repository.AccountStrategy, error) {
	if r.strategy == nil {
		return nil, errors.New("strategy not found")
	}
	return r.strategy, nil
}

type stubProvisionUserRepo struct {
	repository.UserRepository
	user *repository.User
}

func (r stubProvisionUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	if r.user == nil {
		return nil, errors.New("user not found")
	}
	return r.user, nil
}

type stubProvisionAccountRepo struct {
	repository.ApplicationAccountRepository
	account *repository.ApplicationAccount
	deleted bool
}

func (r *stubProvisionAccountRepo) GetByUserApp(context.Context, int64, int64, int64) (*repository.ApplicationAccount, error) {
	if r.account == nil {
		return nil, errors.New("account not found")
	}
	return r.account, nil
}

func (r *stubProvisionAccountRepo) Delete(context.Context, int64) error {
	r.deleted = true
	return nil
}

type stubProvisionConnectorRepo struct {
	repository.ConnectorRepository
	items []*repository.Connector
}

func (r stubProvisionConnectorRepo) ListEnabled(_ context.Context, _ int64, applicationID int64) ([]*repository.Connector, error) {
	out := make([]*repository.Connector, 0, len(r.items))
	for _, item := range r.items {
		if item.State != "active" {
			continue
		}
		if applicationID != 0 && (item.ApplicationID == nil || *item.ApplicationID != applicationID) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func newProvisioningStubs() (*stubStrategyAppRepo, *stubJITStrategyRepo, *stubProvisionConnectorRepo, *stubProvisionUserRepo, *stubProvisionAccountRepo, *fakeTaskEnqueuer) {
	appID := int64(7)
	userID := int64(42)
	app := &repository.Application{ID: appID, InstitutionID: 1, Code: "portal", Name: "Portal", State: "active"}
	apps := &stubStrategyAppRepo{
		byID:   map[int64]*repository.Application{appID: app},
		byCode: map[string]*repository.Application{"portal": app},
	}
	strategies := &stubJITStrategyRepo{strategy: &repository.AccountStrategy{
		InstitutionID: 1, ApplicationID: &appID, Application: "portal", IsEnabled: true,
		Policy: map[string]string{"createWhen": CreateWhenJIT},
	}}
	connectors := &stubProvisionConnectorRepo{items: []*repository.Connector{{
		InstitutionID: 1, ApplicationID: &appID, Application: "portal", Name: "portal-out", State: "active",
	}}}
	users := &stubProvisionUserRepo{user: &repository.User{ID: userID, InstitutionID: 1, Name: "alice"}}
	accounts := &stubProvisionAccountRepo{account: &repository.ApplicationAccount{
		ID: 1, InstitutionID: 1, ApplicationID: &appID, UserID: &userID, Application: "portal", User: "alice",
	}}
	enqueuer := &fakeTaskEnqueuer{}
	return apps, strategies, connectors, users, accounts, enqueuer
}

// 回归：JIT 只看账号策略 createWhen=jit；连接器不再有 just_in_time 开关。
func TestTriggerJITUsesStrategy(t *testing.T) {
	apps, strategies, connectors, users, accounts, enqueuer := newProvisioningStubs()
	svc := &ProvisioningService{apps: apps, strategies: strategies, connectors: connectors, users: users, accounts: accounts, asyncd: enqueuer}
	svc.TriggerJIT(context.Background(), 1, "portal", "alice")
	if enqueuer.calls != 1 || enqueuer.job != TaskTypeProvisionAccount {
		t.Fatalf("JIT 应派发一次供给任务: %+v", enqueuer)
	}
	if enqueuer.payload["action"] != ProvisionActionCreate {
		t.Fatalf("JIT 动作应为 create: %+v", enqueuer.payload)
	}
}

func TestTriggerJITSkipsWhenStrategyIsNotJIT(t *testing.T) {
	apps, strategies, connectors, users, accounts, enqueuer := newProvisioningStubs()
	strategies.strategy.Policy = map[string]string{"createWhen": CreateWhenOnCreate}
	svc := &ProvisioningService{apps: apps, strategies: strategies, connectors: connectors, users: users, accounts: accounts, asyncd: enqueuer}
	svc.TriggerJIT(context.Background(), 1, "portal", "alice")
	if enqueuer.calls != 0 {
		t.Fatalf("非 JIT 策略不应派发任务: %+v", enqueuer)
	}
}

// 回归：「删除应用账号」必须派发下游 delete 任务，不能只删本地台账。
func TestDeleteApplicationAccountEnqueuesDownstreamDelete(t *testing.T) {
	apps, strategies, connectors, users, accounts, enqueuer := newProvisioningStubs()
	svc := &ProvisioningService{apps: apps, strategies: strategies, connectors: connectors, users: users, accounts: accounts, asyncd: enqueuer}
	if err := svc.DeleteApplicationAccount(context.Background(), "admin", 1, "alice", "portal"); err != nil {
		t.Fatalf("删除应用账号失败: %v", err)
	}
	if enqueuer.calls != 1 || enqueuer.payload["action"] != ProvisionActionDelete {
		t.Fatalf("应派发下游 delete 任务: %+v", enqueuer)
	}
	if enqueuer.uniqueKey != ProvisionUniqueKey(1, "portal", "alice") {
		t.Fatalf("单飞键错误: %s", enqueuer.uniqueKey)
	}
	if accounts.deleted {
		t.Fatal("下游任务成功前不能删除本地台账")
	}
}

func TestDeleteApplicationAccountRequiresConnector(t *testing.T) {
	apps, strategies, _, users, accounts, enqueuer := newProvisioningStubs()
	svc := &ProvisioningService{apps: apps, strategies: strategies, connectors: stubProvisionConnectorRepo{}, users: users, accounts: accounts, asyncd: enqueuer}
	if err := svc.DeleteApplicationAccount(context.Background(), "admin", 1, "alice", "portal"); err == nil {
		t.Fatal("没有启用的出方向连接器时应拒绝删除")
	}
	if enqueuer.calls != 0 {
		t.Fatalf("没有连接器时不应派发任务: %+v", enqueuer)
	}
}
