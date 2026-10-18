package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// UserService 用户服务
type UserService struct {
	users    repository.UserRepository
	orgs     repository.InstitutionRepository
	roles    repository.RoleRepository
	jobTitles repository.JobTitleRepository
	depts    repository.DepartmentRepository
	userDepts repository.UserDepartmentRepository
	levels   repository.JobLevelRepository
	sessions *auth.SessionManager
	audits   *AuditRecorder
	cipher   *auth.SecretCipher
	// provision 供给事件挂钩（可选；装配后用户创建/停用/离职会触发下游账号供给）。
	provision *ProvisioningService
}

// NewUserService 创建用户服务。
// roles / groups / depts 用于用户详情页的授权与归属读写（"谁在什么时候归属于哪里"是用户的聚合视图）。
func NewUserService(
	users repository.UserRepository,
	orgs repository.InstitutionRepository,
	roles repository.RoleRepository,
	jobTitles repository.JobTitleRepository,
	depts repository.DepartmentRepository,
	userDepts repository.UserDepartmentRepository,
	levels repository.JobLevelRepository,
	sessions *auth.SessionManager,
	audits *AuditRecorder,
	cipher *auth.SecretCipher,
) *UserService {
	return &UserService{
		users: users, orgs: orgs, roles: roles, jobTitles: jobTitles, depts: depts,
		userDepts: userDepts, levels: levels,
		sessions: sessions, audits: audits, cipher: cipher,
	}
}

// SetProvisioning 注入供给服务（装配阶段调用；避免构造函数循环依赖）。
func (s *UserService) SetProvisioning(p *ProvisioningService) { s.provision = p }

// CreateUserInput 创建用户入参
type CreateUserInput struct {
	User     *repository.User
	Password string
}

// Create 创建用户
func (s *UserService) Create(ctx context.Context, actor string, in CreateUserInput) (*repository.User, error) {
	u := in.User
	u.Name = strings.TrimSpace(u.Name)
	if u.InstitutionID == 0 || u.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	// 表单字段一律去首尾空白（控制台整页表单会带进来）
	trimUserFields(u)
	// 部门必须属于本组织（否则外键会以 409 抛出，语义不如 400 明确）
	if u.DepartmentID != nil {
		dept, err := s.depts.GetByID(ctx, *u.DepartmentID)
		if err != nil || dept.InstitutionID != u.InstitutionID {
			return nil, bizerrors.ErrInvalidParams.WithDetail("department_id", *u.DepartmentID)
		}
	}
	org, err := s.orgs.GetByID(ctx, u.InstitutionID)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", u.InstitutionID)
	}
	if _, err := s.users.GetByName(ctx, u.InstitutionID, u.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "user already exists")
	}
	if u.UID == "" {
		u.UID = uuid.NewString()
	}
	if in.Password != "" {
		if err := auth.FromOptions(org.PasswordOptions).Validate(in.Password); err != nil {
			return nil, bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
		}
		hash, err := auth.HashPasswordWith(in.Password, org.PasswordType, org.PasswordSalt)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
		}
		u.PasswordHash = hash
		u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
		u.PasswordSalt = org.PasswordSalt
		now := time.Now()
		u.PasswordUpdatedAt = &now
	} else if org.DefaultPassword != "" {
		hash, err := auth.HashPasswordWith(org.DefaultPassword, org.PasswordType, org.PasswordSalt)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
		}
		u.PasswordHash = hash
		u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
		u.PasswordSalt = org.PasswordSalt
	}
	normalizeUserType(u)
	id, err := s.users.Create(ctx, u)
	if err != nil {
		return nil, err
	}
	u.ID = id
	s.audits.Record(ctx, actor, u.InstitutionID, "user.create", u.Name, nil)
	// 触发下游应用账号供给（"创建即建号"策略）
	if s.provision != nil {
		s.provision.TriggerUserProvisioning(ctx, u.InstitutionID, u.Name)
	}
	return u, nil
}

// Get 查询用户
func (s *UserService) Get(ctx context.Context, institutionID int64, name string) (*repository.User, error) {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("user", name)
	}
	return u, nil
}

