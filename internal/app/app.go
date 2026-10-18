// Package app 负责应用组装与依赖注入（唯一装配点，禁止业务包级单例）。
package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	"polaris/internal/config"
	"polaris/internal/handler"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/repository/postgres"
	redisrepo "polaris/internal/repository/redis"
	"polaris/internal/server"
	"polaris/internal/service"
	"polaris/internal/service/asyncd"
	providerpkg "polaris/internal/service/provider"
	"polaris/internal/tracing"
	"polaris/pkg/dbclient"
	"polaris/pkg/migrator"
	"polaris/pkg/redisclient"
)

// DefaultShutdownTimeout 默认关闭超时
const DefaultShutdownTimeout = 30 * time.Second

// Application 应用结构
type Application struct {
	cfg           *config.Config
	configManager *config.Manager

	db    *dbclient.Client
	redis *redisclient.Client

	// 仓库层
	orgRepo                *postgres.InstitutionRepository
	deptRepo               *postgres.DepartmentRepository
	userRepo               *postgres.UserRepository
	appRepo                *postgres.ApplicationRepository
	appSubjectRepo         *postgres.ApplicationSubjectRepository
	providerRepo           *postgres.ProviderRepository
	storageSettingsRepo    *postgres.StorageSettingsRepository
	roleRepo               *postgres.RoleRepository
	permRepo               *postgres.PermissionRepository
	jobTitleRepo           *postgres.JobTitleRepository
	jobLevelRepo           *postgres.JobLevelRepository
	userDeptRepo           *postgres.UserDepartmentRepository
	linkRepo               *postgres.SyncerLinkRepository
	certRepo               *postgres.CertRepository
	tokenRepo              *postgres.TokenRepository
	recordRepo             *postgres.RecordRepository
	syncerRepo             *postgres.SyncerRepository
	connRepo               *postgres.ConnectorRepository
	strategyRepo           *postgres.AccountStrategyRepository
	applicationAccountRepo *postgres.ApplicationAccountRepository
	provisionRepo          *postgres.ProvisionHistoryRepository
	sessionRepo            *redisrepo.SessionRepository
	rateLimitRepo          *redisrepo.RateLimitRepository
	stateStore             *redisrepo.StateStore

	// 认证组件
	jwtManager     *auth.JWTManager
	secretCipher   *auth.SecretCipher
	sessionManager *auth.SessionManager
	// providerRuntime / oauthResolver：提供商（含外部身份提供方）的运行时读取与连接器解析
	providerRuntime *providerpkg.Runtime
	oauthResolver   *providerpkg.Resolver
	authMiddleware  *auth.Middleware
	issuer          *oidc.Issuer

	// 服务层
	audits         *service.AuditRecorder
	institutionSvc *service.InstitutionService
	departmentSvc  *service.DepartmentService
	userSvc        *service.UserService
	lockoutSvc     *service.LoginLockoutService
	applicationSvc *service.ApplicationService
	appAccessSvc   *service.ApplicationAccessService
	providerSvc    *service.ProviderService
	roleSvc        *service.RoleService
	permissionSvc  *service.PermissionService
	jobTitleSvc    *service.JobTitleService
	jobLevelSvc    *service.JobLevelService
	certSvc        *service.CertService
	tokenSvc       *service.TokenService
	sessionSvc     *service.SessionService
	recordSvc      *service.RecordService
	syncerSvc      *service.SyncerService
	provisionSvc   *service.ProvisioningService
	authSvc        *service.AuthService
	captchaSvc     *service.CaptchaService
	storageSvc     *service.StorageService
	codeStore      *redisrepo.CodeStore
	magicLinkStore *redisrepo.MagicLinkStore
	oidcSvc        *service.OIDCService
	backchannelSvc *service.BackchannelLogoutService

	// HTTP
	server       *server.Server
	middleware   *server.Middleware
	asyncdSvc    *asyncd.Service
	asyncdWorker *asyncd.Worker
	asyncdSched  *asyncd.Scheduler

	asyncdCtx    context.Context
	asyncdCancel context.CancelFunc
}

