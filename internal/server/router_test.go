package server

import (
	"testing"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	"polaris/internal/config"
	"polaris/internal/handler"
)

func TestPersonalPortalRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	middleware := NewMiddleware(&config.Config{}, auth.NewMiddleware(nil, nil, nil, nil, nil))

	RegisterRoutes(engine, middleware, &Handlers{
		Auth:        (*handler.AuthHandler)(nil),
		OIDC:        (*handler.OIDCHandler)(nil),
		Health:      (*handler.HealthHandler)(nil),
		User:        (*handler.UserHandler)(nil),
		Institution: (*handler.InstitutionHandler)(nil),
		Department:  (*handler.DepartmentHandler)(nil),
		Application: (*handler.ApplicationHandler)(nil),
		Provider:    (*handler.ProviderHandler)(nil),
		Role:        (*handler.RoleHandler)(nil),
		Permission:  (*handler.PermissionHandler)(nil),
		JobTitle:    (*handler.JobTitleHandler)(nil),
		Cert:        (*handler.CertHandler)(nil),
		Token:       (*handler.TokenHandler)(nil),
		Session:     (*handler.SessionHandler)(nil),
		Record:      (*handler.RecordHandler)(nil),
		Syncer:      (*handler.SyncerHandler)(nil),
		Provision:   (*handler.ProvisioningHandler)(nil),
		Connection:  (*handler.ConnectionHandler)(nil),
		Asyncd:      (*handler.AsyncdHandler)(nil),
		Storage:     (*handler.StorageHandler)(nil),
	})

	registered := make(map[string]bool)
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/auth/app-access",
		"GET /api/v1/me/applications",
		"GET /api/v1/me/application-accounts",
		"GET /api/v1/me/identities",
		"POST /api/v1/me/identities/:provider/authorize",
		"DELETE /api/v1/me/identities/:identityID",
		"GET /api/v1/me/sessions",
		"DELETE /api/v1/me/sessions/:sessionID",
		"POST /api/v1/me/sessions/revoke-all",
		"GET /api/v1/me/records",
		"GET /api/v1/connections/catalog",
		"GET /api/v1/system/oauth",
		"GET /api/v1/auth/oauth/callback",
		"POST /api/v1/auth/oauth/callback",
		"POST /api/v1/connectors/:id/reconcile",
		"GET /api/v1/asyncd/jobs",
		"PUT /api/v1/asyncd/jobs/:id",
		"GET /api/v1/asyncd/jobs/:id/runs",
		"GET /api/v1/asyncd/jobs/:id/runs/:runId/attempts",
		"POST /api/v1/asyncd/jobs/:id/runs/:runId/retry",
		"GET /api/v1/asyncd/workers",
	} {
		if !registered[route] {
			t.Errorf("门户自助路由未注册: %s", route)
		}
	}
}

// 回归：系统存储设置与通用上传路由必须注册（新增端点必须接入调用点）。
func TestStorageRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	middleware := NewMiddleware(&config.Config{}, auth.NewMiddleware(nil, nil, nil, nil, nil))
	RegisterRoutes(engine, middleware, &Handlers{
		Auth: (*handler.AuthHandler)(nil), OIDC: (*handler.OIDCHandler)(nil),
		Health: (*handler.HealthHandler)(nil), User: (*handler.UserHandler)(nil),
		Institution: (*handler.InstitutionHandler)(nil), Department: (*handler.DepartmentHandler)(nil),
		Application: (*handler.ApplicationHandler)(nil), Provider: (*handler.ProviderHandler)(nil),
		Role: (*handler.RoleHandler)(nil), Permission: (*handler.PermissionHandler)(nil),
		JobTitle: (*handler.JobTitleHandler)(nil), Cert: (*handler.CertHandler)(nil),
		Token: (*handler.TokenHandler)(nil), Session: (*handler.SessionHandler)(nil),
		Record: (*handler.RecordHandler)(nil), Syncer: (*handler.SyncerHandler)(nil),
		Provision: (*handler.ProvisioningHandler)(nil), Storage: (*handler.StorageHandler)(nil),
	})

	registered := make(map[string]bool)
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/system/storage",
		"PUT /api/v1/system/storage",
		"POST /api/v1/system/storage/test",
		"POST /api/v1/upload",
	} {
		if !registered[route] {
			t.Errorf("存储路由未注册: %s", route)
		}
	}
}