// Delete 删除用户（软删除）
func (s *UserService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	// 内置系统管理员账号属于系统自身，不允许删除（否则将无人可管理系统）
	// 内置组织按**名字**标识（恒定），故这里取回组织名再判断
	if org, err := s.orgs.GetByID(ctx, institutionID); err == nil {
		if repository.IsBuiltInAdmin(org.Name, name) {
			return bizerrors.ErrProtectedObject.WithDetail("user", name)
		}
	}
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("user", name)
	}
	// 防呆：内置 admin 不可删除；组织内最后一个"仍可登录"的管理员不可被删除
	org, _ := s.orgs.GetByID(ctx, institutionID)
	after := *u
	after.IsDeleted = true
	if err := s.assertAdminSafety(ctx, org, u, &after); err != nil {
		return err
	}
	if err := s.users.SoftDelete(ctx, u.ID); err != nil {
		return err
	}
	if s.sessions != nil {
		_, _ = s.sessions.InvalidateUser(ctx, institutionID, name)
	}
	// 离职：触发下游应用账号回收
	if s.provision != nil {
		s.provision.TriggerUserStatusChange(ctx, institutionID, name, true)
	}
	s.audits.Record(ctx, actor, institutionID, "user.delete", name, nil)
	return nil
}

// ListQuery 按目录条件分页列出用户（关键词 + 部门 + 状态 + MFA 等附加过滤）。
func (s *UserService) ListQuery(ctx context.Context, q repository.ListQuery) ([]*repository.User, int64, error) {
	items, err := s.users.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.users.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// SetPassword 管理员重置密码（校验策略与历史，随后失效会话）。
func (s *UserService) SetPassword(ctx context.Context, actor string, institutionID int64, name, newPassword string) error {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found").WithDetail("user", name)
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	if err := s.validatePassword(ctx, org, u, newPassword); err != nil {
		return err
	}
	hash, err := auth.HashPasswordWith(newPassword, org.PasswordType, org.PasswordSalt)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
	}
	u.PasswordHash = hash
	u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
	u.PasswordSalt = org.PasswordSalt
	if err := s.users.UpdatePassword(ctx, u, org.PasswordHistoryCount); err != nil {
		return err
	}
	if s.sessions != nil {
		_, _ = s.sessions.InvalidateUser(ctx, institutionID, name)
	}
	s.audits.Record(ctx, actor, institutionID, "user.reset_password", name, nil)
	return nil
}

// ChangePassword 用户自助修改密码。
func (s *UserService) ChangePassword(ctx context.Context, institutionID int64, name, oldPassword, newPassword string) error {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	if !auth.VerifyPasswordWith(oldPassword, u.PasswordHash, u.PasswordType, u.PasswordSalt) {
		return bizerrors.ErrInvalidCredentials
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	if err := s.validatePassword(ctx, org, u, newPassword); err != nil {
		return err
	}
	hash, err := auth.HashPasswordWith(newPassword, org.PasswordType, org.PasswordSalt)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
	}
	u.PasswordHash = hash
	u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
	u.PasswordSalt = org.PasswordSalt
	if err := s.users.UpdatePassword(ctx, u, org.PasswordHistoryCount); err != nil {
		return err
	}
	if s.sessions != nil {
		_, _ = s.sessions.InvalidateUser(ctx, institutionID, name)
	}
	s.audits.Record(ctx, name, institutionID, "user.change_password", name, nil)
	return nil
}

func (s *UserService) validatePassword(ctx context.Context, org *repository.Institution, u *repository.User, password string) error {
	if err := auth.FromOptions(org.PasswordOptions).Validate(password); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	if org.PasswordHistoryCount > 0 {
		hashes, err := s.users.PasswordHistory(ctx, org.ID, u.Name, org.PasswordHistoryCount)
		if err != nil {
			return err
		}
		for _, h := range hashes {
			if auth.VerifyPasswordWith(password, h, org.PasswordType, org.PasswordSalt) {
				return bizerrors.New(bizerrors.CodeInvalidParams, "new password must not reuse recent passwords")
			}
		}
	}
	return nil
}

