package provider

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

// fakeRuntimeRepo 最小内存仓库：只覆盖 List（按类别过滤，institutionID=0 表示不过滤）。
type fakeRuntimeRepo struct {
	items []*repository.Provider
}

func (r *fakeRuntimeRepo) Create(context.Context, *repository.Provider) (int64, error) { return 0, nil }
func (r *fakeRuntimeRepo) GetByID(_ context.Context, id int64) (*repository.Provider, error) {
	for _, item := range r.items {
		if item.ID == id {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errRuntimeNotFound
}
func (r *fakeRuntimeRepo) GetByName(_ context.Context, institutionID int64, name string) (*repository.Provider, error) {
	for _, item := range r.items {
		if item.InstitutionID == institutionID && item.Name == name {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errRuntimeNotFound
}
func (r *fakeRuntimeRepo) Update(context.Context, *repository.Provider) error { return nil }
func (r *fakeRuntimeRepo) Delete(context.Context, int64) error                { return nil }
func (r *fakeRuntimeRepo) List(_ context.Context, q repository.ListQuery) ([]*repository.Provider, error) {
	out := make([]*repository.Provider, 0, len(r.items))
	for _, item := range r.items {
		if q.InstitutionID != 0 && item.InstitutionID != q.InstitutionID {
			continue
		}
		if q.Field == "category" && q.Value != "" && item.Category != q.Value {
			continue
		}
		clone := *item
		out = append(out, &clone)
	}
	return out, nil
}
func (r *fakeRuntimeRepo) Count(context.Context, repository.ListQuery) (int64, error) { return 0, nil }

type runtimeNotFound struct{}

func (runtimeNotFound) Error() string { return "not found" }

var errRuntimeNotFound = runtimeNotFound{}

// 回归：institutionID == 0 必须表示"跨组织"，不能当成错误。
//
// 第四阶段这里写成硬拒绝，导致两处能力静默失效：
//   - 登录页不带组织时列不出可用身份提供方；
//   - 只带提供商名发起 SSO（/auth/oauth/:provider/authorize）时无法按名称定位 → 必然 404。
func TestListActiveAcrossInstitutions(t *testing.T) {
	repo := &fakeRuntimeRepo{items: []*repository.Provider{
		{ID: 1, InstitutionID: 1, Name: "sso-a", Category: "oauth", State: "active"},
		{ID: 2, InstitutionID: 2, Name: "sso-b", Category: "oauth", State: "active"},
		{ID: 3, InstitutionID: 2, Name: "sso-off", Category: "oauth", State: "disabled"},
		{ID: 4, InstitutionID: 2, Name: "mail", Category: "email", State: "active"},
	}}
	runtime := NewRuntime(repo, nil)

	// 跨组织：两个启用中的 oauth 提供商都应返回（停用与其它类别不返回）
	all, err := runtime.ListActive(context.Background(), 0, CategoryOAuth)
	if err != nil {
		t.Fatalf("institutionID=0 应表示跨组织而不是报错: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("跨组织应返回 2 个启用中的 oauth 提供商，实际 %d", len(all))
	}

	// 指定组织：只返回该组织的
	scoped, err := runtime.ListActive(context.Background(), 2, CategoryOAuth)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].Name != "sso-b" {
		t.Fatalf("按组织过滤结果不正确: %+v", scoped)
	}

	// 类别过滤仍然生效
	emailOnly, err := runtime.ListActive(context.Background(), 0, CategoryEmail)
	if err != nil {
		t.Fatal(err)
	}
	if len(emailOnly) != 1 || emailOnly[0].Name != "mail" {
		t.Fatalf("类别过滤失效: %+v", emailOnly)
	}
}
