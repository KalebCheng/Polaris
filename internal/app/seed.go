package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	"polaris/internal/auth/tokenhash"
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
func seed(ctx context.Context, db *dbclient.Client, cipher *auth.SecretCipher) error {
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
	encryptedKey, err := cipher.Encrypt(keyPEM)
	if err != nil {
		return fmt.Errorf("加密内置证书私钥失败: %w", err)
	}
	expire := time.Now().AddDate(10, 0, 0)
	builtInCertRecord := &repository.Cert{
		InstitutionID: orgID, Name: builtInCert, DisplayName: "内置签名证书",
		Scope: "JWT", Type: "x509", CryptoAlgorithm: "RSA", BitSize: 2048, ExpireInYears: 10,
		Certificate: certPEM, PrivateKey: encryptedKey, ExpireTime: &expire, IsSystem: true,
	}
	if _, err := certRepo.Create(ctx, builtInCertRecord); err != nil {
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
		Category: "default", Type: "oidc", CertID: &builtInCertRecord.ID,
		// access_mode 从 031 起有 CHECK 约束（all | restricted）；缺省零值空串会直接违反约束
		AccessMode:        repository.AccessModeAll,
		SelfSignupEnabled: true, SignupAutoSignin: true, MaxSessions: 5,
		EmailCodeTTLSeconds: 300, SMSCodeTTLSeconds: 300, MagicLinkTTLSeconds: 600,
		SigninMethods: []repository.SigninMethod{
			{
				Name: repository.SigninMethodPassword, DisplayName: "Password", Rule: "All",
				Identifiers: []string{repository.PasswordIdentifierUsername},
				MfaEnabled:  true, MfaRequired: false, MfaProviderIDs: []int64{},
			},
		},
		GrantTypes:       []string{"authorization_code", "password", "refresh_token", "client_credentials"},
		RedirectURIs:     redirects,
		ClientID:         "polaris-built-in-client",
		ClientSecretHash: tokenhash.Hash("polaris-built-in-secret"),
		TokenFormat:      "jwt", TokenSigningMethod: "RS256",
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
	if err := roleRepo.SetMembers(ctx, adminRole.ID, &repository.RoleRelations{PermissionIDs: []int64{allPerm.ID}}); err != nil {
		return err
	}
	userRole := &repository.Role{InstitutionID: orgID, Name: builtInUserRole, DisplayName: "普通用户", IsEnabled: true, IsSystem: true}
	if _, err := roleRepo.Create(ctx, userRole); err != nil {
		return err
	}
	if err := roleRepo.SetMembers(ctx, userRole.ID, &repository.RoleRelations{PermissionIDs: []int64{userPerm.ID}}); err != nil {
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
		return fmt.Errorf("%s is required when seeding is enabled", envAdminPassword)
	}
	hash, err := auth.HashPasswordWith(password, org.PasswordType, org.PasswordSalt)
	if err != nil {
		return err
	}
	now := time.Now()
	adminUser := &repository.User{
		UID: uuid.NewString(), InstitutionID: orgID, Name: builtInAdmin, Type: "admin",
		PasswordHash: hash, PasswordType: auth.NormalizePasswordType(org.PasswordType),
		PasswordUpdatedAt: &now,
		DisplayName:       "管理员", Email: "admin@polaris.local", EmailVerified: true,
		IsAdmin: true, Language: "zh", SignupApplication: builtInApp,
	}
	adminID, err := userRepo.Create(ctx, adminUser)
	if err != nil {
		return err
	}
	adminUser.ID = adminID
	// 管理员归属根部门（唯一真相：user_departments）
	if adminID > 0 && rootDept.ID > 0 {
		if err := userDeptRepo.ReplaceForUser(ctx, orgID, adminID, []int64{rootDept.ID}, rootDept.ID, nil); err != nil {
			return err
		}
	}
	if err := roleRepo.SetMembers(ctx, adminRole.ID, &repository.RoleRelations{
		UserIDs: []int64{adminID}, PermissionIDs: []int64{allPerm.ID},
	}); err != nil {
		return err
	}
	if err := jobTitleRepo.SetMemberIDs(ctx, jobTitle.ID, []int64{adminID}); err != nil {
		return err
	}

	logger.Info("seed data created", "admin", builtInAdmin, "password_hint", "override with POLARIS_ADMIN_PASSWORD")
	return nil
}