// New 创建应用实例
func New() (*Application, error) {
	a := &Application{}

	if err := a.initConfig(); err != nil {
		return nil, fmt.Errorf("初始化配置失败: %w", err)
	}
	if err := a.initLogger(); err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}
	logger.Info("application initializing...", "name", a.cfg.App.Name, "version", a.cfg.App.Version)

	tracing.Setup()

	if err := a.initDB(); err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}
	if err := a.initRedis(); err != nil {
		return nil, fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	a.initRepositories()
	if err := a.initAuth(); err != nil {
		return nil, fmt.Errorf("初始化认证组件失败: %w", err)
	}
	if err := a.initIssuer(); err != nil {
		return nil, fmt.Errorf("初始化令牌签发器失败: %w", err)
	}
	a.initServices()
	a.initServer()

	logger.Info("application initialized")
	return a, nil
}

func (a *Application) initConfig() error {
	a.configManager = config.NewManager()
	path := os.Getenv(config.EnvConfigPath)
	if path == "" {
		path = "configs/app.yaml"
	}
	if err := a.configManager.LoadFromFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: 加载配置文件 %s 失败，使用默认配置: %v\n", path, err)
		a.configManager.Update(config.DefaultConfig())
	}
	a.cfg = a.configManager.Get()
	return nil
}

func (a *Application) initLogger() error {
	return logger.Init(logger.Config{Level: a.cfg.Log.Level, Format: a.cfg.Log.Format, Output: a.cfg.Log.Output})
}

func (a *Application) initDB() error {
	client, err := dbclient.New(dbclient.Config{
		DSN:             a.cfg.Database.DSN(),
		MaxOpenConns:    int32(a.cfg.Database.MaxOpenConns),
		MaxIdleConns:    int32(a.cfg.Database.MaxIdleConns),
		ConnMaxLifetime: time.Duration(a.cfg.Database.ConnMaxLifetime) * time.Second,
	})
	if err != nil {
		return err
	}
	a.db = client
	logger.Info("postgresql connected")

	if a.cfg.Database.AutoMigrate {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		applied, err := migrator.Up(ctx, a.db.Pool(), a.cfg.Database.MigrationsDir)
		if err != nil {
			return fmt.Errorf("自动迁移失败: %w", err)
		}
		if len(applied) > 0 {
			logger.Info("migrations applied", "versions", applied)
		}
	}
	if a.cfg.Database.Seed {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := seed(ctx, a.db); err != nil {
			return fmt.Errorf("写入种子数据失败: %w", err)
		}
	}
	return nil
}

func (a *Application) initRedis() error {
	client, err := redisclient.New(redisclient.Config{
		Addr:         a.cfg.Redis.Addr(),
		Password:     a.cfg.Redis.Password,
		DB:           a.cfg.Redis.DB,
		PoolSize:     a.cfg.Redis.PoolSize,
		MinIdleConns: a.cfg.Redis.MinIdleConns,
	})
	if err != nil {
		return err
	}
	a.redis = client
	logger.Info("redis connected")
	return nil
}

func (a *Application) initRepositories() {
	a.orgRepo = postgres.NewInstitutionRepository(a.db)
	a.deptRepo = postgres.NewDepartmentRepository(a.db)
	a.userRepo = postgres.NewUserRepository(a.db)
	a.appRepo = postgres.NewApplicationRepository(a.db)
	a.appSubjectRepo = postgres.NewApplicationSubjectRepository(a.db)
	a.providerRepo = postgres.NewProviderRepository(a.db)
	a.storageSettingsRepo = postgres.NewStorageSettingsRepository(a.db)
	a.roleRepo = postgres.NewRoleRepository(a.db)
	a.permRepo = postgres.NewPermissionRepository(a.db)
	a.jobTitleRepo = postgres.NewJobTitleRepository(a.db)
	a.jobLevelRepo = postgres.NewJobLevelRepository(a.db)
	a.userDeptRepo = postgres.NewUserDepartmentRepository(a.db)
	a.linkRepo = postgres.NewSyncerLinkRepository(a.db)
	a.certRepo = postgres.NewCertRepository(a.db)
	a.tokenRepo = postgres.NewTokenRepository(a.db)
	a.recordRepo = postgres.NewRecordRepository(a.db)
	a.syncerRepo = postgres.NewSyncerRepository(a.db)
	a.connRepo = postgres.NewConnectorRepository(a.db)
	a.strategyRepo = postgres.NewAccountStrategyRepository(a.db)
	a.applicationAccountRepo = postgres.NewApplicationAccountRepository(a.db)
	a.provisionRepo = postgres.NewProvisionHistoryRepository(a.db)
	a.sessionRepo = redisrepo.NewSessionRepository(a.redis)
	a.rateLimitRepo = redisrepo.NewRateLimitRepository(a.redis)
	a.stateStore = redisrepo.NewStateStore(a.redis)
	logger.Info("repositories initialized")
}

