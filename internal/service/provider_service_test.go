package service

import (
	"context"
	"strings"
	"testing"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// ── 测试替身：只实现本用例用到的仓库方法（其余 panic，避免"悄悄走到未覆盖路径"）──

type fakeProviderRepo struct {
	items  map[string]*repository.Provider
	nextID int64
}

func newFakeProviderRepo() *fakeProviderRepo {
	return &fakeProviderRepo{items: map[string]*repository.Provider{}, nextID: 1}
}

func providerKey(institutionID int64, name string) string {
	return string(rune(institutionID)) + ":" + name
}

func (r *fakeProviderRepo) Create(_ context.Context, p *repository.Provider) (int64, error) {
	p.ID = r.nextID
	r.nextID++
	clone := *p
	r.items[providerKey(p.InstitutionID, p.Name)] = &clone
	return p.ID, nil
}

func (r *fakeProviderRepo) GetByID(_ context.Context, id int64) (*repository.Provider, error) {
	for _, item := range r.items {
		if item.ID == id {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errTestNotFound
}

func (r *fakeProviderRepo) GetByName(_ context.Context, institutionID int64, name string) (*repository.Provider, error) {
	item, ok := r.items[providerKey(institutionID, name)]
	if !ok {
		return nil, errTestNotFound
	}
	clone := *item
	return &clone, nil
}

func (r *fakeProviderRepo) Update(_ context.Context, p *repository.Provider) error {
	if _, ok := r.items[providerKey(p.InstitutionID, p.Name)]; !ok {
		return errTestNotFound
	}
	clone := *p
	r.items[providerKey(p.InstitutionID, p.Name)] = &clone
	return nil
}

func (r *fakeProviderRepo) Delete(_ context.Context, id int64) error {
	for key, item := range r.items {
		if item.ID == id {
			delete(r.items, key)
			return nil
		}
	}
	return errTestNotFound
}

func (r *fakeProviderRepo) List(_ context.Context, q repository.ListQuery) ([]*repository.Provider, error) {
	out := make([]*repository.Provider, 0, len(r.items))
	for _, item := range r.items {
		if q.InstitutionID != 0 && item.InstitutionID != q.InstitutionID {
			continue
		}
		if q.Field != "" && q.Value != "" {
			switch q.Field {
			case "category":
				if item.Category != q.Value {
					continue
				}
			}
		}
		clone := *item
		out = append(out, &clone)
	}
	return out, nil
}

func (r *fakeProviderRepo) Count(_ context.Context, q repository.ListQuery) (int64, error) {
	items, _ := r.List(context.Background(), q)
	return int64(len(items)), nil
}

type fakeOrgRepo struct{}

func (fakeOrgRepo) Create(context.Context, *repository.Institution) (int64, error) { return 0, nil }
func (fakeOrgRepo) GetByID(_ context.Context, id int64) (*repository.Institution, error) {
	if id == 1 {
		return &repository.Institution{ID: 1, Name: "built-in"}, nil
	}
	return nil, errTestNotFound
}
func (fakeOrgRepo) GetByName(_ context.Context, name string) (*repository.Institution, error) {
	if name == "built-in" {
		return &repository.Institution{ID: 1, Name: "built-in"}, nil
	}
	return nil, errTestNotFound
}
func (fakeOrgRepo) GetByDomain(context.Context, string) (*repository.Institution, error) {
	return nil, errTestNotFound
}
func (fakeOrgRepo) Update(context.Context, *repository.Institution) error { return nil }
func (fakeOrgRepo) Delete(context.Context, int64) error                   { return nil }
func (fakeOrgRepo) List(context.Context, repository.ListQuery) ([]*repository.Institution, error) {
	return nil, nil
}
func (fakeOrgRepo) Count(context.Context, repository.ListQuery) (int64, error) { return 0, nil }

type testNotFound struct{}

func (testNotFound) Error() string { return "not found" }

var errTestNotFound = testNotFound{}

func newTestProviderService(t *testing.T) (*ProviderService, *fakeProviderRepo) {
	t.Helper()
	repo := newFakeProviderRepo()
	cipher, err := auth.NewSecretCipher(auth.DeriveKey("test-credential-enc-key"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := providerpkg.NewRuntime(repo, cipher)
	return NewProviderService(repo, fakeOrgRepo{}, runtime, nil, nil, nil, NewAuditRecorder(nil)), repo
}

func TestProviderCreateRejectsUnknownTypeAndKey(t *testing.T) {
	svc, _ := newTestProviderService(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "x", Category: "oauth", Type: "not-a-type",
	}); err == nil {
		t.Fatal("未知类型应被拒绝（杜绝「能选但没实现」）")
	}
	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "x", Category: "oauth", Type: "oidc",
		Config:  map[string]any{"issuer_url": "https://idp.example.com", "client_id": "c", "typo_key": "v"},
		Secrets: map[string]string{"client_secret": "s"},
	}); err == nil {
		t.Fatal("未知 config 键应被拒绝（杜绝「字段被静默丢弃」）")
	}
	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "x", Category: "oauth", Type: "oidc",
		Config: map[string]any{"issuer_url": "https://idp.example.com", "client_id": "c", "client_secret": "plain"},
	}); err == nil {
		t.Fatal("敏感字段出现在 config 桶应被拒绝")
	}
}

