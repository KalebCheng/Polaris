package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
)

func newQueryContext(target string, authCtx *auth.AuthContext) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", target, nil)
	if authCtx != nil {
		c.Set(auth.AuthContextKey, authCtx)
	}
	return c
}

// 平台管理员 + scope=platform → 跨组织（空串以触达全平台列表）。
func TestInstOfPlatformScopeForAdmin(t *testing.T) {
	admin := &auth.AuthContext{UserID: 1, InstitutionID: 1, IsAdmin: true}
	c := newQueryContext("/api/v1/roles?scope=platform", admin)
	if got := instID(c); got != 0 {
		t.Fatalf("平台管理员 scope=platform 应返回 0，得到 %d", got)
	}
}

// 非平台管理员携带 scope=platform 时不得越权，退回其所在组织。
func TestInstOfPlatformScopeRejectedForTenantAdmin(t *testing.T) {
	tenant := &auth.AuthContext{UserID: 2, InstitutionID: 1}
	c := newQueryContext("/api/v1/roles?scope=platform", tenant)
	if got := instID(c); got != 1 {
		t.Fatalf("非平台管理员 scope=platform 应退回所在组织(1)，得到 %d", got)
	}
}

// 显式 institution 参数优先于登录用户所在机构。
func TestInstOfExplicitInstitution(t *testing.T) {
	c := newQueryContext("/api/v1/roles?institution=2", &auth.AuthContext{UserID: 1, InstitutionID: 1})
	if got := instID(c); got != 2 {
		t.Fatalf("应返回显式 institution=2，得到 %d", got)
	}
}

// 缺省回退到登录用户所在机构。
func TestInstOfFallsBackToAuth(t *testing.T) {
	c := newQueryContext("/api/v1/roles", &auth.AuthContext{UserID: 1, InstitutionID: 1})
	if got := instID(c); got != 1 {
		t.Fatalf("应回退到登录机构(1)，得到 %d", got)
	}
}

// 上传目标组织：管理员可写入其工作区组织，非管理员不得越租户。
func TestUploadInstitution(t *testing.T) {
	adminOfOrg1 := &auth.AuthContext{UserID: 1, InstitutionID: 1, IsAdmin: true}
	tenantOfOrg1 := &auth.AuthContext{UserID: 2, InstitutionID: 1}

	cases := []struct {
		name     string
		target   string
		ctx      *auth.AuthContext
		wantID   int64
		wantCode string
	}{
		{"管理员显式指定组织（工作区）", "/api/v1/upload?institution=2", adminOfOrg1, 2, ""},
		{"未指定组织则回落会话组织", "/api/v1/upload", adminOfOrg1, 1, ""},
		{"非管理员写入他组织被拒", "/api/v1/upload?institution=2", tenantOfOrg1, 0, "FORBIDDEN"},
		{"非管理员写自己的组织放行", "/api/v1/upload?institution=1", tenantOfOrg1, 1, ""},
		{"无法解析组织时拒绝", "/api/v1/upload?institution=abc", &auth.AuthContext{UserID: 3, InstitutionID: 0}, 0, "INVALID_PARAMS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := uploadInstitution(newQueryContext(tc.target, tc.ctx))
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("不应报错: %v", err)
				}
				if id != tc.wantID {
					t.Fatalf("组织 id = %d, want %d", id, tc.wantID)
				}
				return
			}
			if err == nil {
				t.Fatal("应被拒绝")
			}
			bizErr, ok := err.(*bizerrors.Error)
			if !ok || bizErr.Code.String() != tc.wantCode {
				t.Fatalf("业务码 = %v, want %s", err, tc.wantCode)
			}
		})
	}
}