func (a *Application) initAuth() error {
	issuerName := a.cfg.Auth.Issuer
	if issuerName == "" {
		issuerName = a.cfg.App.Name
	}
	a.jwtManager = auth.NewJWTManager(a.cfg.Auth.JWTSecret, issuerName, a.cfg.Auth.SessionTTL)

	key, err := resolveEncKey(a.cfg)
	if err != nil {
		return err
	}
	a.secretCipher, err = auth.NewSecretCipher(key)
	if err != nil {
		return err
	}

	a.sessionManager = auth.NewSessionManager(
		a.sessionRepo, a.userRepo, a.roleRepo, a.jobTitleRepo,
		time.Duration(a.cfg.Auth.SessionTTL)*time.Second,
	)
	// 提供商运行时：通知 / 存储 / 人机校验 / 外部身份提供方四处统一从这里读配置
	// （密钥解密只在这一处发生；控制台改动无需重启进程即可生效）
	a.providerRuntime = providerpkg.NewRuntime(a.providerRepo, a.secretCipher)
	a.oauthResolver = providerpkg.NewResolver(a.providerRuntime, 10*time.Second)

	a.authMiddleware = auth.NewMiddleware(
		a.jwtManager, a.sessionManager, a.userRepo, a.tokenRepo,
		originWhitelist(a.cfg.Security.CSRFAllowedOrigins),
	)
	a.addPublicPaths()
	logger.Info("auth components initialized")
	return nil
}

// seedOAuthProviders 把 configs/app.yaml 的 auth.oauth.providers 作为**迁移期种子**写入提供商表。
//
// 为什么保留：老部署的外部身份提供方只存在于 yaml 里；升级后若不做种子，登录页会突然变空。
// 幂等性：目标组织已有同名提供商时跳过（控制台是此后的唯一维护入口，yaml 不再是运行时权威）。
func (a *Application) seedOAuthProviders(ctx context.Context) {
	oauthCfg := a.cfg.Auth.OAuth
	if len(oauthCfg.Providers) == 0 {
		return
	}
	institutionName := oauthCfg.DefaultInstitution
	if institutionName == "" {
		logger.Warn("OAuth 提供商种子跳过：未配置 auth.oauth.default_institution")
		return
	}
	org, err := a.orgRepo.GetByName(ctx, institutionName)
	if err != nil {
		logger.Warn("OAuth 提供商种子跳过：组织不存在", "institution", institutionName, "error", err)
		return
	}
	for name, provider := range oauthCfg.Providers {
		if !provider.Enabled {
			continue
		}
		if _, err := a.providerRepo.GetByName(ctx, org.ID, name); err == nil {
			continue
		}
		category := providerpkg.CategoryOAuth
		typeKey := providerpkg.NormalizeType(category, provider.Type)
		if typeKey == "" {
			logger.Warn("OAuth 提供商种子跳过：类型不受支持", "name", name, "type", provider.Type)
			continue
		}
		item := &repository.Provider{
			InstitutionID: org.ID, Name: name, DisplayName: provider.DisplayName,
			Category: string(category), Type: typeKey, State: "active",
			ClientID: provider.ClientID, ClientSecret: provider.ClientSecret,
			IssuerURL: provider.Issuer, Scopes: provider.Scopes, RedirectURI: provider.RedirectURI,
		}
		policy := config.DefaultProvisioning()
		if provider.Provisioning != nil {
			policy = *provider.Provisioning
		}
		item.AutoCreate = policy.AutoCreate
		item.LinkByEmail = policy.LinkByEmail
		item.AllowedDomains = policy.AllowedDomains
		if err := a.providerRuntime.EncryptSecrets(item); err != nil {
			logger.Warn("OAuth 提供商种子跳过：密钥加密失败", "name", name, "error", err)
			continue
		}
		if _, err := a.providerRepo.Create(ctx, item); err != nil {
			logger.Warn("OAuth 提供商种子写入失败", "name", name, "error", err)
			continue
		}
		logger.Info("OAuth 提供商已从配置种子写入（此后请在控制台「提供商」中维护）", "name", name)
	}
}

