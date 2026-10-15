package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
)

const (
	// AuthHeaderKey 认证头键名
	AuthHeaderKey = "Authorization"
	// AuthContextKey 认证信息在 gin context 中的键名
	AuthContextKey = "auth_context"
	// TokenCookieName 会话 Cookie 名称
	TokenCookieName = "polaris_session"
	// MfaTrustCookieName 「信任设备（记住此设备）」Cookie 名称
	MfaTrustCookieName = "polaris_mfa_trust"
)

// Middleware 认证中间件。
// 认证入口按 Scheme 分流：Cookie → Session；Bearer → JWT 会话（回退到 API access token）。
type Middleware struct {
	jwtManager     *JWTManager
	sessionManager *SessionManager
	users          repository.UserRepository
	tokens         repository.TokenRepository
	publicPaths    map[string]bool
	allowedOrigins []string
}

// NewMiddleware 创建认证中间件。
// allowedOrigins 用于写操作的 CSRF Origin 校验（为空时跳过校验，仅限开发）。
func NewMiddleware(
	jwtManager *JWTManager,
	sessionManager *SessionManager,
	users repository.UserRepository,
	tokens repository.TokenRepository,
	allowedOrigins []string,
) *Middleware {
	return &Middleware{
		jwtManager:     jwtManager,
		sessionManager: sessionManager,
		users:          users,
		tokens:         tokens,
		publicPaths:    make(map[string]bool),
		allowedOrigins: allowedOrigins,
	}
}

// AddPublicPath 添加公开路径（无需认证；以 /* 结尾表示前缀匹配）
func (m *Middleware) AddPublicPath(path string) { m.publicPaths[path] = true }

// IsPublicPath 检查是否为公开路径
func (m *Middleware) IsPublicPath(path string) bool {
	if m.publicPaths[path] {
		return true
	}
	for p := range m.publicPaths {
		if strings.HasSuffix(p, "/*") {
			prefix := strings.TrimSuffix(p, "/*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

// Handle gin 中间件处理函数
func (m *Middleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.IsPublicPath(c.Request.URL.Path) {
			// 公开路径也尝试解析会话（便于登录页展示/审计）
			if authCtx := m.tryResolve(c); authCtx != nil {
				c.Set(AuthContextKey, authCtx)
			}
			c.Next()
			return
		}

		// CSRF Origin 校验：仅对依赖 Cookie 环境凭证的写请求生效；
		// 显式携带 Authorization 的请求无环境凭证，不适用 CSRF。
		if !isSafeMethod(c.Request.Method) && c.GetHeader(AuthHeaderKey) == "" && !m.originAllowed(c) {
			logger.Warn("csrf origin rejected",
				"origin", c.Request.Header.Get("Origin"),
				"method", c.Request.Method,
				"path", c.Request.URL.Path)
			bizerrors.AbortError(c, bizerrors.ErrForbidden.WithDetail("reason", "origin_not_allowed"))
			return
		}

		authCtx := m.tryResolve(c)
		if authCtx == nil {
			bizerrors.AbortError(c, bizerrors.ErrUnauthorized)
			return
		}

		// 组织强制 MFA 但用户尚未绑定时：只放行绑定 MFA / 登出 / 身份查询等必要接口
		if authCtx.MfaEnrollRequired && !mfaEnrollAllowedPath(c.Request.URL.Path) {
			bizerrors.AbortError(c, bizerrors.ErrMfaEnrollRequired)
			return
		}

		c.Set(AuthContextKey, authCtx)
		c.Set("user_id", authCtx.UserID)
		c.Set("institutionID", authCtx.InstitutionID)
		c.Set("user_name", authCtx.Name)
		c.Set("session_id", authCtx.SessionID)
		c.Next()
	}
}

// tryResolve 尝试解析请求身份；失败返回 nil（不写响应）。
func (m *Middleware) tryResolve(c *gin.Context) *AuthContext {
	if header := c.GetHeader(AuthHeaderKey); header != "" {
		scheme, credential, ok := parseAuthorization(header)
		if !ok {
			return nil
		}
		if strings.EqualFold(scheme, "bearer") {
			return m.resolveBearer(c, credential)
		}
		return nil
	}
	token, err := c.Cookie(TokenCookieName)
	if err != nil || token == "" {
		logger.Debug("session cookie missing", "cookie", TokenCookieName, "err", err, "present", c.Request.Header.Get("Cookie") != "")
		return nil
	}
	authCtx := m.resolveJWT(c, token)
	if authCtx == nil {
		logger.Debug("session cookie rejected", "cookie", TokenCookieName)
	}
	return authCtx
}

// resolveBearer 解析 Bearer：优先 JWT 会话；失败则按 API access token 处理。
func (m *Middleware) resolveBearer(c *gin.Context, credential string) *AuthContext {
	if authCtx := m.resolveJWT(c, credential); authCtx != nil {
		return authCtx
	}
	if m.tokens == nil {
		return nil
	}
	record, err := m.tokens.GetByAccessToken(c.Request.Context(), credential)
	if err != nil || record.Revoked {
		return nil
	}
	if record.ExpireTime != nil && record.ExpireTime.Before(nowTime()) {
		return nil
	}
	return m.buildContextFromUser(c, record.InstitutionID, record.User, "")
}

// resolveJWT 解析会话 JWT（仅含 session id）并据此读取 Redis 会话。
func (m *Middleware) resolveJWT(c *gin.Context, token string) *AuthContext {
	claims, err := m.jwtManager.Parse(token)
	if err != nil {
		return nil
	}
	return m.resolveSession(c, claims.SessionID)
}

// resolveSession 通过 session ID 读取 Redis 会话并构造上下文。
func (m *Middleware) resolveSession(c *gin.Context, sessionID string) *AuthContext {
	session, err := m.sessionManager.Load(c.Request.Context(), sessionID)
	if err != nil {
		return nil
	}
	return &AuthContext{
		SessionID:         session.ID,
		UserID:            session.UserID,
		InstitutionID:     session.InstitutionID,
		Name:              session.Name,
		DisplayName:       session.DisplayName,
		Email:             session.Email,
		IsAdmin:           session.IsAdmin,
		Roles:             session.Roles,
		Permissions:       session.Permissions,
		ExpiresAt:         session.ExpiresAt,
		MfaEnrollRequired: session.MfaEnrollRequired,
	}
}

// buildContextFromUser 由用户身份直接构造上下文（API token 场景，权限即时计算）。
func (m *Middleware) buildContextFromUser(c *gin.Context, institutionID int64, name, sessionID string) *AuthContext {
	ctx := c.Request.Context()
	user, err := m.users.GetByName(ctx, institutionID, name)
	if err != nil || user.IsForbidden || user.IsDeleted {
		return nil
	}
	roles, err := m.sessionManager.RolesForUser(ctx, institutionID, name)
	if err != nil {
		roles = nil
	}
	perms, err := m.sessionManager.EffectivePermissions(ctx, institutionID, name)
	if err != nil {
		perms = nil
	}
	return &AuthContext{
		SessionID:     sessionID,
		UserID:        user.ID,
		InstitutionID: institutionID,
		Name:          user.Name,
		DisplayName:   user.DisplayName,
		Email:         user.Email,
		IsAdmin:       user.IsAdmin,
		Roles:         roles,
		Permissions:   perms,
	}
}

// RequirePermission 要求特定资源/动作权限的中间件。
func (m *Middleware) RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx := GetAuthContext(c)
		if !authCtx.IsAuthenticated() {
			bizerrors.AbortError(c, bizerrors.ErrUnauthorized)
			return
		}
		if !authCtx.Enforce(resource, action) {
			logger.Warn("permission denied", "user", authCtx.Name, "resource", resource, "action", action)
			bizerrors.AbortError(c, bizerrors.ErrPermissionDenied.WithDetail("resource", resource).WithDetail("action", action))
			return
		}
		c.Next()
	}
}

// RequireAnyPermission 要求"任一资源具备该动作"的中间件。
// 用于跨资源聚合端点（如「身份连接」目录同时服务于 syncer 与 connector 两个资源）。
func (m *Middleware) RequireAnyPermission(action string, resources ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx := GetAuthContext(c)
		if !authCtx.IsAuthenticated() {
			bizerrors.AbortError(c, bizerrors.ErrUnauthorized)
			return
		}
		if !anyPermissionAllowed(authCtx, action, resources) {
			logger.Warn("permission denied", "user", authCtx.Name, "action", action, "resources", resources)
			bizerrors.AbortError(c, bizerrors.ErrPermissionDenied.
				WithDetail("action", action).WithDetail("resources", strings.Join(resources, ",")))
			return
		}
		c.Next()
	}
}

