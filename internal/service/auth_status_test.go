package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

type statusSessionRepo struct {
	repository.SessionRepository
	data []byte
}

func (r *statusSessionRepo) Get(context.Context, string) ([]byte, error) { return r.data, nil }

type statusUserRepo struct {
	repository.UserRepository
	user *repository.User
}

func (r *statusUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return r.user, nil
}

type statusOrgRepo struct {
	repository.InstitutionRepository
	org *repository.Institution
}

func (r *statusOrgRepo) GetByID(context.Context, int64) (*repository.Institution, error) {
	return r.org, nil
}

// 回归：账号菜单与个人资料页曾直接展示组织主键，且顶栏拿不到头像。
// 状态接口必须回填组织展示名与用户头像（头像可能在登录后才上传，不能只读会话快照）。
func TestStatusIncludesInstitutionNameAndAvatar(t *testing.T) {
	data, err := json.Marshal(auth.Session{
		ID:            "sid-1",
		InstitutionID: 7,
		Name:          "10002",
		DisplayName:   "张三",
		Email:         "zhangsan@example.com",
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	svc := &AuthService{
		orgs: &statusOrgRepo{org: &repository.Institution{
			ID: 7, Name: "acme", DisplayName: "Polaris 科技",
			Logo: "https://cdn.example.com/logo.png", Favicon: "https://cdn.example.com/favicon.png",
		}},
		users: &statusUserRepo{user: &repository.User{
			InstitutionID: 7,
			Name:          "10002",
			Avatar:        "https://cdn.example.com/avatar.png",
		}},
		sessions: auth.NewSessionManager(&statusSessionRepo{data: data}, nil, nil, nil, time.Hour),
	}

	got := svc.Status(context.Background(), "sid-1")
	if !got.Authenticated {
		t.Fatal("expected authenticated status")
	}
	if got.InstitutionName != "Polaris 科技" {
		t.Fatalf("InstitutionName = %q, want 组织展示名", got.InstitutionName)
	}
	if got.InstitutionLogo != "https://cdn.example.com/logo.png" {
		t.Fatalf("InstitutionLogo = %q", got.InstitutionLogo)
	}
	if got.InstitutionFavicon != "https://cdn.example.com/favicon.png" {
		t.Fatalf("InstitutionFavicon = %q", got.InstitutionFavicon)
	}
	if got.Avatar != "https://cdn.example.com/avatar.png" {
		t.Fatalf("Avatar = %q, want 用户头像", got.Avatar)
	}
}
