package server

import (
	"github.com/gin-gonic/gin"

	"polaris/internal/handler"
)

// Handlers 全部 HTTP 处理器
type Handlers struct {
	Auth        *handler.AuthHandler
	OIDC        *handler.OIDCHandler
	Health      *handler.HealthHandler
	User        *handler.UserHandler
	Institution *handler.InstitutionHandler
	Department  *handler.DepartmentHandler
	Application *handler.ApplicationHandler
	Provider    *handler.ProviderHandler
	Role        *handler.RoleHandler
	Permission  *handler.PermissionHandler
	JobTitle    *handler.JobTitleHandler
	JobLevel    *handler.JobLevelHandler
	Cert        *handler.CertHandler
	Token       *handler.TokenHandler
	Session     *handler.SessionHandler
	Record      *handler.RecordHandler
	Syncer      *handler.SyncerHandler
	Provision   *handler.ProvisioningHandler
	Connection  *handler.ConnectionHandler
	Asyncd      *handler.AsyncdHandler
	Storage     *handler.StorageHandler
}

// RegisterRoutes 注册全部路由。
// 约定：公共端点直接挂在根路径（/healthz、/.well-known/*、/oauth/*）；管理 API 挂在 /api/v1。
func RegisterRoutes(engine *gin.Engine, mw *Middleware, h *Handlers) {
	// 健康检查
	// 本地存储文件只读访问（对象存储部署下由对象存储域名提供）
	engine.GET("/static/storage/*path", h.User.ServeLocalStorage)

	engine.GET("/healthz", h.Health.Liveness)
	engine.GET("/readyz", h.Health.Readiness)

	// OIDC Provider 元数据与端点
	engine.GET("/.well-known/openid-configuration", h.OIDC.Discovery)
	engine.GET("/.well-known/jwks", h.OIDC.JWKS)
	// authorize 需要本地登录态（中间件对公开路径仍会尝试解析会话，未登录则由 handler 拒绝）
	engine.GET("/oauth/authorize", mw.Auth(), h.OIDC.Authorize)
	engine.POST("/oauth/token", h.OIDC.Token)
	engine.GET("/oauth/userinfo", h.OIDC.UserInfo)
	engine.POST("/oauth/userinfo", h.OIDC.UserInfo)
	engine.POST("/oauth/introspect", h.OIDC.Introspect)
	engine.GET("/oauth/logout", h.OIDC.EndSession)
	engine.POST("/oauth/logout", h.OIDC.EndSession)

	v1 := engine.Group("/api/v1")
	authGroup := v1.Group("", mw.Auth())

	// 认证（公开端点：登录 / 注册 / SSO 回调）
	publicAuth := v1.Group("/auth")
	publicAuth.POST("/login", h.Auth.Login)
	publicAuth.POST("/signup", h.Auth.Signup)
	publicAuth.GET("/status", h.Auth.Status)
	publicAuth.GET("/providers", h.Auth.Providers)
	publicAuth.GET("/institutions", h.Auth.Institutions)
	publicAuth.GET("/app-info", h.Auth.AppInfo)
	publicAuth.GET("/captcha", h.Auth.Captcha)
	publicAuth.POST("/codes", h.Auth.SendCode)
	publicAuth.POST("/login/code", h.Auth.CodeLogin)
	publicAuth.POST("/magic-link", h.Auth.SendMagicLink)
	publicAuth.POST("/magic-link/verify", h.Auth.VerifyMagicLink)
	// 默认回调地址是 /sso/callback，前端不知道也不应该猜 provider；provider 在 state 载荷里。
	// 保留带 :provider 的旧路由兼容已有客户端。
	publicAuth.GET("/oauth/callback", h.Auth.OAuthCallback)
	publicAuth.POST("/oauth/callback", h.Auth.OAuthCallback)
	publicAuth.GET("/oauth/:provider/callback", h.Auth.OAuthCallback)
	publicAuth.POST("/oauth/:provider/callback", h.Auth.OAuthCallback)

	// 认证
	authGroup.POST("/auth/logout", h.Auth.Logout)
	authGroup.GET("/auth/app-access", h.Auth.AppAccess)
	authGroup.GET("/auth/oauth/:provider/authorize", h.Auth.OAuthAuthorize)
	authGroup.GET("/auth/mfa/options", h.Auth.MfaOptions)
	authGroup.POST("/auth/mfa/enroll", h.Auth.MfaEnroll)
	authGroup.POST("/auth/mfa/enroll/resend", h.Auth.MfaResend)
	authGroup.POST("/auth/mfa/enroll/verify", h.Auth.MfaEnrollVerify)
	authGroup.POST("/auth/mfa/verify", h.Auth.MfaVerify)
	authGroup.POST("/auth/mfa/skip", h.Auth.MfaSkip)
	authGroup.POST("/auth/mfa/disable", h.Auth.MfaDisable)
	authGroup.POST("/auth/change-password", h.Auth.ChangePassword)
	authGroup.GET("/me", h.User.Me)
	authGroup.POST("/me/avatar", h.User.UploadAvatar)
	authGroup.GET("/me/identities", h.User.MyIdentities)
	authGroup.DELETE("/me/identities/:identityID", h.User.UnbindMyIdentity)
	authGroup.POST("/me/identities/:provider/authorize", h.Auth.AuthorizeIdentityLink)
	authGroup.GET("/me/sessions", h.Session.MySessions)
	authGroup.DELETE("/me/sessions/:sessionID", h.Session.RevokeMySession)
	authGroup.POST("/me/sessions/revoke-all", h.Session.RevokeMySessions)
	authGroup.GET("/me/records", h.Record.MyList)

	// 组织
	org := authGroup.Group("/institutions")
	org.GET("", h.Institution.List)
	org.POST("", mw.RequireAdmin(), h.Institution.Create)
	org.GET("/:id", h.Institution.Get)
	org.GET("/:id/stats", h.Institution.Stats)
	org.GET("/stats", h.Institution.StatsAll)
	org.PUT("/:id", mw.RequireAdmin(), h.Institution.Update)
	org.DELETE("/:id", mw.RequireAdmin(), h.Institution.Delete)

	// 部门（租户内的部门树；仅用于用户归属与筛选）
	depts := authGroup.Group("/departments")
	depts.GET("", mw.RequirePermission("department", "read"), h.Department.List)
	depts.POST("", mw.RequirePermission("department", "create"), h.Department.Create)
	depts.GET("/:id", mw.RequirePermission("department", "read"), h.Department.Get)
	depts.GET("/:id/members", mw.RequirePermission("department", "read"), h.Department.Members)
	depts.PUT("/:id", mw.RequirePermission("department", "update"), h.Department.Update)
	depts.POST("/:id/reorder", mw.RequirePermission("department", "update"), h.Department.Reorder)
	depts.DELETE("/:id", mw.RequirePermission("department", "delete"), h.Department.Delete)

	// 用户
	users := authGroup.Group("/users")
	users.GET("", mw.RequirePermission("user", "read"), h.User.List)
	users.POST("", mw.RequirePermission("user", "create"), h.User.Create)
	users.GET("/:id", mw.RequirePermission("user", "read"), h.User.Get)
	// 资料部分更新（只改提交的字段；凭证与状态字段各有专用端点）
	users.PUT("/:id", mw.RequirePermission("user", "update"), h.User.UpdateProfile)
	// 部门归属（多值，Spec 第六阶段 Q2 = A）与职级（仅属性，Q4.1 = A）
	users.PUT("/:id/departments", mw.RequirePermission("user", "update"), h.User.SetDepartments)
	users.PUT("/:id/job-level", mw.RequirePermission("user", "update"), h.User.SetJobLevel)
	// 设为 / 取消管理员（危险动作，服务层有"最后一个管理员"防呆）
	users.PUT("/:id/admin", mw.RequirePermission("user", "update"), h.User.SetAdmin)
	users.POST("/:id/disable", mw.RequirePermission("user", "update"), h.User.Disable)
	users.POST("/:id/enable", mw.RequirePermission("user", "update"), h.User.Enable)
	users.DELETE("/:id", mw.RequirePermission("user", "delete"), h.User.Delete)
	users.POST("/:id/reset-password", mw.RequirePermission("user", "update"), h.User.ResetPassword)
	// 重置 MFA 是唯一能"降级第二因子"的动作 → 独立权限，不与 user:update 混同
	users.POST("/:id/reset-mfa", mw.RequirePermission("user", "reset_mfa"), h.User.ResetMFA)
	// 解除登录失败锁定（客服/Helpdesk 场景：能解锁但不该能改资料）
	users.POST("/:id/unlock", mw.RequirePermission("user", "unlock"), h.User.Unlock)
	users.GET("/:id/security", mw.RequirePermission("user", "read"), h.User.Security)
	users.GET("/:id/authorizations", mw.RequirePermission("user", "read"), h.User.Authorizations)
	users.PUT("/:id/roles", mw.RequirePermission("role", "update"), h.User.SetRoles)
	users.PUT("/:id/job-titles", mw.RequirePermission("user", "update"), h.User.SetJobTitles)
	users.GET("/:id/application-accounts", mw.RequirePermission("application_account", "read"), h.User.ApplicationAccounts)
	users.GET("/:id/provision-history", mw.RequirePermission("provision_history", "read"), h.User.ProvisionHistory)
	users.GET("/:id/records", mw.RequirePermission("record", "read"), h.User.Records)
	users.GET("/:id/external-identities", mw.RequirePermission("user", "read"), h.User.ExternalIdentities)
	users.GET("/:id/identities", mw.RequirePermission("user", "read"), h.User.Identities)
	users.DELETE("/:id/identities/:identityID", mw.RequirePermission("user", "update"), h.User.UnbindIdentity)
	users.POST("/:id/api-keys", mw.RequirePermission("user", "update"), h.User.IssueAPIKey)

	// 应用
	apps := authGroup.Group("/applications")
	apps.GET("", mw.RequirePermission("application", "read"), h.Application.List)
	apps.POST("", mw.RequirePermission("application", "create"), h.Application.Create)
	apps.GET("/:id", mw.RequirePermission("application", "read"), h.Application.Get)
	apps.PUT("/:id", mw.RequirePermission("application", "update"), h.Application.Update)
	apps.DELETE("/:id", mw.RequirePermission("application", "delete"), h.Application.Delete)
	apps.POST("/:id/reset-secret", mw.RequirePermission("application", "update"), h.Application.ResetSecret)
	// 应用访问授权（谁能使用该应用）：读取随 application:read，写入随 application:update
	apps.GET("/:id/access", mw.RequirePermission("application", "read"), h.Application.Access)
	apps.PUT("/:id/access", mw.RequirePermission("application", "update"), h.Application.SetAccess)
	// 可授权主体候选：必须有独立端点，否则只有 application:read 的应用管理员选不出人；
	// 作用域是组织（新建应用时还没有主键，候选必须先能取到）
	apps.GET("/subject-options", mw.RequirePermission("application", "read"), h.Application.SubjectOptions)

	// 身份提供方
	providers := authGroup.Group("/providers")
	providers.GET("/catalog", mw.RequirePermission("provider", "read"), h.Provider.Categories)
	providers.GET("", mw.RequirePermission("provider", "read"), h.Provider.List)
	providers.POST("", mw.RequirePermission("provider", "create"), h.Provider.Create)
	providers.GET("/:id", mw.RequirePermission("provider", "read"), h.Provider.Get)
	providers.PUT("/:id", mw.RequirePermission("provider", "update"), h.Provider.Update)
	providers.DELETE("/:id", mw.RequirePermission("provider", "delete"), h.Provider.Delete)
	providers.POST("/:id/test", mw.RequirePermission("provider", "update"), h.Provider.Test)
	providers.POST("/:id/state", mw.RequirePermission("provider", "update"), h.Provider.SetState)
	providers.GET("/:id/message-templates", mw.RequirePermission("provider", "read"), h.Provider.Templates)
	providers.POST("/:id/message-templates", mw.RequirePermission("provider", "update"), h.Provider.CreateTemplate)
	providers.GET("/:id/message-templates/:templateId", mw.RequirePermission("provider", "read"), h.Provider.GetTemplate)
	providers.PUT("/:id/message-templates/:templateId", mw.RequirePermission("provider", "update"), h.Provider.UpdateTemplate)
	providers.DELETE("/:id/message-templates/:templateId", mw.RequirePermission("provider", "delete"), h.Provider.DeleteTemplate)

	// 角色
	roles := authGroup.Group("/roles")
	roles.GET("", mw.RequirePermission("role", "read"), h.Role.List)
	roles.POST("", mw.RequirePermission("role", "create"), h.Role.Create)
	roles.GET("/:id", mw.RequirePermission("role", "read"), h.Role.Get)
	roles.PUT("/:id", mw.RequirePermission("role", "update"), h.Role.Update)
	roles.PUT("/:id/relations", mw.RequirePermission("role", "update"), h.Role.SetRelations)
	roles.DELETE("/:id", mw.RequirePermission("role", "delete"), h.Role.Delete)

	// 权限
	perms := authGroup.Group("/permissions")
	perms.GET("", mw.RequirePermission("permission", "read"), h.Permission.List)
	perms.POST("", mw.RequirePermission("permission", "create"), h.Permission.Create)
	perms.GET("/:id", mw.RequirePermission("permission", "read"), h.Permission.Get)
	perms.PUT("/:id", mw.RequirePermission("permission", "update"), h.Permission.Update)
	perms.PUT("/:id/subjects", mw.RequirePermission("permission", "update"), h.Permission.SetSubjects)
	perms.DELETE("/:id", mw.RequirePermission("permission", "delete"), h.Permission.Delete)
	perms.POST("/enforce", mw.RequirePermission("permission", "read"), h.Permission.Enforce)

	// 职务（原「用户组」；Spec 第六阶段 Q3 = A。权限资源名同步为 job_title）
	jobTitles := authGroup.Group("/job-titles")
	jobTitles.GET("", mw.RequirePermission("job_title", "read"), h.JobTitle.List)
	jobTitles.POST("", mw.RequirePermission("job_title", "create"), h.JobTitle.Create)
	jobTitles.GET("/:id", mw.RequirePermission("job_title", "read"), h.JobTitle.Get)
	jobTitles.PUT("/:id", mw.RequirePermission("job_title", "update"), h.JobTitle.Update)
	jobTitles.PUT("/:id/members", mw.RequirePermission("job_title", "update"), h.JobTitle.SetMembers)
	// 职务详情：任职人员明细 + 关联角色 + 汇总权限（"这个职务到底授了什么权"）
	jobTitles.GET("/:id/detail", mw.RequirePermission("job_title", "read"), h.JobTitle.Detail)
	jobTitles.DELETE("/:id", mw.RequirePermission("job_title", "delete"), h.JobTitle.Delete)

	// 职级（Spec 第六阶段 Q4：仅作用户属性，不参与授权）
	jobLevels := authGroup.Group("/job-levels")
	jobLevels.GET("", mw.RequirePermission("job_level", "read"), h.JobLevel.List)
	jobLevels.GET("/distribution", mw.RequirePermission("job_level", "read"), h.JobLevel.Distribution)
	jobLevels.POST("", mw.RequirePermission("job_level", "create"), h.JobLevel.Create)
	jobLevels.GET("/:id", mw.RequirePermission("job_level", "read"), h.JobLevel.Get)
	jobLevels.PUT("/:id", mw.RequirePermission("job_level", "update"), h.JobLevel.Update)
	jobLevels.DELETE("/:id", mw.RequirePermission("job_level", "delete"), h.JobLevel.Delete)

	// 证书
	certs := authGroup.Group("/certs")
	certs.GET("", mw.RequirePermission("cert", "read"), h.Cert.List)
	certs.POST("", mw.RequirePermission("cert", "create"), h.Cert.Create)
	certs.GET("/:id", mw.RequirePermission("cert", "read"), h.Cert.Get)
	certs.PUT("/:id", mw.RequirePermission("cert", "update"), h.Cert.Update)
	certs.DELETE("/:id", mw.RequirePermission("cert", "delete"), h.Cert.Delete)

	// 令牌
	tokens := authGroup.Group("/tokens")
	tokens.GET("", mw.RequirePermission("token", "read"), h.Token.List)
	tokens.GET("/:id", mw.RequirePermission("token", "read"), h.Token.Get)
	tokens.POST("/:id/revoke", mw.RequirePermission("token", "update"), h.Token.Revoke)
	tokens.DELETE("/:id", mw.RequirePermission("token", "delete"), h.Token.Delete)

	// 会话
	sessions := authGroup.Group("/sessions")
	sessions.GET("", mw.RequirePermission("session", "read"), h.Session.ListByUser)
	sessions.DELETE("/:sessionID", mw.RequirePermission("session", "delete"), h.Session.Revoke)
	sessions.POST("/revoke-all", mw.RequirePermission("session", "delete"), h.Session.RevokeAll)

	// 审计记录
	records := authGroup.Group("/records")
	records.GET("", mw.RequirePermission("record", "read"), h.Record.List)
	records.GET("/:id", mw.RequirePermission("record", "read"), h.Record.Get)

	// 身份连接：类型目录（入/出方向类型的单一来源，同时服务于 syncer 与 connector）
	connections := authGroup.Group("/connections")
	connections.GET("/catalog", mw.RequireAnyPermission("read", "syncer", "connector"), h.Connection.Catalog)

	// 外部用户数据源
	syncers := authGroup.Group("/syncers")
	syncers.GET("/types", mw.RequirePermission("syncer", "read"), h.Syncer.Types)
	syncers.GET("", mw.RequirePermission("syncer", "read"), h.Syncer.List)
	syncers.POST("", mw.RequirePermission("syncer", "create"), h.Syncer.Create)
	syncers.GET("/:id", mw.RequirePermission("syncer", "read"), h.Syncer.Get)
	syncers.PUT("/:id", mw.RequirePermission("syncer", "update"), h.Syncer.Update)
	syncers.DELETE("/:id", mw.RequirePermission("syncer", "delete"), h.Syncer.Delete)
	syncers.POST("/:id/enabled", mw.RequirePermission("syncer", "update"), h.Syncer.SetEnabled)
	syncers.POST("/:id/run", mw.RequirePermission("syncer", "update"), h.Syncer.Run)
	syncers.POST("/:id/dry-run", mw.RequirePermission("syncer", "update"), h.Syncer.DryRun)
	syncers.GET("/:id/runs", mw.RequirePermission("syncer", "read"), h.Syncer.Runs)

	// 下游连接器（推送型供给）
	connectors := authGroup.Group("/connectors")
	connectors.GET("/types", mw.RequirePermission("connector", "read"), h.Provision.ConnectorTypes)
	connectors.GET("", mw.RequirePermission("connector", "read"), h.Provision.ListConnectors)
	connectors.POST("", mw.RequirePermission("connector", "create"), h.Provision.CreateConnector)
	connectors.GET("/:id", mw.RequirePermission("connector", "read"), h.Provision.GetConnector)
	connectors.PUT("/:id", mw.RequirePermission("connector", "update"), h.Provision.UpdateConnector)
	connectors.DELETE("/:id", mw.RequirePermission("connector", "delete"), h.Provision.DeleteConnector)
	connectors.POST("/:id/test", mw.RequirePermission("connector", "update"), h.Provision.TestConnector)
	connectors.POST("/:id/reconcile", mw.RequirePermission("connector", "update"), h.Provision.ReconcileConnector)

	// 账号策略
	strategies := authGroup.Group("/account-strategies")
	strategies.GET("", mw.RequirePermission("account_strategy", "read"), h.Provision.ListStrategies)
	strategies.POST("", mw.RequirePermission("account_strategy", "create"), h.Provision.CreateStrategy)
	strategies.GET("/:id", mw.RequirePermission("account_strategy", "read"), h.Provision.GetStrategy)
	strategies.PUT("/:id", mw.RequirePermission("account_strategy", "update"), h.Provision.UpdateStrategy)
	strategies.DELETE("/:id", mw.RequirePermission("account_strategy", "delete"), h.Provision.DeleteStrategy)

	// 应用账号
	applicationAccounts := authGroup.Group("/application-accounts")
	applicationAccounts.GET("", mw.RequirePermission("application_account", "read"), h.Provision.ListApplicationAccounts)
	applicationAccounts.POST("", mw.RequirePermission("application_account", "create"), h.Provision.CreateApplicationAccount)
	applicationAccounts.POST("/retry", mw.RequirePermission("application_account", "update"), h.Provision.RetryApplicationAccount)
	applicationAccounts.DELETE("", mw.RequirePermission("application_account", "delete"), h.Provision.DeleteApplicationAccount)
	// 门户"我的应用账号"（当前用户自己的账号，只读）
	authGroup.GET("/me/application-accounts", h.Provision.MyApplicationAccounts)
	authGroup.GET("/me/applications", h.Provision.MyApplications)

	// 供给历史
	provisioning := authGroup.Group("/provisioning")
	provisioning.GET("", mw.RequirePermission("provision_history", "read"), h.Provision.ListHistory)

	// 系统级设置（平台管理员）：默认对象存储
	system := authGroup.Group("/system")
	system.GET("/storage", mw.RequireAdmin(), h.Storage.GetSettings)
	system.PUT("/storage", mw.RequireAdmin(), h.Storage.UpdateSettings)
	system.GET("/oauth", mw.RequireAdmin(), h.Auth.OAuthInfo)

	// asyncd：任务定义 / 执行实例 / 执行记录 / 执行器实例（平台级运维可见性 + 人工干预）。
	// 路径约定：资源标识统一用数字主键；执行实例嵌套在任务定义下（归属由路径表达）。
	asyncdGroup := authGroup.Group("/asyncd")
	asyncdGroup.GET("/jobs", mw.RequireAdmin(), h.Asyncd.Jobs)
	asyncdGroup.PUT("/jobs/:id", mw.RequireAdmin(), h.Asyncd.UpdateJob)
	asyncdGroup.GET("/jobs/:id/runs", mw.RequireAdmin(), h.Asyncd.JobRuns)
	asyncdGroup.GET("/jobs/:id/runs/:runId/attempts", mw.RequireAdmin(), h.Asyncd.JobRunAttempts)
	asyncdGroup.POST("/jobs/:id/runs/:runId/retry", mw.RequireAdmin(), h.Asyncd.RetryJobRun)
	asyncdGroup.GET("/workers", mw.RequireAdmin(), h.Asyncd.Workers)
	system.POST("/storage/test", mw.RequireAdmin(), h.Storage.TestSettings)

	// 通用上传：写入当前组织的存储后端，返回可访问 URL（受保护路由，按登录用户组织收敛）
	authGroup.POST("/upload", h.Storage.Upload)
}