// anyPermissionAllowed 任一资源具备该动作即放行（Deny 优先与管理员直通语义由 Enforce 保证）。
func anyPermissionAllowed(authCtx *AuthContext, action string, resources []string) bool {
	for _, resource := range resources {
		if authCtx.Enforce(resource, action) {
			return true
		}
	}
	return false
}

// RequireAdmin 要求管理员权限的中间件。
func (m *Middleware) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx := GetAuthContext(c)
		if !authCtx.IsAuthenticated() {
			bizerrors.AbortError(c, bizerrors.ErrUnauthorized)
			return
		}
		if !authCtx.IsAdmin && !authCtx.Enforce("*", "*") {
			bizerrors.AbortError(c, bizerrors.ErrPermissionDenied)
			return
		}
		c.Next()
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

// originAllowed 校验请求 Origin 是否在白名单内。无 Origin 视为允许；白名单为空表示跳过校验。
func (m *Middleware) originAllowed(c *gin.Context) bool {
	origin := c.Request.Header.Get("Origin")
	if origin == "" || len(m.allowedOrigins) == 0 {
		return true
	}
	for _, allowed := range m.allowedOrigins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// parseAuthorization 按 Scheme 解析 Authorization header（RFC 7235）
func parseAuthorization(header string) (scheme, credential string, ok bool) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

// GetAuthContext 从 gin context 获取认证信息
func GetAuthContext(c *gin.Context) *AuthContext {
	if val, exists := c.Get(AuthContextKey); exists {
		if authCtx, ok := val.(*AuthContext); ok {
			return authCtx
		}
	}
	return nil
}

// GetInstitution 获取当前组织
func GetInstitution(c *gin.Context) int64 {
	if authCtx := GetAuthContext(c); authCtx != nil {
		return authCtx.InstitutionID
	}
	return 0
}

// GetUserName 获取当前用户
func GetUserName(c *gin.Context) string {
	if authCtx := GetAuthContext(c); authCtx != nil {
		return authCtx.Name
	}
	return ""
}

// mfaEnrollAllowedPath 待绑定 MFA 的受限会话允许访问的路径（绑定 MFA / 登出 / 状态查询）。
func mfaEnrollAllowedPath(path string) bool {
	switch path {
	case "/api/v1/auth/mfa/setup", "/api/v1/auth/mfa/verify",
		"/api/v1/auth/logout", "/api/v1/auth/status", "/api/v1/me":
		return true
	}
	return false
}
