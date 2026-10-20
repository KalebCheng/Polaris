package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"polaris/internal/auth"
	"polaris/internal/auth/oauth"
	"polaris/internal/repository"
)

type selfServiceUserRepo struct {
	repository.UserRepository
	identities []*repository.UserIdentity
	unboundID  int64
	identity   *repository.UserIdentity
	bound      *repository.UserIdentity
}

func (r *selfServiceUserRepo) GetIdentitiesByUser(context.Context, int64, string) ([]*repository.UserIdentity, error) {
	return r.identities, nil
}

func (r *selfServiceUserRepo) UnbindIdentity(_ context.Context, id int64) error {
	r.unboundID = id
	return nil
}

func (r *selfServiceUserRepo) GetIdentity(context.Context, int64, string, string) (*repository.UserIdentity, error) {
	if r.identity == nil {
		return nil, errors.New("identity not found")
	}
	return r.identity, nil
}

func (r *selfServiceUserRepo) BindIdentity(_ context.Context, identity *repository.UserIdentity) (int64, error) {
	r.bound = identity
	return 1, nil
}

type selfServiceSessionRepo struct {
	repository.SessionRepository
	sessions map[string][]byte
	ids      []string
	deleted  []string
}

func (r *selfServiceSessionRepo) Get(_ context.Context, id string) ([]byte, error) {
	return r.sessions[id], nil
}

func (r *selfServiceSessionRepo) Delete(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	delete(r.sessions, id)
	return nil
}

func (r *selfServiceSessionRepo) RemoveUserSession(context.Context, int64, string, string) error {
	return nil
}

func (r *selfServiceSessionRepo) UserSessions(context.Context, int64, string) ([]string, error) {
	return r.ids, nil
}

func (r *selfServiceSessionRepo) DeleteUserSessions(context.Context, int64, string) ([]string, error) {
	r.deleted = append(r.deleted, r.ids...)
	return r.ids, nil
}

type selfServiceRecordRepo struct {
	repository.RecordRepository
	listQuery  repository.ListQuery
	countQuery repository.ListQuery
}

func (r *selfServiceRecordRepo) List(_ context.Context, q repository.ListQuery) ([]*repository.Record, error) {
	r.listQuery = q
	return []*repository.Record{}, nil
}

func (r *selfServiceRecordRepo) Count(_ context.Context, q repository.ListQuery) (int64, error) {
	r.countQuery = q
	return 0, nil
}

type selfServiceProvisioningUserRepo struct {
	repository.UserRepository
	users map[string]*repository.User
}

func (r *selfServiceProvisioningUserRepo) GetByName(_ context.Context, _ int64, name string) (*repository.User, error) {
	if u := r.users[name]; u != nil {
		return u, nil
	}
	return nil, errors.New("user not found")
}

func TestUnbindOwnIdentityRejectsIdentityOwnedByAnotherUser(t *testing.T) {
	repo := &selfServiceUserRepo{
		identities: []*repository.UserIdentity{{ID: 7, InstitutionID: 1, User: "bob"}},
	}
	svc := NewUserService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	if err := svc.UnbindOwnIdentity(context.Background(), 1, "alice", 7); err == nil {
		t.Fatal("不应允许当前用户解绑其他用户的身份")
	}
	if repo.unboundID != 0 {
		t.Fatalf("不应删除其他用户身份，实际删除 ID=%d", repo.unboundID)
	}

	if err := svc.UnbindOwnIdentity(context.Background(), 1, "bob", 7); err != nil {
		t.Fatalf("用户应可解绑自己的身份: %v", err)
	}
	if repo.unboundID != 7 {
		t.Fatalf("应删除身份 ID=7，实际为 %d", repo.unboundID)
	}
}