func (a *Application) addPublicPaths() {
	for _, p := range []string{
		"/healthz", "/readyz",
		"/.well-known/openid-configuration", "/.well-known/jwks",
		"/oauth/token", "/oauth/userinfo", "/oauth/introspect", "/oauth/logout", "/oauth/authorize",
		"/api/v1/auth/login", "/api/v1/auth/signup", "/api/v1/auth/oauth/*",
	} {
		a.authMiddleware.AddPublicPath(p)
	}
}

func (a *Application) initIssuer() error {
	certName := a.cfg.Auth.OIDC.CertName
	if certName == "" {
		certName = builtInCert
	}
	// 签名证书按组织主键定位（配置里给的是组织标识，先解析成 id）
	institutionName := a.cfg.Auth.OAuth.DefaultInstitution
	if institutionName == "" {
		institutionName = builtInInstitution
	}
	signingOrg, orgErr := a.orgRepo.GetByName(context.Background(), institutionName)
	if orgErr != nil {
		return orgErr
	}
	cert, err := a.certRepo.GetByName(context.Background(), signingOrg.ID, certName)
	if err != nil || cert.Certificate == "" {
		certPEM, keyPEM, _, genErr := oidc.GenerateCert(2048, 10)
		if genErr != nil {
			return genErr
		}
		cert = &repository.Cert{
			InstitutionID: signingOrg.ID, Name: certName, DisplayName: "运行时签名证书",
			Scope: "JWT", Type: "x509", CryptoAlgorithm: "RSA", BitSize: 2048, ExpireInYears: 10,
			Certificate: certPEM, PrivateKey: keyPEM,
		}
		if _, createErr := a.certRepo.Create(context.Background(), cert); createErr != nil {
			logger.Warn("签名证书落库失败，使用内存证书", "error", createErr)
		}
	}
	certManager, err := oidc.NewCertManager(cert.Certificate, cert.PrivateKey)
	if err != nil {
		return err
	}
	// JWKS 多密钥平滑轮换：加载附加校验密钥（含公钥即可），一并发布并用于验签既有令牌。
	for _, extra := range a.cfg.Auth.OIDC.AdditionalKeyCertNames {
		if extra == "" || extra == certName {
			continue
		}
		extraCert, err := a.certRepo.GetByName(context.Background(), signingOrg.ID, extra)
		if err != nil || extraCert.Certificate == "" {
			logger.Warn("附加签名证书不可用，跳过", "name", extra, "error", err)
			continue
		}
		if err := certManager.AddKey(extraCert.Certificate, extraCert.PrivateKey); err != nil {
			logger.Warn("附加签名证书加载失败，跳过", "name", extra, "error", err)
		}
	}
	a.issuer = oidc.NewIssuer(oidc.Config{
		CertManager:          certManager,
		IssuerURL:            a.cfg.IssuerURL(),
		AccessTokenExpire:    time.Duration(a.cfg.Auth.OIDC.AccessTokenExpireHours) * time.Hour,
		RefreshTokenExpire:   time.Duration(a.cfg.Auth.OIDC.RefreshTokenExpireHours) * time.Hour,
		AuthorizationCodeTTL: time.Duration(a.cfg.Auth.OIDC.AuthorizationCodeExpireSeconds) * time.Second,
	})
	logger.Info("token issuer initialized", "issuer", a.cfg.IssuerURL(), "kid", certManager.KID())
	return nil
}

