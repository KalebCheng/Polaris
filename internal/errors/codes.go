package errors

import (
	"fmt"
	"net/http"
)

// Code 业务码。将业务码的 wire 值、默认英文提示、HTTP 状态码与开发说明收敛到一处声明。
// 约定：
//  1. value 全局唯一——同一个字符串在整个服务里只有且仅有一个含义；
//  2. 命名 UPPER_SNAKE_CASE，在 API 语境下自解释；
//  3. 只有前端/调用方需要针对该 code 做不同处理时才新开业务 code，否则复用通用 code；
//  4. message 为下发给客户端的默认英文文案；前端应基于 code 走 i18n，description 仅作开发说明、不下发。
type Code struct {
	value       string
	message     string
	httpStatus  int
	description string
}

// allCodes 全部业务码（按声明顺序），供唯一性/完整性测试与反向查找使用。
var allCodes []Code

func defineCode(value, message string, status int, description string) Code {
	c := Code{value: value, message: message, httpStatus: status, description: description}
	allCodes = append(allCodes, c)
	return c
}

// String 返回业务码 wire 值。
func (c Code) String() string { return c.value }

// Message 返回默认英文提示。
func (c Code) Message() string { return c.message }

// HTTPStatus 返回业务码对应的 HTTP 状态码。
func (c Code) HTTPStatus() int { return c.httpStatus }

// Description 返回开发用说明。
func (c Code) Description() string { return c.description }

// MarshalText 使 Code 序列化为 JSON 字符串（如 "code": "OK"）。
func (c Code) MarshalText() ([]byte, error) { return []byte(c.value), nil }

// UnmarshalText 使 Code 可从 JSON 字符串反序列化。
func (c *Code) UnmarshalText(b []byte) error {
	if c2, ok := lookupCode(string(b)); ok {
		*c = c2
		return nil
	}
	return fmt.Errorf("unknown code %q", string(b))
}

func lookupCode(value string) (Code, bool) {
	for _, c := range allCodes {
		if c.value == value {
			return c, true
		}
	}
	return Code{}, false
}

// AllCodes 返回全部业务码（测试用）。
func AllCodes() []Code { return allCodes }

