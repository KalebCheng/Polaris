package service

import (
	"context"
	"testing"
	"time"

	"polaris/internal/auth/oauth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

type failingStateStore struct{}

func (failingStateStore) Save(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (failingStateStore) Take(context.Context, string) ([]byte, error) {
	return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error")
}

// 回归：回调不能把 Redis 故障伪装成 SSO_STATE_INVALID，否则真实原因会被日志吞掉。
func TestCallbackPropagatesStateStoreError(t *testing.T) {
	svc := &AuthService{states: failingStateStore{}}
	_, _, err := svc.Callback(context.Background(), "code", "state", "", "")
	if !bizerrors.Is(err, bizerrors.CodeRedisError) {
		t.Fatalf("Callback error = %v, want REDIS_ERROR", err)
	}
}

type matchOrgRepo struct {
	repository.InstitutionRepository
}

func (matchOrgRepo) GetByID(_ context.Context, id int64) (*repository.Institution, error) {
	return &repository.Institution{ID: id}, nil
}

type matchUserRepo struct {
	repository.UserRepository
	candidates []*repository.User
	bound      bool
	field      string
	value      string
	syncerID   int64
	linkField  string
	// JIT 建号捕获
	createdUser     *repository.User
	createdIdentity *repository.UserIdentity
}

func (f *matchUserRepo) GetIdentity(context.Context, int64, string, string) (*repository.UserIdentity, error) {
	return nil, bizerrors.ErrNotFound
}

func (f *matchUserRepo) FindMatchCandidates(_ context.Context, _ int64, field, value string, _ int) ([]*repository.User, error) {
	f.field = field
	f.value = value
	return f.candidates, nil
}

func (f *matchUserRepo) FindIdentityLinkMatchCandidates(
	_ context.Context, _ int64, syncerID int64, field, value string, _ int,
) ([]*repository.User, error) {
	f.syncerID = syncerID
	f.linkField = field
	f.value = value
	return f.candidates, nil
}

func (f *matchUserRepo) BindIdentity(context.Context, *repository.UserIdentity) (int64, error) {
	f.bound = true
	return 1, nil
}

func (f *matchUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return nil, bizerrors.ErrNotFound
}

func (f *matchUserRepo) CreateWithIdentity(_ context.Context, u *repository.User, identity *repository.UserIdentity) (int64, error) {
	f.createdUser = u
	f.createdIdentity = identity
	return 99, nil
}

func matchProvider() *repository.Provider {
	return &repository.Provider{
		ID: 1, InstitutionID: 2, Name: "dingtalk-insuresmart-auth", Type: "dingtalk", AutoCreate: true,
		MatchType: "system", MatchLocalField: "email", MatchProviderField: "email",
	}
}

func TestProvisionBindsUniqueMatch(t *testing.T) {
	existing := &repository.User{ID: 50, InstitutionID: 2, Name: "17397636699832229"}
	users := &matchUserRepo{candidates: []*repository.User{existing}}
	svc := &AuthService{orgs: matchOrgRepo{}, users: users}

	user, created, err := svc.provision(context.Background(), matchProvider(), &oauth.UserInfo{
		ProviderUserID: "unionid", Email: "same@example.com", DisplayName: "成凯",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if created || user != existing {
		t.Fatalf("应命中并返回已有用户: user=%+v created=%v", user, created)
	}
	if !users.bound {
		t.Fatal("唯一命中后应写入 OAuth 身份绑定")
	}
}

// 身份链接字段可以是连接的任意源字段（如 unionid），不再限于 external_id——
// 这是"同一个钉钉应用同时被 SSO 与通讯录连接引用"能被认领的前提。
func TestProvisionMatchesIdentityLinkByRawSourceField(t *testing.T) {
	existing := &repository.User{ID: 52, InstitutionID: 2, Name: "synced-by-unionid"}
	users := &matchUserRepo{candidates: []*repository.User{existing}}
	svc := &AuthService{orgs: matchOrgRepo{}, users: users}
	item := matchProvider()
	item.MatchType = "identity_link"
	item.MatchSyncerID = 9
	item.MatchSyncerField = "unionid"
	item.MatchLocalField = ""
	item.MatchProviderField = "unionid"
	item.AutoCreate = false

	user, _, err := svc.provision(context.Background(), item, &oauth.UserInfo{
		ProviderUserID: "ding-open", UnionID: "ding-union-9",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if user != existing {
		t.Fatalf("应命中 unionid 对应的已有用户: %+v", user)
	}
	if users.linkField != "unionid" || users.value != "ding-union-9" {
		t.Fatalf("匹配条件 = field:%q value:%q", users.linkField, users.value)
	}
}

// JIT 建号的字段来源由 provision_* 策略决定，并写入 attributes 供同步侧反向认领。
func TestProvisionAutoCreateUsesConfiguredFieldMapping(t *testing.T) {
	users := &matchUserRepo{}
	svc := &AuthService{orgs: matchOrgRepo{}, users: users}
	item := matchProvider()
	item.MatchType = ""
	item.MatchLocalField = ""
	item.MatchProviderField = ""
	item.AutoCreate = true
	item.ProvisionUsernameField = "preferred_username"
	item.ProvisionNameField = "name"
	item.ProvisionEmailField = "email"
	item.ProvisionPhoneField = "mobile"
	item.ProvisionAvatarField = "avatar"

	_, created, err := svc.provision(context.Background(), item, &oauth.UserInfo{
		ProviderUserID: "ding-open-1", UnionID: "ding-union-1",
		Username: "cheng.kai", DisplayName: "成凯",
		Email: "ck@example.com", Phone: "13800000000", Avatar: "https://cdn/avatar.png",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !created || users.createdUser == nil {
		t.Fatal("未建号")
	}
	got := users.createdUser
	if got.Name != "cheng.kai" || got.DisplayName != "成凯" || got.Email != "ck@example.com" ||
		got.Phone != "13800000000" || got.Avatar != "https://cdn/avatar.png" {
		t.Fatalf("字段映射未按策略装配: %+v", got)
	}
	if users.createdIdentity == nil || users.createdIdentity.Attributes["unionid"] != "ding-union-1" {
		t.Fatalf("建号必须记录提供商 attributes: %+v", users.createdIdentity)
	}
}

func TestProvisionRejectsAmbiguousMatch(t *testing.T) {
	users := &matchUserRepo{candidates: []*repository.User{{ID: 1}, {ID: 2}}}
	svc := &AuthService{orgs: matchOrgRepo{}, users: users}

	_, _, err := svc.provision(context.Background(), matchProvider(), &oauth.UserInfo{
		ProviderUserID: "unionid", Email: "dup@example.com",
	})
	if !bizerrors.Is(err, bizerrors.CodeIdentityAmbiguous) {
		t.Fatalf("多命中 error = %v, want IDENTITY_AMBIGUOUS", err)
	}
	if users.bound {
		t.Fatal("多命中时不应绑定")
	}
}

func TestProvisionRejectsMissingMatchField(t *testing.T) {
	svc := &AuthService{orgs: matchOrgRepo{}, users: &matchUserRepo{}}
	_, _, err := svc.provision(context.Background(), matchProvider(), &oauth.UserInfo{
		ProviderUserID: "unionid",
	})
	if !bizerrors.Is(err, bizerrors.CodeIdentityNotBound) {
		t.Fatalf("缺少匹配字段 error = %v, want IDENTITY_NOT_BOUND", err)
	}
}

// 身份链接匹配必须同时带上入方向连接 ID 和 external_id；
// 唯一命中后仍走同一套绑定流程。
func TestProvisionMatchesIdentityExternalID(t *testing.T) {
	existing := &repository.User{ID: 51, InstitutionID: 2, Name: "synced-user"}
	users := &matchUserRepo{candidates: []*repository.User{existing}}
	svc := &AuthService{orgs: matchOrgRepo{}, users: users}
	item := matchProvider()
	item.MatchType = "identity_link"
	item.MatchSyncerID = 9
	item.MatchSyncerField = "external_id"
	item.MatchLocalField = ""
	item.MatchProviderField = "unionid"
	item.AutoCreate = false

	user, created, err := svc.provision(context.Background(), item, &oauth.UserInfo{
		ProviderUserID: "unionid", UnionID: "ding-42",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if created || user != existing {
		t.Fatalf("应命中身份链接对应的已有用户: user=%+v created=%v", user, created)
	}
	if users.syncerID != 9 || users.linkField != "external_id" || users.value != "ding-42" {
		t.Fatalf("匹配条件 = syncer:%d field:%q value:%q", users.syncerID, users.linkField, users.value)
	}
	if !users.bound {
		t.Fatal("唯一命中后应写入 OAuth 身份绑定")
	}
}