// MfaSetupResult MFA 绑定结果
type MfaSetupResult struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

// SetupMFA 生成 TOTP 密钥（暂存于用户记录，待 VerifyMFA 启用）。
func (s *UserService) SetupMFA(ctx context.Context, institutionID int64, name string) (*MfaSetupResult, error) {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	// 组织标识（name）优先，取不到时回落主键字符串——标签仅用于人眼识别
	institution := strconv.FormatInt(institutionID, 10)
	if org, orgErr := s.orgs.GetByID(ctx, institutionID); orgErr == nil && org != nil && org.Name != "" {
		institution = org.Name
	}
	secret, url, err := auth.GenerateTOTPSecret(institution, name)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成 TOTP 密钥失败", err)
	}
	encrypted, err := s.cipher.Encrypt(secret)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "加密 TOTP 密钥失败", err)
	}
	u.MfaSecret = encrypted
	u.MfaType = "totp"
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	return &MfaSetupResult{Secret: secret, URL: url}, nil
}

// VerifyMFA 校验动态码并启用 MFA，返回一次性恢复码。
func (s *UserService) VerifyMFA(ctx context.Context, institutionID int64, name, code string) ([]string, error) {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	secret, err := s.cipher.Decrypt(u.MfaSecret)
	if err != nil || secret == "" {
		return nil, bizerrors.New(bizerrors.CodeMfaNotBound, "no pending mfa secret; call setup first")
	}
	if !auth.VerifyTOTP(secret, code) {
		return nil, bizerrors.ErrMfaInvalid
	}
	codes, err := auth.GenerateRecoveryCodes(8)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成恢复码失败", err)
	}
	u.MfaEnabled = true
	u.MfaRecoveryCodes = codes
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, name, institutionID, "user.mfa_enable", name, nil)
	return codes, nil
}

// DisableMFA 关闭 MFA（用户自助关闭；管理员重置走 AdminResetMFA，审计动作不同）。
func (s *UserService) DisableMFA(ctx context.Context, actor string, institutionID int64, name string) error {
	return s.clearMFA(ctx, institutionID, name, actor, "user.mfa_disable")
}

// DecryptMfaSecret 返回用户 TOTP 明文密钥（仅用于登录二次校验）。
func (s *UserService) DecryptMfaSecret(u *repository.User) string {
	if u.MfaSecret == "" {
		return ""
	}
	secret, err := s.cipher.Decrypt(u.MfaSecret)
	if err != nil {
		return ""
	}
	return secret
}

// VerifyRecoveryCode 校验并消费一次性恢复码。
func (s *UserService) VerifyRecoveryCode(ctx context.Context, u *repository.User, code string) bool {
	code = strings.TrimSpace(code)
	for i, c := range u.MfaRecoveryCodes {
		if c == code {
			u.MfaRecoveryCodes = append(u.MfaRecoveryCodes[:i], u.MfaRecoveryCodes[i+1:]...)
			_ = s.users.Update(ctx, u)
			return true
		}
	}
	return false
}

// Identities 返回用户绑定的外部身份。
func (s *UserService) Identities(ctx context.Context, institutionID int64, name string) ([]*repository.UserIdentity, error) {
	return s.users.GetIdentitiesByUser(ctx, institutionID, name)
}

// UnbindIdentity 解绑外部身份。
func (s *UserService) UnbindIdentity(ctx context.Context, actor string, institutionID int64, name string, identityID int64) error {
	if err := s.users.UnbindIdentity(ctx, identityID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "user.unbind_identity", name, map[string]interface{}{"identity_id": identityID})
	return nil
}

// UnbindOwnIdentity 仅允许当前用户解绑自己名下的外部身份。
func (s *UserService) UnbindOwnIdentity(ctx context.Context, institutionID int64, name string, identityID int64) error {
	identities, err := s.users.GetIdentitiesByUser(ctx, institutionID, name)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if identity.ID == identityID && identity.InstitutionID == institutionID && identity.User == name {
			return s.UnbindIdentity(ctx, name, institutionID, name, identityID)
		}
	}
	return bizerrors.ErrNotFound.WithDetail("identity", identityID)
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *UserService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.users.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