func (a *Application) initServices() {
	a.audits = service.NewAuditRecorder(a.recordRepo)
	a.audits.SetUserResolver(a.userRepo)
	// 人机校验与验证码（需先于 AuthService / ProviderService 构造）
	a.captchaSvc = service.NewCaptchaService(a.providerRuntime, a.orgRepo, redisrepo.NewCaptchaStore(a.redis, 5*time.Minute))
	a.codeStore = redisrepo.NewCodeStore(a.redis)
	a.magicLinkStore = redisrepo.NewMagicLinkStore(a.redis)
	a.institutionSvc = service.NewInstitutionService(service.InstitutionDeps{
		Orgs: a.orgRepo, Users: a.userRepo, JobTitles: a.jobTitleRepo, Depts: a.deptRepo, JobLevels: a.jobLevelRepo, Roles: a.roleRepo,
		Permissions: a.permRepo, Apps: a.appRepo, Providers: a.providerRepo,
		Syncers: a.syncerRepo, Tokens: a.tokenRepo, Records: a.recordRepo, Audits: a.audits,
	})
	a.departmentSvc = service.NewDepartmentService(a.deptRepo, a.orgRepo, a.userRepo, a.audits)
	a.userSvc = service.NewUserService(
		a.userRepo, a.orgRepo, a.roleRepo, a.jobTitleRepo, a.deptRepo, a.userDeptRepo, a.jobLevelRepo,
		a.sessionManager, a.audits, a.secretCipher,
	)
	// 登录失败锁定（管理面枚举 + 解除；键结构与登录链路同源，见 service/login_lockout.go）
	a.lockoutSvc = service.NewLoginLockoutService(
		a.rateLimitRepo, a.userRepo, a.orgRepo, a.appRepo, a.audits,
		int64(a.cfg.Auth.RateLimit.LoginMaxAttempts),
		time.Duration(a.cfg.Auth.RateLimit.LoginWindowSeconds)*time.Second,
	)
	a.applicationSvc = service.NewApplicationService(a.appRepo, a.orgRepo, a.audits)
	// 应用访问授权（谁能使用该应用）：唯一判定入口，见 Spec 第六阶段
	a.appAccessSvc = service.NewApplicationAccessService(
		a.appSubjectRepo, a.userRepo, a.jobTitleRepo, a.deptRepo, a.sessionManager, a.audits,
	)
	a.storageSvc = service.NewStorageService(a.providerRuntime, a.userRepo, a.storageSettingsRepo, a.secretCipher, a.audits)
	a.providerSvc = service.NewProviderService(
		a.providerRepo, a.orgRepo, a.providerRuntime, a.oauthResolver, a.captchaSvc, a.storageSvc, a.audits,
	)
	a.roleSvc = service.NewRoleService(a.roleRepo, a.jobTitleRepo, a.orgRepo, a.sessionManager, a.audits)
	a.permissionSvc = service.NewPermissionService(a.permRepo, a.orgRepo, a.roleRepo, a.sessionManager, a.audits)
	a.jobTitleSvc = service.NewJobTitleService(a.jobTitleRepo, a.orgRepo, a.userRepo, a.roleRepo, a.sessionManager, a.audits)
	a.jobLevelSvc = service.NewJobLevelService(a.jobLevelRepo, a.orgRepo, a.audits)
	a.certSvc = service.NewCertService(a.certRepo, a.orgRepo, a.audits)
	a.tokenSvc = service.NewTokenService(a.tokenRepo, a.audits)
	a.sessionSvc = service.NewSessionService(a.sessionManager, a.audits)
	a.recordSvc = service.NewRecordService(a.recordRepo)
	a.syncerSvc = service.NewSyncerService(a.syncerRepo, a.userRepo, a.deptRepo, a.linkRepo, a.userDeptRepo, a.jobTitleRepo, a.orgRepo, a.secretCipher, a.audits)
	// 供给服务（连接器 / 账号策略 / 应用账号 / 供给历史 + 工作池）
	a.provisionSvc = service.NewProvisioningService(
		a.connRepo, a.strategyRepo, a.applicationAccountRepo, a.provisionRepo,
		a.userRepo, a.appRepo, a.orgRepo, a.secretCipher, a.audits,
	)
	a.authSvc = service.NewAuthService(
		a.orgRepo, a.userRepo, a.appRepo, a.jobTitleRepo, a.providerRuntime, a.oauthResolver,
		a.captchaSvc, a.codeStore, a.magicLinkStore,
		a.userSvc, a.sessionManager, a.jwtManager,
		a.stateStore, a.rateLimitRepo, a.audits,
		service.AuthServiceConfig{
			OAuth:       a.cfg.Auth.OAuth,
			LoginMax:    int64(a.cfg.Auth.RateLimit.LoginMaxAttempts),
			LoginWindow: time.Duration(a.cfg.Auth.RateLimit.LoginWindowSeconds) * time.Second,
			SessionTTL:  time.Duration(a.cfg.Auth.SessionTTL) * time.Second,
		},
	)
	a.oidcSvc = service.NewOIDCService(a.appRepo, a.userRepo, a.orgRepo, a.roleRepo, a.jobTitleRepo, a.tokenRepo, a.issuer, a.audits)
	// 后端登出服务（OIDC Back-Channel Logout）：会话结束时通知下游应用
	a.backchannelSvc = service.NewBackchannelLogoutService(a.appRepo, a.tokenRepo, a.issuer, a.audits)
	// 供给事件挂钩：用户创建/停用/离职、角色授予、组成员、登录 JIT
	a.userSvc.SetProvisioning(a.provisionSvc)
	a.roleSvc.SetProvisioning(a.provisionSvc)
	a.jobTitleSvc.SetProvisioning(a.provisionSvc)
	a.authSvc.SetProvisioning(a.provisionSvc)
	// 后端登出挂钩：本地登出时通知下游 RP
	a.authSvc.SetBackchannelLogout(a.backchannelSvc)
	// 应用准入挂钩：登录族 / IdP 令牌链路 / 门户应用中心共用同一判定入口
	a.authSvc.SetApplicationAccess(a.appAccessSvc)
	a.oidcSvc.SetApplicationAccess(a.appAccessSvc)
	a.provisionSvc.SetApplicationAccess(a.appAccessSvc)
	// 迁移期兜底：把 yaml 里的外部身份提供方种子写入提供商表（幂等）
	a.seedOAuthProviders(context.Background())
	logger.Info("services initialized")
}