func TestProviderCreateEncryptsSecretAndNeverReturnsIt(t *testing.T) {
	svc, repo := newTestProviderService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "corp-sso", DisplayName: "企业单点登录",
		Category: "OAuth", Type: "OIDC", // 大小写应被归一化
		Config: map[string]any{
			"issuer_url": "https://idp.example.com", "client_id": "polaris",
			"scopes": []any{"openid", "email"}, "enable_pkce": true, "state": "active",
		},
		Secrets: map[string]string{"client_secret": "s3cr3t-value"},
		Policy:  map[string]any{"auto_create": true, "link_by_email": false, "allowed_domains": []any{"corp.example.com"}},
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.Category != "oauth" || created.Type != "oidc" {
		t.Fatalf("类别/类型未归一化: %s/%s", created.Category, created.Type)
	}
	// 响应实体不得带明文密钥
	if strings.Contains(created.ClientSecret, "s3cr3t-value") {
		t.Fatal("响应实体泄漏明文密钥")
	}
	if !created.HasSecret {
		t.Fatal("应以 has_secret 表达密钥已配置")
	}
	// 落库的是密文，且能被 Runtime 解回明文
	stored, _ := repo.GetByName(ctx, 1, "corp-sso")
	if stored.ClientSecret == "s3cr3t-value" || stored.ClientSecret == "" {
		t.Fatalf("密钥未加密入库: %q", stored.ClientSecret)
	}
	cipher, _ := auth.NewSecretCipher(auth.DeriveKey("test-credential-enc-key"))
	plain, decErr := cipher.Decrypt(stored.ClientSecret)
	if decErr != nil || plain != "s3cr3t-value" {
		t.Fatalf("密钥无法解密回明文: %v", decErr)
	}
	// 信任策略落库
	if !stored.AutoCreate || stored.LinkByEmail {
		t.Fatalf("信任策略未生效: auto_create=%v link_by_email=%v", stored.AutoCreate, stored.LinkByEmail)
	}
	if len(stored.AllowedDomains) != 1 {
		t.Fatalf("allowed_domains 未生效: %+v", stored.AllowedDomains)
	}
}

