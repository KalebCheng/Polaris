package service

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

// stubSigninMethodRepo 只覆盖 resolver 用例需要的读方法。
type stubSigninMethodRepo struct {
	repository.InstitutionSigninMethodRepository
	items []*repository.InstitutionSigninMethod
	byApp map[int64][]int64
}

func (r *stubSigninMethodRepo) ListActive(context.Context, int64) ([]*repository.InstitutionSigninMethod, error) {
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range r.items {
		if item.State == "active" {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *stubSigninMethodRepo) ListByApplication(_ context.Context, _ int64, applicationID int64) ([]*repository.InstitutionSigninMethod, error) {
	allowed := map[int64]bool{}
	for _, id := range r.byApp[applicationID] {
		allowed[id] = true
	}
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range r.items {
		if allowed[item.ID] {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *stubSigninMethodRepo) ListByIDs(_ context.Context, _ int64, ids []int64) ([]*repository.InstitutionSigninMethod, error) {
	allowed := map[int64]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range r.items {
		if allowed[item.ID] {
			out = append(out, item)
		}
	}
	return out, nil
}

type stubProviderRepo struct {
	repository.ProviderRepository
	byID map[int64]*repository.Provider
	all  []*repository.Provider
}

func (r *stubProviderRepo) GetByID(_ context.Context, id int64) (*repository.Provider, error) {
	if item := r.byID[id]; item != nil {
		return item, nil
	}
	return nil, context.Canceled
}

func (r *stubProviderRepo) List(_ context.Context, q repository.ListQuery) ([]*repository.Provider, error) {
	out := []*repository.Provider{}
	for _, item := range r.all {
		if q.InstitutionID != 0 && item.InstitutionID != q.InstitutionID {
			continue
		}
		if q.Field == "category" && q.Value != "" && item.Category != q.Value {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func resolverFixture() (*SigninPolicyResolver, *repository.Application) {
	methods := &stubSigninMethodRepo{
		items: []*repository.InstitutionSigninMethod{
			{ID: 1, InstitutionID: 1, Name: repository.SigninMethodPassword, DisplayName: "Password", Identifiers: []string{"username"}, State: "active"},
			{ID: 2, InstitutionID: 1, Name: repository.SigninMethodProvider, DisplayName: "DingTalk", ProviderID: int64Ptr(7), State: "active"},
			{ID: 3, InstitutionID: 1, Name: repository.SigninMethodProvider, DisplayName: "Disabled IdP", ProviderID: int64Ptr(8), State: "active"},
			{ID: 4, InstitutionID: 1, Name: repository.SigninMethodPassword, DisplayName: "Old Password", Identifiers: []string{"username"}, State: "disabled"},
		},
		byApp: map[int64][]int64{9: {1, 2, 3}},
	}
	providers := &stubProviderRepo{
		byID: map[int64]*repository.Provider{
			7: {ID: 7, InstitutionID: 1, Name: "dingtalk", DisplayName: "钉钉", Category: "oauth", State: "active"},
			8: {ID: 8, InstitutionID: 1, Name: "disabled", DisplayName: "停用 IdP", Category: "oauth", State: "disabled"},
		},
	}
	resolver := NewSigninPolicyResolver(methods, providers, nil)
	return resolver, &repository.Application{ID: 9, InstitutionID: 1, Code: "portal"}
}

func int64Ptr(v int64) *int64 { return &v }

func applicationServiceForTest(apps repository.ApplicationRepository, orgs repository.InstitutionRepository, audits *AuditRecorder) *ApplicationService {
	methodRepo := &stubSigninMethodRepo{
		items: []*repository.InstitutionSigninMethod{{
			ID: 1, InstitutionID: 1, Name: repository.SigninMethodPassword,
			DisplayName: "Password", Identifiers: []string{repository.PasswordIdentifierUsername}, State: "active",
		}},
		byApp: map[int64][]int64{1: {1}},
	}
	providerRepo := &stubProviderRepo{}
	signin := NewSigninMethodService(methodRepo, providerRepo, nil, audits)
	resolver := NewSigninPolicyResolver(methodRepo, providerRepo, nil)
	return NewApplicationServiceWithResources(apps, orgs, providerRepo, nil, signin, resolver, audits)
}

func testPasswordPolicy() *SigninPolicyResolver {
	methodRepo := &stubSigninMethodRepo{
		items: []*repository.InstitutionSigninMethod{{
			ID: 1, InstitutionID: 1, Name: repository.SigninMethodPassword,
			DisplayName: "Password", Identifiers: []string{repository.PasswordIdentifierUsername}, State: "active",
		}},
		byApp: map[int64][]int64{1: {1}},
	}
	return NewSigninPolicyResolver(methodRepo, &stubProviderRepo{}, nil)
}

func TestResolverOrganizationMethodsFiltersDisabled(t *testing.T) {
	resolver, _ := resolverFixture()
	methods, err := resolver.OrganizationMethods(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 {
		t.Fatalf("active Password + 启用的 Provider 应保留 2 条，实际 %+v", methods)
	}
	if methods[0].Name != repository.SigninMethodPassword {
		t.Fatalf("Password 应排在前，实际 %+v", methods)
	}
}

func TestResolverApplicationMethodsUsesSelectionAndSkipsInvalidProvider(t *testing.T) {
	resolver, app := resolverFixture()
	methods, err := resolver.ApplicationMethods(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 {
		t.Fatalf("应用应得到 Password + 可用 Provider 共 2 条，实际 %+v", methods)
	}
	for _, method := range methods {
		if method.ProviderID == 8 {
			t.Fatalf("停用 provider 不应下发: %+v", methods)
		}
	}
}

func TestResolverMethodsPlatformVsApplication(t *testing.T) {
	resolver, app := resolverFixture()
	org, err := resolver.Methods(context.Background(), 1, nil)
	if err != nil || len(org) != 2 {
		t.Fatalf("平台登录应使用组织方法，实际 %+v err=%v", org, err)
	}
	appMethods, err := resolver.Methods(context.Background(), 1, app)
	if err != nil || len(appMethods) != 2 {
		t.Fatalf("应用登录应使用选择交集，实际 %+v err=%v", appMethods, err)
	}
}

func TestResolverProviderItemsFiltersDisabledProvider(t *testing.T) {
	resolver, app := resolverFixture()
	methods, _ := resolver.ApplicationMethods(context.Background(), app)
	items := resolver.ProviderItems(context.Background(), methods)
	if len(items) != 1 || items[0].Name != "dingtalk" {
		t.Fatalf("ProviderItems 应只保留 dingtalk，实际 %+v", items)
	}
}
