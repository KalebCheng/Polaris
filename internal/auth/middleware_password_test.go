package auth

import "testing"

func TestPasswordChangeAllowedPath(t *testing.T) {
	allowed := []string{
		"/api/v1/auth/change-password",
		"/api/v1/auth/logout",
		"/api/v1/auth/status",
		"/api/v1/auth/mfa/options",
		"/api/v1/me",
	}
	for _, path := range allowed {
		if !passwordChangeAllowedPath(path) {
			t.Fatalf("应放行: %s", path)
		}
	}
	blocked := []string{
		"/api/v1/users",
		"/api/v1/institutions",
		"/api/v1/me/sessions",
		"/oauth/authorize",
	}
	for _, path := range blocked {
		if passwordChangeAllowedPath(path) {
			t.Fatalf("应拦截: %s", path)
		}
	}
}