// 通用业务码
var (
	CodeOK                 = defineCode("OK", "ok", http.StatusOK, "request succeeded")
	CodeUnknown            = defineCode("UNKNOWN", "unknown error", http.StatusInternalServerError, "unclassified error")
	CodeInvalidParams      = defineCode("INVALID_PARAMS", "invalid parameters", http.StatusBadRequest, "request body/path/query validation failed")
	CodeUnauthorized       = defineCode("UNAUTHORIZED", "unauthorized", http.StatusUnauthorized, "no valid credentials provided")
	CodeForbidden          = defineCode("FORBIDDEN", "forbidden", http.StatusForbidden, "authenticated but not permitted")
	CodeNotFound           = defineCode("NOT_FOUND", "resource not found", http.StatusNotFound, "target resource does not exist")
	CodeConflict           = defineCode("CONFLICT", "resource conflict", http.StatusConflict, "resource already exists or state conflict")
	CodeInternalError      = defineCode("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError, "unexpected server-side failure")
	CodeServiceUnavailable = defineCode("SERVICE_UNAVAILABLE", "service unavailable", http.StatusServiceUnavailable, "dependency unavailable (readiness check failed)")
	// CodeSyncGuardTripped 同步保护触发：待删除/停用的数据量超过阈值，任务被取消
	CodeSyncGuardTripped = defineCode("SYNC_GUARD_TRIPPED", "sync cancelled by delete guard", http.StatusConflict, "incoming changes would disable too many records; adjust guard threshold or run a dry-run first")
	CodeTooManyRequests  = defineCode("TOO_MANY_REQUESTS", "too many requests, please try again later", http.StatusTooManyRequests, "rate limit exceeded")
)

// 认证业务码
var (
	// CodeInstitutionRequired 未指定租户且无法唯一解析（同名账号存在于多个租户）
	CodeInstitutionRequired = defineCode("INSTITUTION_REQUIRED", "institution is required for this account", http.StatusBadRequest, "identifier exists in multiple institutions; client must specify institution")
)

var (
	CodeInvalidCredentials = defineCode("INVALID_CREDENTIALS", "invalid username or password", http.StatusUnauthorized, "credentials rejected during login")
	CodeSessionExpired     = defineCode("SESSION_EXPIRED", "session expired", http.StatusUnauthorized, "session TTL elapsed")
	CodeSessionNotFound    = defineCode("SESSION_NOT_FOUND", "session not found", http.StatusUnauthorized, "session missing from store")
	CodeInvalidToken       = defineCode("INVALID_TOKEN", "invalid token", http.StatusUnauthorized, "JWT/credential token malformed or rejected")
	CodeTokenExpired       = defineCode("TOKEN_EXPIRED", "token expired", http.StatusUnauthorized, "token past its expiry")
	CodeUserDisabled       = defineCode("USER_DISABLED", "user is disabled", http.StatusForbidden, "user account is forbidden/disabled")
	CodePasswordExpired    = defineCode("PASSWORD_EXPIRED", "password expired, please change it", http.StatusForbidden, "password exceeded institution expiry policy")
	CodeMfaRequired        = defineCode("MFA_REQUIRED", "multi-factor authentication required", http.StatusUnauthorized, "login requires a second factor")
	CodeMfaInvalid         = defineCode("MFA_INVALID", "invalid multi-factor code", http.StatusUnauthorized, "second factor code rejected")
	CodeMfaNotBound        = defineCode("MFA_NOT_BOUND", "multi-factor authentication not bound", http.StatusBadRequest, "user has no bound MFA authenticator")
	CodeMfaEnrollRequired  = defineCode("MFA_ENROLL_REQUIRED", "multi-factor authentication enrollment required", http.StatusForbidden, "login method requires MFA; user must bind or verify a credential before using other APIs")
)

// 人机校验业务码
var (
	CodeCaptchaRequired = defineCode("CAPTCHA_REQUIRED", "captcha verification required", http.StatusBadRequest, "captcha token/answer missing while a captcha provider is enabled")
	CodeCaptchaInvalid  = defineCode("CAPTCHA_INVALID", "captcha verification failed", http.StatusBadRequest, "captcha answer or remote token rejected")
)

// 验证码（邮件/短信）业务码
var (
	CodeVerifyCodeInvalid   = defineCode("VERIFY_CODE_INVALID", "verification code is invalid or expired", http.StatusBadRequest, "email/sms verification code mismatch or expired")
	CodeVerifyCodeTooMany   = defineCode("VERIFY_CODE_TOO_MANY_REQUESTS", "too many verification code requests", http.StatusTooManyRequests, "verification code send rate limit exceeded")
	CodeProviderUnavailable = defineCode("PROVIDER_UNAVAILABLE", "provider unavailable", http.StatusServiceUnavailable, "提供商缺失/停用/不可达（通知、人机校验、存储、外部身份提供方共用）")
)

// 授权/权限业务码
var (
	CodePermissionDenied = defineCode("PERMISSION_DENIED", "permission denied", http.StatusForbidden, "principal lacks required permission")
	CodeRoleNotFound     = defineCode("ROLE_NOT_FOUND", "role not found", http.StatusNotFound, "role does not exist")
	CodeProtectedObject  = defineCode("PROTECTED_OBJECT", "built-in object is protected and cannot be modified or deleted", http.StatusForbidden, "built-in/seed object cannot be modified or deleted")
)

// OAuth/OIDC 业务码
var (
	CodeOAuthUnsupportedGrant = defineCode("OAUTH_UNSUPPORTED_GRANT", "unsupported grant type", http.StatusBadRequest, "grant_type is not supported by the application")
	CodeOAuthInvalidClient    = defineCode("OAUTH_INVALID_CLIENT", "invalid client", http.StatusUnauthorized, "client_id/client_secret rejected")
	CodeOAuthInvalidGrant     = defineCode("OAUTH_INVALID_GRANT", "invalid grant", http.StatusBadRequest, "authorization code/refresh token invalid, used or expired")
	CodeOAuthInvalidRedirect  = defineCode("OAUTH_INVALID_REDIRECT", "invalid redirect_uri", http.StatusBadRequest, "redirect_uri is not registered on the application")
	CodeOAuthInvalidScope     = defineCode("OAUTH_INVALID_SCOPE", "invalid scope", http.StatusBadRequest, "requested scope is not granted to the application")
	CodeOAuthProviderError    = defineCode("OAUTH_PROVIDER_ERROR", "upstream identity provider error", http.StatusBadGateway, "external identity provider exchange failed")
	CodeSSOStateInvalid       = defineCode("SSO_STATE_INVALID", "invalid or expired sso state", http.StatusBadRequest, "oauth state missing, mismatched or expired")
	CodeIdentityNotBound      = defineCode("IDENTITY_NOT_BOUND", "identity not bound to a local account", http.StatusUnauthorized, "external identity not linked and auto-provisioning disabled")
	CodeIdentityAmbiguous     = defineCode("IDENTITY_AMBIGUOUS", "multiple local accounts match this identity", http.StatusConflict, "provider identity mapping matched more than one local user")
)

// 文件上传业务码
var (
	CodeFileSizeExceeded = defineCode("FILE_SIZE_EXCEEDED", "file size exceeds limit", http.StatusRequestEntityTooLarge, "uploaded file exceeds the configured size limit")
)

// 数据库/存储业务码
var (
	CodeDBError      = defineCode("DB_ERROR", "database error", http.StatusInternalServerError, "generic database failure")
	CodeDBNotFound   = defineCode("DB_NOT_FOUND", "record not found", http.StatusNotFound, "database record missing")
	CodeDBDuplicate  = defineCode("DB_DUPLICATE", "record already exists", http.StatusConflict, "unique constraint violated")
	CodeDBConstraint = defineCode("DB_CONSTRAINT", "data constraint conflict", http.StatusConflict, "check/foreign-key constraint violated")
	CodeRedisError   = defineCode("REDIS_ERROR", "redis error", http.StatusInternalServerError, "redis operation failed")
)

// 租户/资源归属业务码
var (
	CodeInstitutionNotFound = defineCode("INSTITUTION_NOT_FOUND", "institution not found", http.StatusNotFound, "referenced institution does not exist")
	CodeUserNotFound        = defineCode("USER_NOT_FOUND", "user not found", http.StatusNotFound, "referenced user does not exist")
	CodeApplicationNotFound = defineCode("APPLICATION_NOT_FOUND", "application not found", http.StatusNotFound, "referenced application does not exist")
	CodeProviderNotFound    = defineCode("PROVIDER_NOT_FOUND", "provider not found", http.StatusNotFound, "referenced provider does not exist")
	CodeGroupNotFound       = defineCode("GROUP_NOT_FOUND", "group not found", http.StatusNotFound, "referenced group does not exist")
	CodePermissionNotFound  = defineCode("PERMISSION_NOT_FOUND", "permission not found", http.StatusNotFound, "referenced permission does not exist")
	CodeCertNotFound        = defineCode("CERT_NOT_FOUND", "certificate not found", http.StatusNotFound, "referenced certificate does not exist")
	CodeTokenNotFound       = defineCode("TOKEN_NOT_FOUND", "token not found", http.StatusNotFound, "referenced token does not exist")
)

// 应用准入业务码（见 docs/specs/261015-application-access-phase6.md）
var (
	// CodeAppAccessDenied 用户不在该应用的访问授权范围内（受限应用 + 未命中名单）。
	// 判定语义对所有人一致（含管理员）；判定作用域限定在"携带应用上下文的请求"。
	CodeAppAccessDenied = defineCode("APP_ACCESS_DENIED", "you are not authorized to access this application",
		http.StatusForbidden, "authenticated user is outside the application access scope")
)
