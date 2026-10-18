package app

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/repository/postgres"
	"polaris/pkg/dbclient"
)

// 内置资源命名（与前端约定一致）
const (
	builtInInstitution     = repository.BuiltInInstitution
	builtInInstitutionName = "Polaris"
	builtInRootDept        = "built-in" // 根部门（与租户同名，见迁移 005 的根部门约定）
	builtInApp             = "app-built-in"
	builtInCert            = "cert-built-in"
	builtInAdmin           = repository.BuiltInAdminUser
	builtInAdminRole       = "admin"
	builtInUserRole        = "user"
	envAdminPassword       = "POLARIS_ADMIN_PASSWORD"
)

// seed 写入初始数据（幂等：内置租户已存在则跳过）。
func seed(ctx context.Context, db *dbclient.Client) error {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM institutions WHERE name=$1)`, builtInInstitution).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	logger.Info("seeding initial data", "institution", builtInInstitution)

	orgRepo := postgres.NewInstitutionRepository(db)
	deptRepo := postgres.NewDepartmentRepository(db)
	userRepo := postgres.NewUserRepository(db)
	appRepo := postgres.NewApplicationRepository(db)
	certRepo := postgres.NewCertRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	permRepo := postgres.NewPermissionRepository(db)
	jobTitleRepo := postgres.NewJobTitleRepository(db)
	userDeptRepo := postgres.NewUserDepartmentRepository(db)

	// 1) 内置租户
	org := &repository.Institution{
		Name:                builtInInstitution,
		DisplayName:         builtInInstitutionName,
		PasswordType:        auth.NormalizePasswordType(""),
		PasswordOptions:     []string{auth.OptionAtLeast8},
		TokenRetentionDays:  30,
		RecordRetentionDays: 30,
		UseEmailAsUsername:  false,
		Status:              "active",
	}
	orgID, err := orgRepo.Create(ctx, org)
	if err != nil {
		return err
	}

	// 1.1) 根部门（部门树的起点；管理员默认归属于此）
	rootDept := &repository.Department{
		InstitutionID: orgID,
		Name:          builtInRootDept,
		DisplayName:   builtInInstitutionName,
		CodePath:      builtInRootDept,
		NamePath:      builtInInstitutionName,
		Level:         0,
		IsRoot:        true, // 组织根节点（Spec Q4.5：每个组织恰好一行，禁止改名/移动/删除）
	}
	if _, err := deptRepo.Create(ctx, rootDept); err != nil {
		return err
	}

	// 2) 内置证书（IdP 签名）
	certPEM, keyPEM, _, err := oidc.GenerateCert(2048, 10)
	if err != nil {
		return err
	}
	expire := time.Now().AddDate(10, 0, 0)
	if _, err := certRepo.Create(ctx, &repository.Cert{
		InstitutionID: orgID, Name: builtInCert, DisplayName: "内置签名证书",
		Scope: "JWT", Type: "x509", CryptoAlgorithm: "RSA", BitSize: 2048, ExpireInYears: 10,
		Certificate: certPEM, PrivateKey: keyPEM, ExpireTime: &expire, IsSystem: true,
	}); err != nil {
		return err
	}

	// 3) 内置应用（OAuth/OIDC 客户端）
	// 回调中间页挂在前端命名空间 /sso/callback（与后端 OIDC 协议路径 /oauth/* 分开）
	redirects := []string{
		"http://localhost:5173/sso/callback",
		"http://127.0.0.1:5173/sso/callback",
		"http://localhost:8000/sso/callback",
	}
	if _, err := appRepo.Create(ctx, &repository.Application{
		InstitutionID: orgID, Code: builtInApp, Name: "内置应用",
		Category: "default", Type: "oidc", Cert: builtInCert,
		// access_mode 从 031 起有 CHECK 约束（all | restricted）；缺省零值空串会直接违反约束
		AccessMode: repository.AccessModeAll,
		SelfSignupEnabled: true, SignupAutoSignin: true, MaxSessions: 5,
		EmailCodeTTLSeconds: 300, SMSCodeTTLSeconds: 300, MagicLinkTTLSeconds: 600,
		SigninMethods: []repository.SigninMethod{
			{Name: repository.SigninMethodPassword, DisplayName: "Password", Rule: "All"},
		},
		GrantTypes:   []string{"authorization_code", "password", "refresh_token", "client_credentials"},
		RedirectURIs: redirects,
		ClientID:     "polaris-built-in-client",
		ClientSecret: "polaris-built-in-secret",
		TokenFormat:  "jwt", TokenSigningMethod: "RS256",
		ExpireInHours: 1, RefreshExpireInHours: 24, CookieExpireInHours: 720,
		Scopes: []repository.ScopeItem{
			{Name: "openid", DisplayName: "OpenID"},
			{Name: "profile", DisplayName: "Profile"},
			{Name: "email", DisplayName: "Email"},
			{Name: "phone", DisplayName: "Phone"},
		},
		State: "active",
	}); err != nil {
		return err
	}

	// 4) 权限
	allPerm := &repository.Permission{
		InstitutionID: orgID, Name: "permission-all", DisplayName: "全部权限",
		Resources: []string{"*"}, Actions: []string{"*"}, Effect: "Allow", IsEnabled: true, State: "Approved",
	}
	if _, err := permRepo.Create(ctx, allPerm); err != nil {
		return err
	}
	userPerm := &repository.Permission{
		InstitutionID: orgID, Name: "permission-self", DisplayName: "自行读取",
		Resources: []string{"user", "application", "institution", "department", "connector", "account_strategy", "application_account", "provision_history"},
		Actions:   []string{"read"}, Effect: "Allow",
		IsEnabled: true, State: "Approved",
	}
	if _, err := permRepo.Create(ctx, userPerm); err != nil {
		return err
	}

	// 5) 角色
	adminRole := &repository.Role{InstitutionID: orgID, Name: builtInAdminRole, DisplayName: "管理员", IsEnabled: true, IsSystem: true}
	if _, err := roleRepo.Create(ctx, adminRole); err != nil {
		return err
	}
	if err := roleRepo.SetMembers(ctx, adminRole.ID, &repository.RoleRelations{Permissions: []string{"permission-all"}}); err != nil {
		return err
	}
	userRole := &repository.Role{InstitutionID: orgID, Name: builtInUserRole, DisplayName: "普通用户", IsEnabled: true, IsSystem: true}
	if _, err := roleRepo.Create(ctx, userRole); err != nil {
		return err
	}
	if err := roleRepo.SetMembers(ctx, userRole.ID, &repository.RoleRelations{Permissions: []string{"permission-self"}}); err != nil {
		return err
	}

	// 6) 内置职务（原「用户组」；Spec 第六阶段 Q3 = A 升级为职务）
	jobTitle := &repository.JobTitle{InstitutionID: orgID, Name: "built-in-job-title", DisplayName: "内置职务", IsEnabled: true}
	if _, err := jobTitleRepo.Create(ctx, jobTitle); err != nil {
		return err
	}

	// 7) 管理员用户
	password := os.Getenv(envAdminPassword)
	if password == "" {
		password = "admin123"
	}
	hash, err := auth.HashPasswordWith(password, org.PasswordType, org.PasswordSalt)
	if err != nil {
		return err
	}
	now := time.Now()
	adminUser := &repository.User{
		UID: uuid.NewString(), InstitutionID: orgID, Name: builtInAdmin, Type: "admin",
		DepartmentID: &rootDept.ID,
		PasswordHash: hash, PasswordType: auth.NormalizePasswordType(org.PasswordType),
		PasswordUpdatedAt: &now,
		DisplayName:       "管理员", Email: "admin@polaris.local", EmailVerified: true,
		IsAdmin: true, Language: "zh", SignupApplication: builtInApp,
	}
	if _, err := userRepo.Create(ctx, adminUser); err != nil {
		return err
	}
	// 管理员归属根部门：多值归属的真相在 user_departments（users.department_id 只是主部门的便利视图）
	if adminUser.ID > 0 && rootDept.ID > 0 {
		if err := userDeptRepo.ReplaceForUser(ctx, orgID, adminUser.ID, []int64{rootDept.ID}, rootDept.ID, nil); err != nil {
			return err
		}
	}
	if err := roleRepo.SetMembers(ctx, adminRole.ID, &repository.RoleRelations{
		Users: []string{builtInAdmin}, Permissions: []string{"permission-all"},
	}); err != nil {
		return err
	}
	if err := jobTitleRepo.SetMembers(ctx, jobTitle.ID, []string{builtInAdmin}); err != nil {
		return err
	}

	logger.Info("seed data created", "admin", builtInAdmin, "password_hint", "override with POLARIS_ADMIN_PASSWORD")
	return nil
}
