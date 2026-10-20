package server

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	"polaris/internal/auth"
	"polaris/internal/config"
	"polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/requestmeta"
	"polaris/internal/tracing"
)

// Middleware 中间件集合
type Middleware struct {
	cfg            *config.Config
	authMiddleware *auth.Middleware
}

// NewMiddleware 创建中间件集合
func NewMiddleware(cfg *config.Config, authMiddleware *auth.Middleware) *Middleware {
	return &Middleware{cfg: cfg, authMiddleware: authMiddleware}
}

// Register 注册全局中间件（按顺序）
func (m *Middleware) Register(engine *gin.Engine) {
	engine.Use(m.Recovery())        // 1. panic 恢复
	engine.Use(m.Tracing())         // 2. 链路追踪
	engine.Use(m.TraceHeaders())    // 3. 响应头注入 trace id
	engine.Use(m.RequestMetadata()) // 4. 请求元数据（审计来源）
	engine.Use(m.Logger())          // 5. 访问日志
	engine.Use(m.CORS())            // 6. 跨域
	engine.Use(errors.Middleware()) // 7. 统一响应（handler 之后渲染业务错误）
}

// Recovery panic 恢复中间件
func (m *Middleware) Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(errors.RecoveryFunc)
}

// Tracing 链路追踪中间件
func (m *Middleware) Tracing() gin.HandlerFunc {
	return otelgin.Middleware(m.cfg.App.Name)
}

// TraceHeaders 将 trace_id 以品牌化请求 ID 写入响应头
func (m *Middleware) TraceHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsValid() {
			c.Writer.Header().Set("X-Polaris-Request-Id", sc.TraceID().String())
		}
		c.Next()
	}
}

// RequestMetadata attaches client IP, method and URI to the request context.
// AuditRecorder reads them when it writes service-level audit records.
func (m *Middleware) RequestMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		info := requestmeta.Info{
			ClientIP:   c.ClientIP(),
			Method:     c.Request.Method,
			RequestURI: c.Request.URL.RequestURI(),
		}
		c.Request = c.Request.WithContext(requestmeta.With(c.Request.Context(), info))
		c.Next()
	}
}

// Logger 访问日志中间件
func (m *Middleware) Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		c.Next()
		if raw != "" {
			path = path + "?" + raw
		}
		logger.Info("http request",
			"trace_id", tracing.TraceID(c.Request.Context()),
			"span_id", tracing.SpanID(c.Request.Context()),
			"client_ip", c.ClientIP(),
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency", time.Since(start).String(),
			"error", c.Errors.String(),
		)
	}
}

// CORS 跨域中间件（仅控制"读"；写操作的 Origin 校验由 CSRF 白名单独立控制）
func (m *Middleware) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowOrigin := "*"
		if len(m.cfg.CORS.AllowOrigins) > 0 && m.cfg.CORS.AllowOrigins[0] != "*" {
			for _, allowed := range m.cfg.CORS.AllowOrigins {
				if strings.EqualFold(allowed, origin) {
					allowOrigin = origin
					break
				}
			}
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		if allowOrigin != "*" {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(m.cfg.CORS.AllowHeaders, ", "))
		c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(m.cfg.CORS.AllowMethods, ", "))
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Polaris-Request-Id")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// Auth 认证中间件
func (m *Middleware) Auth() gin.HandlerFunc { return m.authMiddleware.Handle() }

// RequirePermission 权限检查中间件
func (m *Middleware) RequirePermission(resource, action string) gin.HandlerFunc {
	return m.authMiddleware.RequirePermission(resource, action)
}

// RequireAdmin 管理员检查中间件
// RequireAnyPermission 要求任一资源具备该动作（跨资源聚合端点用）
func (m *Middleware) RequireAnyPermission(action string, resources ...string) gin.HandlerFunc {
	return m.authMiddleware.RequireAnyPermission(action, resources...)
}

func (m *Middleware) RequireAdmin() gin.HandlerFunc { return m.authMiddleware.RequireAdmin() }
