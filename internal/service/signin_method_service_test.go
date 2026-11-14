package service

import (
	"context"
	"errors"
	"testing"

	"polaris/internal/repository"
)

type fakeSigninRepo struct {
	repository.InstitutionSigninMethodRepository
	items   []*repository.InstitutionSigninMethod
	nextID  int64
	apps    []*repository.Application
	mfaByID map[int64][]int64
}

func (r *fakeSigninRepo) ensure() {
	if r.nextID == 0 {
		r.nextID = 1
		for _, item := range r.items {
			if item.ID >= r.nextID {
				r.nextID = item.ID + 1
			}
		}
	}
}

func (r *fakeSigninRepo) Create(_ context.Context, method *repository.InstitutionSigninMethod) (int64, error) {
	r.ensure()
	method.ID = r.nextID
	r.nextID++
	clone := *method
	clone.MfaProviderIDs = append([]int64(nil), method.MfaProviderIDs...)
	r.items = append(r.items, &clone)
	return method.ID, nil
}

func (r *fakeSigninRepo) GetByID(_ context.Context, institutionID, id int64) (*repository.InstitutionSigninMethod, error) {
	for _, item := range r.items {
		if item.ID == id && item.InstitutionID == institutionID {
			clone := *item
			return &clone, nil
		}
	}
	return nil, errors.New("not found")
}

func (r *fakeSigninRepo) List(_ context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range r.items {
		if item.InstitutionID == institutionID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeSigninRepo) ListActive(ctx context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	items, _ := r.List(ctx, institutionID)
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range items {
		if item.State == "active" {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeSigninRepo) ListByIDs(ctx context.Context, institutionID int64, ids []int64) ([]*repository.InstitutionSigninMethod, error) {
	allowed := map[int64]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	items, _ := r.List(ctx, institutionID)
	out := []*repository.InstitutionSigninMethod{}
	for _, item := range items {
		if allowed[item.ID] {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fakeSigninRepo) Update(_ context.Context, method *repository.InstitutionSigninMethod) error {
	for i, item := range r.items {
		if item.ID == method.ID {
			clone := *method
			r.items[i] = &clone
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeSigninRepo) SetState(_ context.Context, institutionID, id int64, state string) error {
	for _, item := range r.items {
		if item.ID == id && item.InstitutionID == institutionID {
			item.State = state
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeSigninRepo) Delete(_ context.Context, institutionID, id int64) error {
	for i, item := range r.items {
		if item.ID == id && item.InstitutionID == institutionID {
			r.items = append(r.items[:i], r.items[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeSigninRepo) ListApplicationsByMethod(context.Context, int64, int64) ([]*repository.Application, error) {
	return r.apps, nil
}

func newFakeSigninService(items ...*repository.InstitutionSigninMethod) (*SigninMethodService, *fakeSigninRepo) {
	repo := &fakeSigninRepo{items: items}
	providers := &stubProviderRepo{}
	return NewSigninMethodService(repo, providers, nil, nil), repo
}

func TestSigninMethodServiceCreatesPasswordAndRejectsDuplicate(t *testing.T) {
	svc, repo := newFakeSigninService()
	created, err := svc.Create(context.Background(), "admin", 1, SigninMethodInput{
		Name: "Password", Identifiers: []string{"email", "username", "email"},
		MfaEnabled: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Identifiers) != 2 || created.Identifiers[0] != "username" || created.Identifiers[1] != "email" {
		t.Fatalf("identifiers 未归一化: %+v", created.Identifiers)
	}
	if len(repo.items) != 1 {
		t.Fatalf("应落一条方法，实际 %d", len(repo.items))
	}
	if _, err := svc.Create(context.Background(), "admin", 1, SigninMethodInput{
		Name: "Password", Identifiers: []string{"username"},
	}); err == nil {
		t.Fatal("同组织第二条 Password 应被拒绝")
	}
}

func TestSigninMethodServiceProviderDefaultsDisplayName(t *testing.T) {
	repo := &fakeSigninRepo{}
	providers := &stubProviderRepo{byID: map[int64]*repository.Provider{
		7: {ID: 7, InstitutionID: 1, Name: "dingtalk", DisplayName: "钉钉", Category: "oauth", State: "active"},
	}}
	svc := NewSigninMethodService(repo, providers, nil, nil)
	created, err := svc.Create(context.Background(), "admin", 1, SigninMethodInput{
		Name: "Provider", ProviderID: int64Ptr(7),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.DisplayName != "钉钉" {
		t.Fatalf("Provider 默认展示名应取提供商名称，实际 %q", created.DisplayName)
	}
}

func TestSigninMethodServiceProtectsLastActiveMethod(t *testing.T) {
	svc, _ := newFakeSigninService(&repository.InstitutionSigninMethod{
		ID: 1, InstitutionID: 1, Name: "Password", DisplayName: "Password",
		Identifiers: []string{"username"}, State: "active",
	})
	if err := svc.SetState(context.Background(), "admin", 1, 1, "disabled"); err == nil {
		t.Fatal("禁用最后一个启用方法应被拒绝")
	}
	if err := svc.Delete(context.Background(), "admin", 1, 1); err == nil {
		t.Fatal("删除最后一个启用方法应被拒绝")
	}
}

func TestSigninMethodServiceDeleteRejectsApplicationReference(t *testing.T) {
	svc, repo := newFakeSigninService(
		&repository.InstitutionSigninMethod{ID: 1, InstitutionID: 1, Name: "Password", Identifiers: []string{"username"}, State: "active"},
		&repository.InstitutionSigninMethod{ID: 2, InstitutionID: 1, Name: "MagicLink", State: "active"},
	)
	repo.apps = []*repository.Application{{ID: 9, Code: "portal"}}
	if err := svc.Delete(context.Background(), "admin", 1, 2); err == nil {
		t.Fatal("被应用引用的方法不应允许硬删")
	}
}

func TestSigninMethodServiceValidateSelectionRejectsDisabledAndUnknown(t *testing.T) {
	svc, _ := newFakeSigninService(
		&repository.InstitutionSigninMethod{ID: 1, InstitutionID: 1, Name: "Password", State: "active"},
		&repository.InstitutionSigninMethod{ID: 2, InstitutionID: 1, Name: "MagicLink", State: "disabled"},
	)
	if _, err := svc.ValidateSelection(context.Background(), 1, []int64{1}); err != nil {
		t.Fatalf("合法选择不应报错: %v", err)
	}
	if _, err := svc.ValidateSelection(context.Background(), 1, []int64{2}); err == nil {
		t.Fatal("禁用方法不应允许选择")
	}
	if _, err := svc.ValidateSelection(context.Background(), 1, []int64{1, 99}); err == nil {
		t.Fatal("未知方法不应允许选择")
	}
	if _, err := svc.ValidateSelection(context.Background(), 1, nil); err == nil {
		t.Fatal("空选择应被拒绝")
	}
}

func TestApplicationCreateDefaultsToAllActiveSigninMethods(t *testing.T) {
	apps := &stubAppRepo{}
	svc := applicationServiceForTest(apps, &stubOrgRepo{}, nil)
	created, err := svc.Create(context.Background(), "admin", &repository.Application{
		InstitutionID: 1, Code: "portal", RedirectURIs: []string{"https://rp.example.com/cb"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.SigninMethodIDs) != 1 || created.SigninMethodIDs[0] != 1 {
		t.Fatalf("新应用应默认全选组织启用方法，实际 %+v", created.SigninMethodIDs)
	}
}
