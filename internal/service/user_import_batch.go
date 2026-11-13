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

// CreateUserBatchInput 批量创建的单条入参（导入专用）。
//
// 与 CreateUserInput 的区别：部门是多值的（Departments[0] 为主部门），并带上职务 / 角色主键。
type CreateUserBatchInput struct {
	User        *repository.User
	Password    string
	Departments []int64
	JobTitleIDs []int64
	RoleIDs     []int64
	// RoleNames 仅用于审计可读性（如 ["开发者","运维"]）
	RoleNames []string
	// RegisterSource is stored in user_registrations.register_source (the import
	// file name) to identify where the account came from.
	RegisterSource string
}

// CreateUserBatchResult 批量创建结果
type CreateUserBatchResult struct {
	User  *repository.User
	Roles []string
}

// CreateUserBatch 在一批的同一事务内创建多条用户（用户 + 部门 + 职务 + 角色关联），
// 任一失败整批回滚——调用方（导入）按批重试以定位失败行。
//
// 不复用 Create：单条 Create 逐条提交会让"批次"失去原子性；但校验口径保持一致
// （密码策略、默认密码、密码历史长度、审计、下游供给）。
func (s *UserService) CreateUserBatch(
	ctx context.Context, actor string, institutionID int64, items []CreateUserBatchInput,
) ([]CreateUserBatchResult, error) {
	if len(items) == 0 {
		return []CreateUserBatchResult{}, nil
	}
	org, err := s.orgs.GetByID(ctx, institutionID)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", institutionID)
	}
	// 预检（任一条不过则整批拒绝，错误带行内 user 名供导入侧定位）
	for i := range items {
		if err := s.prepareImportUser(org, &items[i]); err != nil {
			return nil, err
		}
	}
	repoItems := make([]repository.UserBatchItem, len(items))
	for i, item := range items {
		repoItems[i] = repository.UserBatchItem{
			User:        item.User,
			PrimaryID:   primaryDepartmentID(item.Departments),
			Departments: item.Departments,
			JobTitleIDs: item.JobTitleIDs,
			RoleIDs:     item.RoleIDs,
		}
	}
	if err := s.users.CreateManyWithRelations(ctx, repoItems); err != nil {
		return nil, err
	}
	out := make([]CreateUserBatchResult, len(items))
	for i, item := range items {
		out[i] = CreateUserBatchResult{User: item.User, Roles: item.RoleNames}
		s.audits.Record(ctx, actor, institutionID, "user.create", item.User.Name, map[string]interface{}{
			"source": "import",
			"roles":  orEmptyStrings(item.RoleNames),
		})
		if s.provision != nil {
			s.provision.TriggerUserProvisioning(ctx, institutionID, item.User.Name)
		}
	}
	return out, nil
}

// prepareImportUser 补齐单条用户可入库的默认值并做与 Create 同口径的校验。
func (s *UserService) prepareImportUser(org *repository.Institution, item *CreateUserBatchInput) error {
	u := item.User
	if u == nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, "user is required")
	}
	u.Name = strings.TrimSpace(u.Name)
	if u.Name == "" {
		return bizerrors.New(bizerrors.CodeInvalidParams, "name is required")
	}
	trimUserFields(u)
	// 导入来源标记（字段被反射映射写库）：区分"谁建的号"与手工建号/同步/JIT
	u.RegisterType = "import"
	u.RegisterSource = item.RegisterSource
	if item.Password == "" {
		return bizerrors.New(bizerrors.CodeInvalidParams, "password is required").WithDetail("user", u.Name)
	}
	if err := auth.FromOptions(org.PasswordOptions).Validate(item.Password); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, err.Error()).WithDetail("user", u.Name)
	}
	hash, err := auth.HashPasswordWith(item.Password, org.PasswordType, org.PasswordSalt)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成密码失败", err)
	}
	u.PasswordHash = hash
	u.PasswordType = auth.NormalizePasswordType(org.PasswordType)
	u.PasswordSalt = org.PasswordSalt
	u.MustChangePassword = true
	now := time.Now()
	u.PasswordUpdatedAt = &now
	if u.UID == "" {
		u.UID = uuid.NewString()
	}
	// DepartmentID 是 API 派生字段（唯一真相是 user_departments.is_primary），由 Departments[0] 表达
	u.DepartmentID = nil
	normalizeUserType(u)
	if len(item.Departments) > 0 {
		primary := item.Departments[0]
		u.DepartmentID = &primary
	}
	item.Password = ""
	return nil
}

func primaryDepartmentID(ids []int64) int64 {
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}
