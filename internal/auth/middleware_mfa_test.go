package auth

import "testing"

// 回归：待绑定 MFA 的受限会话仅放行绑定/登出/状态类接口，其余一律拦截。
func TestMfaPendingAllowedPath(t *testing.T) {
	allowed := []string{
		"/api/v1/auth/mfa/options",
		"/api/v1/auth/mfa/enroll",
		"/api/v1/auth/mfa/enroll/resend",
		"/api/v1/auth/mfa/enroll/verify",
		"/api/v1/auth/mfa/verify",
		"/api/v1/auth/mfa/skip",
		"/api/v1/auth/mfa/setup",
		"/api/v1/auth/logout",
		"/api/v1/auth/status",
	}
	for _, path := range allowed {
		if !mfaPendingAllowedPath(path) {
			t.Fatalf("应放行: %s", path)
		}
	}
	blocked := []string{
		"/api/v1/users",
		"/api/v1/institutions",
		"/api/v1/auth/change-password",
		"/api/v1/roles",
	}
	for _, path := range blocked {
		if mfaPendingAllowedPath(path) {
			t.Fatalf("应拦截: %s", path)
		}
	}
}