func TestProviderUpdateKeepsSecretWhenBlank(t *testing.T) {
	svc, repo := newTestProviderService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "corp-sso", Category: "oauth", Type: "oidc",
		Config:  map[string]any{"issuer_url": "https://idp.example.com", "client_id": "polaris"},
		Secrets: map[string]string{"client_secret": "first-secret"},
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := repo.GetByName(ctx, 1, "corp-sso")

	updated, err := svc.Update(ctx, "admin", 1, "corp-sso", ProviderInput{
		DisplayName: "改名后的名称",
		Config:      map[string]any{"client_id": "polaris-2"},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	after, _ := repo.GetByName(ctx, 1, "corp-sso")
	if after.ClientSecret != before.ClientSecret {
		t.Fatal("密钥留空时不应覆盖原值")
	}
	if after.ClientID != "polaris-2" {
		t.Fatalf("非敏感配置未更新: %s", after.ClientID)
	}
	if after.IssuerURL != "https://idp.example.com" {
		t.Fatal("未提交的字段应保持原值")
	}
	if updated.DisplayName != "改名后的名称" {
		t.Fatalf("展示名未更新: %s", updated.DisplayName)
	}

	// 显式提交新密钥时应替换
	if _, err := svc.Update(ctx, "admin", 1, "corp-sso", ProviderInput{
		Secrets: map[string]string{"client_secret": "second-secret"},
	}); err != nil {
		t.Fatal(err)
	}
	replaced, _ := repo.GetByName(ctx, 1, "corp-sso")
	cipher, _ := auth.NewSecretCipher(auth.DeriveKey("test-credential-enc-key"))
	if plain, err := cipher.Decrypt(replaced.ClientSecret); err != nil || plain != "second-secret" {
		t.Fatalf("新密钥未生效: %q %v", plain, err)
	}
}

func TestProviderListByCategoryFiltersDisabled(t *testing.T) {
	svc, _ := newTestProviderService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "mail", Category: "email", Type: "smtp", State: "active",
		Config:  map[string]any{"host": "smtp.example.com", "port": 465, "state": "active"},
		Secrets: map[string]string{"client_secret": "pw"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "admin", ProviderInput{
		InstitutionID: 1, Name: "mail-off", Category: "email", Type: "smtp", State: "disabled",
		Config:  map[string]any{"host": "smtp2.example.com", "port": 587, "state": "disabled"},
		Secrets: map[string]string{"client_secret": "pw"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListByCategory(ctx, 1, "Email")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "mail" {
		t.Fatalf("停用的提供商不应出现在可用列表: %+v", items)
	}
	// 列表结果同样不得携带密钥
	if items[0].ClientSecret != "" {
		t.Fatal("列表响应泄漏密钥")
	}
}

// 单例类别：同一组织最多一个启用项（否则"到底用哪个"取决于排序）。
func TestProviderSingletonCategoryAllowsOnlyOneActive(t *testing.T) {
	svc, _ := newTestProviderService(t)
	ctx := context.Background()
	create := func(name, state string) error {
		_, err := svc.Create(ctx, "admin", ProviderInput{
			InstitutionID: 1, Name: name, Category: "storage", Type: "local", State: state,
			Config: map[string]any{"endpoint": "/data/" + name},
		})
		return err
	}
	if err := create("store-a", "active"); err != nil {
		t.Fatalf("第一个启用项应创建成功: %v", err)
	}
	// 第二个启用项：拒绝，且提示当前生效的是谁
	err := create("store-b", "active")
	if err == nil {
		t.Fatal("单例类别不应允许第二个启用项")
	}
	bizErr, ok := err.(*bizerrors.Error)
	if !ok || bizErr.Code != bizerrors.CodeConflict {
		t.Fatalf("应返回冲突业务码，实际 %v", err)
	}
	if bizErr.Details["active_provider"] != "store-a" {
		t.Fatalf("应指出当前生效的提供商，实际 %+v", bizErr.Details)
	}
	// 不带启用的（disabled）可以继续创建
	if err := create("store-b", "disabled"); err != nil {
		t.Fatalf("停用态的提供商应可创建: %v", err)
	}

	// 多例类别（oauth）不受限：多个启用项并存是合法语义（登录页列出全部）
	for _, name := range []string{"sso-a", "sso-b"} {
		if _, err := svc.Create(ctx, "admin", ProviderInput{
			InstitutionID: 1, Name: name, Category: "oauth", Type: "oidc",
			Config:  map[string]any{"issuer_url": "https://idp.example.com", "client_id": "c"},
			Secrets: map[string]string{"client_secret": "s"},
		}); err != nil {
			t.Fatalf("oauth 类别应允许多个启用项（%s）: %v", name, err)
		}
	}
}

func TestProviderSetStateEnforcesSingletonAndKeepsOtherFields(t *testing.T) {
	svc, repo := newTestProviderService(t)
	ctx := context.Background()
	mustCreate := func(name, state string) {
		t.Helper()
		if _, err := svc.Create(ctx, "admin", ProviderInput{
			InstitutionID: 1, Name: name, DisplayName: "展示名-" + name,
			Category: "storage", Type: "local", State: state,
			Config: map[string]any{"endpoint": "/data/" + name, "path_prefix": "polaris/"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	mustCreate("store-a", "active")
	mustCreate("store-b", "disabled")

	// 切换到启用：先停用 store-a 才允许（否则冲突）
	if _, err := svc.SetState(ctx, "admin", 1, "store-b", "active"); err == nil {
		t.Fatal("已有启用项时切换到启用应被拒绝")
	}
	if _, err := svc.SetState(ctx, "admin", 1, "store-a", "disabled"); err != nil {
		t.Fatalf("停用应成功: %v", err)
	}
	if _, err := svc.SetState(ctx, "admin", 1, "store-b", "active"); err != nil {
		t.Fatalf("停用原生效项后应可切换: %v", err)
	}

	// 切状态不得动其它字段（这正是走专用端点而不是整表 PUT 的原因）
	item, _ := repo.GetByName(ctx, 1, "store-b")
	if item.DisplayName != "展示名-store-b" || item.Endpoint != "/data/store-b" || item.PathPrefix != "polaris/" {
		t.Fatalf("切换状态不应改动其它字段: %+v", item)
	}
}
