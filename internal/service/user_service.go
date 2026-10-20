package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// UserService 用户服务
type UserService struct {
	users     repository.UserRepository
	orgs      repository.InstitutionRepository
	roles     repository.RoleRepository
	jobTitles repository.JobTitleRepository
	depts     repository.DepartmentRepository
	userDepts repository.UserDepartmentRepository
	levels    repository.JobLevelRepository
	links     repository.SyncerLinkRepository
	sessions  sessionController
	audits    *AuditRecorder
	cipher    *auth.SecretCipher
	// provision 供给事件挂钩（可选；装配后用户创建/停用/离职会触发下游账号供给）。
	provision *ProvisioningService
	mfa       *MFAService
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
	links repository.SyncerLinkRepository,
	sessions sessionController,
	audits *AuditRecorder,
	cipher *auth.SecretCipher,
	provision *ProvisioningService,
	mfa *MFAService,
) *UserService {
	return &UserService{
		users: users, orgs: orgs, roles: roles, jobTitles: jobTitles, depts: depts,
		userDepts: userDepts, levels: levels, links: links,
		sessions: sessions, audits: audits, cipher: cipher, provision: provision, mfa: mfa,
	}
}

// ExternalIdentities returns source-system mappings for a local user.
func (s *UserService) ExternalIdentities(ctx context.Context, institutionID int64, name string) ([]*repository.ExternalIdentityRef, error) {
	user, err := s.Get(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	if s.links == nil {
		return []*repository.ExternalIdentityRef{}, nil
	}
	items, err := s.links.ListForLocal(ctx, "user", user.ID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*repository.ExternalIdentityRef{}
	}
	return items, nil
}

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
	} else if org.DefaultPasswordEnc != "" {
		defaultPassword, err := s.defaultPassword(org)
		if err != nil {
			return nil, err
		}
		hash, err := auth.HashPasswordWith(defaultPassword, org.PasswordType, org.PasswordSalt)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
		}
		u.PasswordHash = hash
		u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
		u.PasswordSalt = org.PasswordSalt
	}
	normalizeUserType(u)
	var id int64
	if u.DepartmentID != nil {
		id, err = s.users.CreateWithDepartments(ctx, u, []int64{*u.DepartmentID}, *u.DepartmentID)
	} else {
		id, err = s.users.Create(ctx, u)
	}
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

func (s *UserService) defaultPassword(org *repository.Institution) (string, error) {
	if org == nil || org.DefaultPasswordEnc == "" {
		return "", nil
	}
	if s.cipher == nil {
		return "", bizerrors.New(bizerrors.CodeInternalError, "institution cipher is not configured")
	}
	plain, err := s.cipher.Decrypt(org.DefaultPasswordEnc)
	if err != nil {
		return "", bizerrors.Wrap(bizerrors.CodeInternalError, "解密默认密码失败", err)
	}
	return plain, nil
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
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{name}); err != nil {
		return err
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
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{name}); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "user.reset_password", name, nil)
	return nil
}

// ChangePassword 用户自助修改密码；账号尚无密码时（如 SSO 自动建号）直接完成首次设密，
// 因此 oldPassword 只在已有密码时必须校验。
func (s *UserService) ChangePassword(ctx context.Context, institutionID int64, name, oldPassword, newPassword string) error {
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	initial := u.PasswordHash == ""
	if !initial && !auth.VerifyPasswordWith(oldPassword, u.PasswordHash, u.PasswordType, u.PasswordSalt) {
		return bizerrors.ErrInvalidCredentials
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
	if initial {
		if err := s.users.UpdateInitialPassword(ctx, u, org.PasswordHistoryCount); err != nil {
			// 读取后密码被并发设置：按凭证错误返回，绝不把它当作"仍然没有密码"覆盖。
			if bizerrors.Is(err, bizerrors.CodeConflict) {
				return bizerrors.ErrInvalidCredentials
			}
			return err
		}
	} else if err := s.users.UpdatePassword(ctx, u, org.PasswordHistoryCount); err != nil {
		return err
	}
	if err := invalidateUserSessions(ctx, s.sessions, institutionID, []string{name}); err != nil {
		return err
	}
	action := "user.change_password"
	if initial {
		action = "user.set_password"
	}
	s.audits.Record(ctx, name, institutionID, action, name, nil)
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