func (a *Application) initServer() {
	a.server = server.New(&a.cfg.App)
	a.middleware = server.NewMiddleware(a.cfg, a.authMiddleware)
	a.middleware.Register(a.server.Router())

	// 异步任务与定时调度（DB 版 asynq；四维度模型见 docs/specs/261015-asyncd-task-model.md）：
	// **必须在构造 handlers 之前完成**——handler 持有的是服务实例，晚构造会让 /asyncd/* 拿到 nil。
	a.asyncdSvc = asyncd.NewService(
		postgres.NewAsyncdJobRepository(a.db),
		postgres.NewAsyncdJobRunRepository(a.db),
		postgres.NewAsyncdJobRunAttemptRepository(a.db),
		postgres.NewAsyncdWorkerRepository(a.db),
	)
	a.syncerSvc.SetAsyncd(a.asyncdSvc)
	a.provisionSvc.SetAsyncd(a.asyncdSvc)

	handlers := &server.Handlers{
		Auth:        handler.NewAuthHandler(a.authSvc, a.userSvc, a.applicationSvc),
		OIDC:        handler.NewOIDCHandler(a.oidcSvc, a.authSvc),
		Health:      handler.NewHealthHandler(a.db, a.redis),
		User:        handler.NewUserHandler(a.userSvc, a.lockoutSvc, a.sessionSvc, a.provisionSvc, a.recordSvc, a.tokenSvc, a.storageSvc, a.linkRepo),
		Institution: handler.NewInstitutionHandler(a.institutionSvc),
		Department:  handler.NewDepartmentHandler(a.departmentSvc),
		Application: handler.NewApplicationHandler(a.applicationSvc, a.appAccessSvc),
		Provider:    handler.NewProviderHandler(a.providerSvc),
		Storage:     handler.NewStorageHandler(a.storageSvc),
		Role:        handler.NewRoleHandler(a.roleSvc),
		Permission:  handler.NewPermissionHandler(a.permissionSvc),
		JobTitle:    handler.NewJobTitleHandler(a.jobTitleSvc),
		JobLevel:    handler.NewJobLevelHandler(a.jobLevelSvc),
		Cert:        handler.NewCertHandler(a.certSvc),
		Token:       handler.NewTokenHandler(a.tokenSvc),
		Session:     handler.NewSessionHandler(a.sessionSvc),
		Record:      handler.NewRecordHandler(a.recordSvc),
		Syncer:      handler.NewSyncerHandler(a.syncerSvc),
		Provision:   handler.NewProvisioningHandler(a.provisionSvc),
		Connection:  handler.NewConnectionHandler(),
		Asyncd:      handler.NewAsyncdHandler(a.asyncdSvc),
	}
	server.RegisterRoutes(a.server.Router(), a.middleware, handlers)

	// 任务入队可与业务写入同事务，worker 领取用 SKIP LOCKED（多实例安全），
	// 定时任务只负责入队，执行路径唯一（worker → 处理器）。
	registry := a.asyncdSvc.Registry()
	registry.Register("maintenance.token_cleanup", func(ctx context.Context, _ *repository.AsyncdJobRun, _ map[string]any) error {
		return a.cleanupTokens(ctx)
	}, asyncd.HandlerMaxAttempts(3))
	registry.Register("maintenance.record_cleanup", func(ctx context.Context, _ *repository.AsyncdJobRun, _ map[string]any) error {
		return a.cleanupRecords(ctx)
	}, asyncd.HandlerMaxAttempts(3))
	// 执行实例/执行记录/执行器实例的保留期清理（配置来自任务定义的 payload，运行时读取）
	registry.Register("maintenance.asyncd_cleanup", func(ctx context.Context, _ *repository.AsyncdJobRun, payload map[string]any) error {
		return a.cleanupAsyncd(ctx, payload)
	}, asyncd.HandlerMaxAttempts(3))
	// 同步计划对账：连接的「同步计划」现在就是 asyncd 的定时任务（一条定义），
	// 这个任务只负责让"库里的定义"与"连接的真实状态"保持一致（自愈），不再做"到点判定"。
	registry.Register("syncer.schedule.reconcile", func(ctx context.Context, _ *repository.AsyncdJobRun, _ map[string]any) error {
		return a.syncerSvc.ReconcileScheduleJobs(ctx)
	})
	// 同步器运行任务：真正执行一次同步（差异 → 保护 → 落地 → 运行记录）。
	// 单飞键由入队侧给出（syncer:<id>），因此同一数据源不会并发跑两次。
	registry.Register(service.TaskTypeSyncerRun, func(ctx context.Context, _ *repository.AsyncdJobRun, payload map[string]any) error {
		syncerID := int64(payloadNumber(payload, "syncer_id"))
		institutionID := int64(payloadNumber(payload, "institution_id"))
		trigger := payloadString(payload, "trigger")
		if trigger == "" {
			trigger = service.SyncTriggerSchedule
		}
		sy, err := a.syncerSvc.ByID(ctx, institutionID, syncerID)
		if err != nil {
			// 数据源已被删除：直接失败（不再重试，避免无意义堆积）
			return err
		}
		_, err = a.syncerSvc.RunWithTrigger(ctx, "asyncd", trigger, sy)
		return err
	})
	registry.Register("provisioning.retry", func(ctx context.Context, _ *repository.AsyncdJobRun, _ map[string]any) error {
		return a.retryProvisioning(ctx)
	})
	// 单账号供给任务：建号 / 改密 / 启用停用 / 删除（幂等键 = 机构×应用×用户，失败按退避重试、超限进死信）
	registry.Register(service.TaskTypeProvisionAccount, func(ctx context.Context, _ *repository.AsyncdJobRun, payload map[string]any) error {
		a.provisionSvc.ProcessTask(ctx, service.ProvisionTaskFromPayload(payload))
		return nil
	})

	// 任务定义声明（幂等 upsert；重启不会打乱 next_run_at，也不会把界面上的启停掰回默认值）：
	//   kind=async     —— 由业务动作按需入队（定义入库后，任务中心连"从未执行过"的任务也列得出）
	//   kind=scheduled —— 由调度器按间隔/时刻入队，可停用
	declared := make([]string, 0, 8)
	for _, job := range []asyncd.Job{
		{Name: service.TaskTypeSyncerRun, TaskType: service.TaskTypeSyncerRun, Kind: repository.AsyncdJobKindAsync},
		{Name: service.TaskTypeProvisionAccount, TaskType: service.TaskTypeProvisionAccount, Kind: repository.AsyncdJobKindAsync},
		{
			Name: "maintenance.token_cleanup", TaskType: "maintenance.token_cleanup",
			Kind: repository.AsyncdJobKindScheduled, Enabled: true,
			Schedule: asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 3600},
		},
		{
			Name: "maintenance.record_cleanup", TaskType: "maintenance.record_cleanup",
			Kind: repository.AsyncdJobKindScheduled, Enabled: true,
			Schedule: asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 6 * 3600},
		},
		{
			// 保留期清理：执行实例/尝试记录留 14 天，失联执行器留 3 天（payload 由处理器读取）
			Name: "maintenance.asyncd_cleanup", TaskType: "maintenance.asyncd_cleanup",
			Kind: repository.AsyncdJobKindScheduled, Enabled: true,
			Payload:  map[string]any{"retention_days": 14, "worker_retention_days": 3},
			Schedule: asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 3600},
		},
		{
			// 同步计划对账：每分钟把"连接持有的计划任务"与连接的真实状态对齐（自愈兜底）
			Name: "syncer.schedule.reconcile", TaskType: "syncer.schedule.reconcile",
			Kind: repository.AsyncdJobKindScheduled, Enabled: true,
			Schedule: asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 60},
		},
		{
			Name: "provisioning.retry", TaskType: "provisioning.retry",
			Kind: repository.AsyncdJobKindScheduled, Enabled: true,
			Schedule: asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 120},
		},
	} {
		// 定义注册失败不应阻断启动：记日志并在下一次启动时重试（upsert 幂等）
		if err := a.asyncdSvc.RegisterJob(context.Background(), job); err != nil {
			logger.Error("注册任务定义失败", "job", job.Name, "error", err)
			continue
		}
		declared = append(declared, job.Name)
	}
	// 代码里删掉的任务声明：**退役**而不是删除（删定义会级联删掉它的历史执行记录）
	if retired, err := a.asyncdSvc.RetireUndeclared(context.Background(), declared); err != nil {
		logger.Error("退役未声明的任务定义失败", "error", err)
	} else if retired > 0 {
		logger.Info("已退役未声明的任务定义", "count", retired)
	}
}

