package service

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

// 未在编排中声明的字段必须被拒绝（防越权字段注入）。
func TestValidateSignupFieldsRejectsUndeclaredField(t *testing.T) {
	items := []repository.SignupItem{
		{Name: "Username", Visible: true, Required: true},
		{Name: "Email", Visible: true, Required: true},
		{Name: "Password", Visible: true, Required: true},
	}
	values := map[string]string{"username": "alice", "email": "a@example.com", "password": "pw"}
	if err := validateSignupFields(items, values, "alice", "a@example.com", "pw"); err != nil {
		t.Fatalf("合法提交不应报错: %v", err)
	}
	values["is_admin"] = "true"
	if err := validateSignupFields(items, values, "alice", "a@example.com", "pw"); err == nil {
		t.Fatal("未声明的字段应被拒绝")
	}
}

// 编排中 required 的字段缺失必须报错。
func TestValidateSignupFieldsRequiresDeclaredFields(t *testing.T) {
	items := []repository.SignupItem{
		{Name: "Username", Visible: true, Required: true},
		{Name: "Phone", Visible: true, Required: true},
		{Name: "Password", Visible: true, Required: true},
	}
	values := map[string]string{"username": "alice", "password": "pw"}
	if err := validateSignupFields(items, values, "alice", "", "pw"); err == nil {
		t.Fatal("缺失必填字段应被拒绝")
	}
}

// 声明了 regex 的字段必须匹配。
func TestValidateSignupFieldsAppliesRegex(t *testing.T) {
	items := []repository.SignupItem{
		{Name: "Username", Visible: true, Required: true},
		{Name: "Phone", Visible: true, Required: true, Regex: `^\d{11}$`},
		{Name: "Password", Visible: true, Required: true},
	}
	values := map[string]string{"username": "alice", "phone": "123", "password": "pw"}
	if err := validateSignupFields(items, values, "alice", "", "pw"); err == nil {
		t.Fatal("不匹配正则的字段应被拒绝")
	}
}

// 未配置编排时跳过校验（兼容历史应用）。
func TestValidateSignupFieldsSkipsWhenEmpty(t *testing.T) {
	if err := validateSignupFields(nil, map[string]string{"anything": "x"}, "a", "b", "c"); err != nil {
		t.Fatalf("空编排应跳过校验: %v", err)
	}
}

// 请求归一化：核心字段与自定义编排字段都进入同一命名空间。
func TestSignupValuesNormalizesKeys(t *testing.T) {
	got := signupValues(SignupRequest{
		Name: "alice", Email: "a@example.com", Password: "pw",
		Fields: map[string]string{"First name": "Alice"},
	})
	if got["username"] != "alice" || got["email"] != "a@example.com" || got["password"] != "pw" {
		t.Fatalf("核心字段归一化失败: %+v", got)
	}
	if got["first name"] != "Alice" {
		t.Fatalf("自定义字段应小写归一化: %+v", got)
	}
}

// stubDefaultJobTitleRepo 只实现默认职务接线需要的两个方法。
type stubDefaultJobTitleRepo struct {
	repository.JobTitleRepository
	item    *repository.JobTitle
	lookErr error
	addedID int64
	added   []string
}

func (s *stubDefaultJobTitleRepo) GetByName(context.Context, int64, string) (*repository.JobTitle, error) {
	return s.item, s.lookErr
}

func (s *stubDefaultJobTitleRepo) AddMembers(_ context.Context, id int64, users []string) error {
	s.addedID = id
	s.added = append(s.added, users...)
	return nil
}

// 回归：应用「默认职务」不能只写 users.signup_application，必须真正建立职务关系。
func TestAddDefaultJobTitleAddsMember(t *testing.T) {
	repo := &stubDefaultJobTitleRepo{item: &repository.JobTitle{ID: 41, InstitutionID: 3, Name: "AI"}}
	svc := &AuthService{jobTitles: repo}
	svc.addDefaultJobTitle(context.Background(), 3, 41, "alice")
	if repo.addedID != 41 || len(repo.added) != 1 || repo.added[0] != "alice" {
		t.Fatalf("默认职务未加入新用户: id=%d users=%v", repo.addedID, repo.added)
	}
}

// Regression: an empty default job-title ID is ignored.
func TestAddDefaultJobTitleMissingIsIgnored(t *testing.T) {
	repo := &stubDefaultJobTitleRepo{}
	svc := &AuthService{jobTitles: repo}
	svc.addDefaultJobTitle(context.Background(), 3, 0, "alice")
	if repo.addedID != 0 || len(repo.added) != 0 {
		t.Fatalf("职务不存在时不应写成员: %+v", repo)
	}
}