func TestRevokeOwnSessionRejectsForeignSession(t *testing.T) {
	foreign, _ := json.Marshal(&auth.Session{ID: "session-bob", InstitutionID: 1, Name: "bob"})
	owned, _ := json.Marshal(&auth.Session{ID: "session-alice", InstitutionID: 1, Name: "alice"})
	repo := &selfServiceSessionRepo{
		sessions: map[string][]byte{"session-bob": foreign, "session-alice": owned},
	}
	manager := auth.NewSessionManager(repo, nil, nil, nil, 0)
	svc := NewSessionService(manager, nil)

	if err := svc.RevokeOwnSession(context.Background(), 1, "alice", "session-bob"); err == nil {
		t.Fatal("不应允许用户撤销其他用户的会话")
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("外部会话不应被删除，实际删除: %v", repo.deleted)
	}
	if err := svc.RevokeOwnSession(context.Background(), 1, "alice", "session-alice"); err != nil {
		t.Fatalf("用户应可撤销自己的会话: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != "session-alice" {
		t.Fatalf("应只删除本人会话，实际删除: %v", repo.deleted)
	}
}

func TestOwnSessionsFiltersMismatchedRedisIndexEntries(t *testing.T) {
	foreign, _ := json.Marshal(&auth.Session{ID: "session-bob", InstitutionID: 1, Name: "bob"})
	owned, _ := json.Marshal(&auth.Session{ID: "session-alice", InstitutionID: 1, Name: "alice"})
	repo := &selfServiceSessionRepo{
		sessions: map[string][]byte{"session-bob": foreign, "session-alice": owned},
		ids:      []string{"session-bob", "session-alice"},
	}
	manager := auth.NewSessionManager(repo, nil, nil, nil, 0)
	svc := NewSessionService(manager, nil)

	items, err := svc.OwnSessions(context.Background(), 1, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "session-alice" {
		t.Fatalf("个人会话列表必须复核租户与用户归属: %+v", items)
	}
}

func TestCompleteIdentityLinkBindsToInitiatingSessionAndRejectsOtherOwner(t *testing.T) {
	sessionBytes, _ := json.Marshal(&auth.Session{ID: "session-alice", InstitutionID: 1, Name: "alice"})
	repo := &selfServiceSessionRepo{sessions: map[string][]byte{"session-alice": sessionBytes}}
	manager := auth.NewSessionManager(repo, nil, nil, nil, 0)
	users := &selfServiceUserRepo{}
	svc := &AuthService{sessions: manager, users: users}

	result, sessionID, err := svc.completeIdentityLink(context.Background(), "session-alice", "github", &oauth.UserInfo{
		ProviderUserID: "github-123", Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("绑定应成功: %v", err)
	}
	if sessionID != "" || result == nil || !result.IdentityLinked {
		t.Fatalf("应返回绑定完成结果而不是新登录会话: result=%+v session=%q", result, sessionID)
	}
	if users.bound == nil || users.bound.InstitutionID != 1 || users.bound.User != "alice" ||
		users.bound.Provider != "github" || users.bound.ProviderUserID != "github-123" {
		t.Fatalf("身份必须绑定到发起授权的会话用户: %+v", users.bound)
	}

	users.identity = &repository.UserIdentity{
		InstitutionID: 1, User: "bob", Provider: "github", ProviderUserID: "other-subject",
	}
	users.bound = nil
	if _, _, err := svc.completeIdentityLink(context.Background(), "session-alice", "github", &oauth.UserInfo{
		ProviderUserID: "other-subject",
	}); err == nil {
		t.Fatal("已关联到其他账号的外部身份不应被转绑")
	}
	if users.bound != nil {
		t.Fatal("冲突身份不应执行绑定")
	}
}

func TestIdentityLinkRequiresCookieBackedSession(t *testing.T) {
	svc := &AuthService{}
	if _, _, err := svc.AuthorizeIdentityLink(context.Background(), "", "github", ""); err == nil {
		t.Fatal("缺少会话 ID 的 API 身份不能发起外部身份绑定")
	}
}

func TestListForUserScopesAuditRecordsByTenantAndUser(t *testing.T) {
	repo := &selfServiceRecordRepo{}
	svc := NewRecordService(repo)

	if _, _, err := svc.ListForUser(context.Background(), 1, "alice", 20, 10); err != nil {
		t.Fatal(err)
	}
	for _, q := range []repository.ListQuery{repo.listQuery, repo.countQuery} {
		if q.InstitutionID != 1 || q.Field != `"user"` || q.Value != "alice" || q.Offset != 20 || q.Limit != 10 {
			t.Fatalf("个人审计过滤条件不正确: %+v", q)
		}
	}
}

// 回归：门户应用中心枚举「组织内启用应用」，再按访问授权过滤；
// 不能要求应用先有 application_accounts（OIDC 应用通常没有本地供给账号）。
func TestListApplicationsForUserListsActiveAppsFilteredByAccess(t *testing.T) {
	subjectRepo := &stubSubjectRepo{byApp: map[int64][]repository.ApplicationSubject{
		2: {departmentSubject(1)},
	}}
	accessUsers := &stubAccessUserRepo{users: map[string]*repository.User{
		"alice": {ID: 42, InstitutionID: 1, Name: "alice"},
	}}
	depts := &stubAccessDeptRepo{all: []*repository.Department{
		{ID: 1, InstitutionID: 1, Name: "tech", CodePath: "tech"},
		{ID: 2, InstitutionID: 1, Name: "fe", CodePath: "tech/fe"},
	}}
	userDepts := &stubAccessUserDeptRepo{byUser: map[int64][]int64{42: {2}}}
	access := NewApplicationAccessService(subjectRepo, accessUsers, &stubAccessGroupRepo{}, depts, userDepts, nil, &AuditRecorder{})

	apps := &stubAppRepo{all: []*repository.Application{
		{ID: 1, InstitutionID: 1, Code: "all-app", Name: "全员应用", State: "active", AccessMode: repository.AccessModeAll},
		{ID: 2, InstitutionID: 1, Code: "dept-app", Name: "部门应用", State: "active", AccessMode: repository.AccessModeRestricted},
		{ID: 3, InstitutionID: 1, Code: "denied-app", Name: "未授权应用", State: "active", AccessMode: repository.AccessModeRestricted},
		{ID: 4, InstitutionID: 1, Code: "disabled-app", Name: "停用应用", State: "disabled", AccessMode: repository.AccessModeAll},
	}}
	users := &selfServiceProvisioningUserRepo{users: map[string]*repository.User{
		"alice": {ID: 42, InstitutionID: 1, Name: "alice"},
	}}
	svc := &ProvisioningService{apps: apps, users: users, access: access}

	items, err := svc.ListApplicationsForUser(context.Background(), 1, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "all-app" || items[1].Name != "dept-app" {
		t.Fatalf("应返回全员可访问与部门授权应用（停用/未授权除外），实际 %+v", items)
	}
}