// Start 启动应用
func (a *Application) Start() error {
	if a.asyncdSvc != nil {
		a.asyncdCtx, a.asyncdCancel = context.WithCancel(context.Background())
		a.asyncdWorker = asyncd.NewWorker(a.asyncdSvc, asyncd.WithConcurrency(4))
		a.asyncdWorker.Start(a.asyncdCtx)
		a.asyncdSched = asyncd.NewScheduler(a.asyncdSvc)
		a.asyncdSched.Start(a.asyncdCtx)
	}
	return a.server.Start()
}

// Stop 停止应用
func (a *Application) Stop(ctx context.Context) error {
	logger.Info("stopping application...")
	if a.asyncdSched != nil {
		a.asyncdSched.Stop()
	}
	if a.asyncdWorker != nil {
		a.asyncdWorker.Stop()
	}
	if a.asyncdCancel != nil {
		a.asyncdCancel()
	}
	if err := a.server.Stop(ctx); err != nil {
		logger.Error("failed to stop server", "error", err)
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
	_ = logger.Sync()
	return nil
}

// Config 获取配置
func (a *Application) Config() *config.Config { return a.cfg }

// DB 获取数据库客户端
func (a *Application) DB() *dbclient.Client { return a.db }

// Redis 获取 Redis 客户端
func (a *Application) Redis() *redisclient.Client { return a.redis }

// originWhitelist 过滤配置中的 "*"（"*" 表示不参与 CSRF 校验）
func originWhitelist(origins []string) []string {
	result := make([]string, 0, len(origins))
	for _, o := range origins {
		if o != "*" {
			result = append(result, o)
		}
	}
	return result
}

// resolveEncKey 解析敏感字段加密密钥：
// 优先 POLARIS_CREDENTIAL_ENC_KEY（base64 32 字节）；开发环境未配置时以 jwt_secret 派生，生产强制注入。
func resolveEncKey(cfg *config.Config) ([]byte, error) {
	if cfg.Security.CredentialEncKey != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.Security.CredentialEncKey)
		if err != nil {
			return nil, fmt.Errorf("POLARIS_CREDENTIAL_ENC_KEY 不是合法 base64: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("POLARIS_CREDENTIAL_ENC_KEY 解码后必须为 32 字节（AES-256），实际 %d", len(key))
		}
		return key, nil
	}
	if cfg.App.Env == "production" {
		return nil, fmt.Errorf("生产环境必须配置 POLARIS_CREDENTIAL_ENC_KEY")
	}
	logger.Warn("POLARIS_CREDENTIAL_ENC_KEY 未配置，使用 jwt_secret 派生加密密钥（仅限开发）")
	sum := sha256.Sum256([]byte(cfg.Auth.JWTSecret))
	return sum[:], nil
}

// payloadNumber / payloadString 读取任务载荷字段（JSON 数字统一为 float64）
func payloadNumber(payload map[string]any, key string) float64 {
	switch value := payload[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	}
	return 0
}

func payloadString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}
