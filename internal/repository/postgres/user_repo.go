package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const userTable = "users"

// UserRepository 用户仓库（PostgreSQL 实现）。
//
// 列清单完全由 `db` tag 反射推导（§3.6：>40 列实体禁止手写三份列清单）。
// 时间戳列由数据库负责生成：INSERT 不写 created_at / updated_at，创建后随 RETURNING 回填。
type UserRepository struct {
	db *dbclient.Client
}

// NewUserRepository 创建用户仓库
func NewUserRepository(db *dbclient.Client) *UserRepository { return &UserRepository{db: db} }

// DB 暴露底层客户端（供需要事务编排的服务使用）
func (r *UserRepository) DB() *dbclient.Client { return r.db }

// normalizeUser 补齐 NOT NULL 列的最小默认值（列级默认 + 与历史行为一致）。
func normalizeUser(u *repository.User) {
	if u.Type == "" {
		u.Type = "normal"
	}
	if u.PasswordType == "" {
		u.PasswordType = "bcrypt"
	}
}

const userDetailFrom = `
FROM users u
LEFT JOIN user_passwords p ON p.user_id = u.id
LEFT JOIN user_registrations r ON r.user_id = u.id
LEFT JOIN user_login_stats s ON s.user_id = u.id`

const userDetailExtraColumns = `
COALESCE(p.password_hash, ''),
COALESCE(p.password_type, 'bcrypt'),
COALESCE(p.password_salt, ''),
p.password_updated_at,
COALESCE(r.signup_application_code, ''),
COALESCE(r.signup_application_id, 0),
COALESCE(r.register_type, ''),
COALESCE(r.register_source, ''),
COALESCE(r.registration_syncer_id, 0),
COALESCE(r.created_ip, ''),
s.last_signin_at,
COALESCE(s.last_signin_ip, ''),
EXISTS (
    SELECT 1 FROM user_mfa_credentials c
    WHERE c.institution_id = u.institution_id
      AND c.user_id = u.id
      AND c.status = 'active'
)`

// userDetailTargets returns scan targets for users columns plus 1:1 child fields.
func userDetailTargets(u *repository.User) ([]interface{}, error) {
	targets, err := scanTargets(u)
	if err != nil {
		return nil, err
	}
	return append(targets,
		&u.PasswordHash, &u.PasswordType, &u.PasswordSalt, &u.PasswordUpdatedAt,
		&u.SignupApplication, &u.SignupApplicationID, &u.RegisterType, &u.RegisterSource,
		&u.RegistrationSyncerID, &u.CreatedIP,
		&u.LastSigninTime, &u.LastSigninIP, &u.MfaEnabled,
	), nil
}

func (r *UserRepository) selectUserWithDetails(ctx context.Context, where string, args ...interface{}) (*repository.User, error) {
	u := &repository.User{}
	cols, err := columnList(u)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(cols, ", ")
	for i := range parts {
		parts[i] = "u." + parts[i]
	}
	targets, err := userDetailTargets(u)
	if err != nil {
		return nil, err
	}
	sql := "SELECT " + strings.Join(parts, ", ") + ", " + userDetailExtraColumns + " " + userDetailFrom + " " + where
	if err := r.db.QueryRow(ctx, sql, args...).Scan(targets...); err != nil {
		return nil, mapError(err, "user not found")
	}
	u.HasPassword = u.PasswordHash != ""
	return u, nil
}

// attachUserDetails fills 1:1 child fields in one batch (no per-row N+1).
func (r *UserRepository) attachUserDetails(ctx context.Context, users []*repository.User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(users))
	byID := make(map[int64]*repository.User, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		ids = append(ids, u.ID)
		byID[u.ID] = u
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.db.Query(ctx, `SELECT u.id, `+userDetailExtraColumns+` `+userDetailFrom+` WHERE u.id = ANY($1)`, ids)
	if err != nil {
		return mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var passwordHash, passwordType, passwordSalt string
		var passwordUpdatedAt *time.Time
		var signupApplication, registerType, registerSource, createdIP string
		var signupApplicationID, registrationSyncerID int64
		var lastSigninTime *time.Time
		var lastSigninIP string
		var mfaEnabled bool
		if err := rows.Scan(
			&id,
			&passwordHash, &passwordType, &passwordSalt, &passwordUpdatedAt,
			&signupApplication, &signupApplicationID, &registerType, &registerSource,
			&registrationSyncerID, &createdIP,
			&lastSigninTime, &lastSigninIP, &mfaEnabled,
		); err != nil {
			return mapError(err, "")
		}
		u := byID[id]
		if u == nil {
			continue
		}
		u.PasswordHash, u.PasswordType, u.PasswordSalt = passwordHash, passwordType, passwordSalt
		u.PasswordUpdatedAt = passwordUpdatedAt
		u.HasPassword = passwordHash != ""
		u.SignupApplication, u.SignupApplicationID = signupApplication, signupApplicationID
		u.RegisterType, u.RegisterSource = registerType, registerSource
		u.RegistrationSyncerID, u.CreatedIP = registrationSyncerID, createdIP
		u.LastSigninTime, u.LastSigninIP = lastSigninTime, lastSigninIP
		u.MfaEnabled = mfaEnabled
	}
	return mapError(rows.Err(), "")
}

// insertUserRelations writes password and registration rows inside the
// user-creation transaction.
func insertUserRelations(ctx context.Context, tx pgx.Tx, u *repository.User) error {
	if u.PasswordHash != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO user_passwords
			(user_id, institution_id, password_hash, password_type, password_salt, password_updated_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,COALESCE($6,NOW()),NOW(),NOW())
			ON CONFLICT (user_id) DO NOTHING`,
			u.ID, u.InstitutionID, u.PasswordHash, defaultStr(u.PasswordType, "bcrypt"), u.PasswordSalt, u.PasswordUpdatedAt); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO password_history
			(institution_id, user_id, password_hash, password_type, password_salt)
			VALUES ($1,$2,$3,$4,$5)`,
			u.InstitutionID, u.ID, u.PasswordHash, defaultStr(u.PasswordType, "bcrypt"), u.PasswordSalt); err != nil {
			return mapError(err, "")
		}
		u.HasPassword = true
	}
	if u.SignupApplication != "" || u.SignupApplicationID != 0 || u.RegisterType != "" ||
		u.RegisterSource != "" || u.RegistrationSyncerID != 0 || u.CreatedIP != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO user_registrations
			(user_id, institution_id, signup_application_id, signup_application_code,
			 register_type, register_source, registration_syncer_id, created_ip, created_at, updated_at)
			VALUES ($1,$2,NULLIF($3,0),$4,$5,$6,NULLIF($7,0),$8,NOW(),NOW())
			ON CONFLICT (user_id) DO NOTHING`,
			u.ID, u.InstitutionID, u.SignupApplicationID, u.SignupApplication,
			u.RegisterType, u.RegisterSource, u.RegistrationSyncerID, u.CreatedIP); err != nil {
			return mapError(err, "")
		}
	}
	return nil
}

// userQuery 依据通用查询参数拼装条件（组织 / 字段等值 / 关键词 / 附加过滤）。
func userQuery(q repository.ListQuery) *queryBuilder {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.DepartmentID != 0 {
		// 多值归属的唯一真相是 user_departments：按部门筛选必须命中任意一条归属。
		b.add(`EXISTS (SELECT 1 FROM user_departments ud WHERE ud.user_id = users.id AND ud.department_id = ` + b.arg(q.DepartmentID) + `)`)
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + ` = ` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + ` OR email ILIKE ` + p + `)`)
	}
	for _, f := range q.Filters {
		if f.Field == "" {
			continue
		}
		// mfa_enabled is no longer stored on users. Filter through an EXISTS
		// on active credentials instead of maintaining a boolean cache.
		if f.Field == "mfa_enabled" {
			bound := false
			switch v := f.Value.(type) {
			case bool:
				bound = v
			case string:
				bound = strings.EqualFold(v, "true")
			}
			clause := `EXISTS (
				SELECT 1 FROM user_mfa_credentials c
				WHERE c.institution_id = users.institution_id
				  AND c.user_id = users.id
				  AND c.status = 'active'
			)`
			if !bound {
				clause = "NOT " + clause
			}
			b.add(clause)
			continue
		}
		b.add(columnExpr(f.Field) + ` = ` + b.arg(f.Value))
	}
	return b
}

// Create 创建用户
func (r *UserRepository) Create(ctx context.Context, u *repository.User) (int64, error) {
	normalizeUser(u)
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = insertReturningID(ctx, tx, userTable, u)
		if err != nil {
			return err
		}
		u.ID = id
		return insertUserRelations(ctx, tx, u)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// CreateWithDepartments 事务内创建用户并写入多值部门归属（主部门用 is_primary 表达）。
func (r *UserRepository) CreateWithDepartments(
	ctx context.Context, u *repository.User, departmentIDs []int64, primaryID int64,
) (int64, error) {
	normalizeUser(u)
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = insertReturningID(ctx, tx, userTable, u)
		if err != nil {
			return err
		}
		u.ID = id
		if err := insertUserRelations(ctx, tx, u); err != nil {
			return err
		}
		seen := make(map[int64]bool, len(departmentIDs))
		for _, deptID := range departmentIDs {
			if deptID == 0 || seen[deptID] {
				continue
			}
			seen[deptID] = true
			if _, err := tx.Exec(ctx, `INSERT INTO user_departments
				(institution_id, user_id, department_id, is_primary)
				VALUES ($1,$2,$3,$4)`, u.InstitutionID, id, deptID, deptID == primaryID); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetByID 按 ID 查询
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*repository.User, error) {
	return r.selectUserWithDetails(ctx, "WHERE u.id = $1", id)
}

// GetByUID 按全局唯一 UID 查询（标准 token 的 sub 即 UID）。
func (r *UserRepository) GetByUID(ctx context.Context, uid string) (*repository.User, error) {
	return r.selectUserWithDetails(ctx, "WHERE u.uid = $1", uid)
}

// GetByName 按组织+账号名查询
func (r *UserRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.User, error) {
	return r.selectUserWithDetails(ctx, "WHERE u.institution_id = $1 AND u.name = $2", institutionID, name)
}

// GetByEmail 按组织+邮箱查询（大小写不敏感）
func (r *UserRepository) GetByEmail(ctx context.Context, institutionID int64, email string) (*repository.User, error) {
	return r.selectUserWithDetails(ctx, "WHERE u.institution_id = $1 AND lower(u.email) = lower($2)", institutionID, email)
}

// FindMatchCandidates 按提供商配置的本地字段查候选用户。
// 字段白名单防止把任意列名拼进 SQL；最多返回 limit 条，调用方据此判断唯一命中。
func (r *UserRepository) FindMatchCandidates(
	ctx context.Context, institutionID int64, field, value string, limit int,
) ([]*repository.User, error) {
	if limit <= 0 || limit > 10 {
		limit = 2
	}
	where, err := matchCandidateWhere(field)
	if err != nil {
		return nil, err
	}
	items, err := queryEntities[repository.User](ctx, r.db, userTable, "WHERE "+where,
		[]interface{}{institutionID, value}, "", 0, limit)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindIdentityLinkMatchCandidates scopes identity matching to one inbound syncer.
//
// field 是连接器声明的源字段名（external_id 走映射主键列，其余走 syncer_user_links.raw，
// 键名与连接器的 source_fields 一致）。字段名以参数绑定给 jsonb 的 ->> 操作符，无注入面。
func (r *UserRepository) FindIdentityLinkMatchCandidates(
	ctx context.Context, institutionID, syncerID int64, field, value string, limit int,
) ([]*repository.User, error) {
	if syncerID <= 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("match_syncer_id", syncerID)
	}
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, bizerrors.ErrInvalidParams.WithDetail("match_syncer_field", field)
	}
	if limit <= 0 || limit > 10 {
		limit = 2
	}
	args := []interface{}{institutionID, syncerID, value}
	condition := `l.external_id = $3 AND l.external_id <> ''`
	if field != "external_id" {
		condition = `l.raw ->> $4 = $3 AND $3 <> ''`
		args = append(args, field)
	}
	where := `institution_id = $1 AND is_deleted = FALSE AND id IN (
		SELECT l.user_id FROM syncer_user_links l
		WHERE l.institution_id = $1 AND l.syncer_id = $2 AND ` + condition + `
	)`
	items, err := queryEntities[repository.User](ctx, r.db, userTable, "WHERE "+where,
		args, "", 0, limit)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindUsersByIdentityAttribute 按提供商的原始身份字段查本地用户（反向认领）。
//
// 用于"同源身份连接"：同步侧拿到连接源字段的值后，找回此前由 SSO 自动建号的同一用户，
// 避免两条链路各自建号。属性以 jsonb 包含查询（attributes @> {...}）命中 GIN 索引。
func (r *UserRepository) FindUsersByIdentityAttribute(
	ctx context.Context, institutionID, providerID int64, attribute, value string, limit int,
) ([]*repository.User, error) {
	attribute = strings.TrimSpace(attribute)
	if providerID <= 0 || attribute == "" || value == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 10 {
		limit = 2
	}
	where := `institution_id = $1 AND is_deleted = FALSE AND id IN (
		SELECT user_id FROM user_identities
		WHERE institution_id = $1 AND provider_id = $2
		  AND attributes @> jsonb_build_object($3::text, $4::text)
	)`
	return queryEntities[repository.User](ctx, r.db, userTable, "WHERE "+where,
		[]interface{}{institutionID, providerID, attribute, value}, "", 0, limit)
}

// matchCandidateWhere 返回提供商唯一性匹配的本地字段条件。
// 白名单防止把任意列名拼进 SQL；身份链接匹配必须经 FindIdentityLinkMatchCandidates 指定连接。
func matchCandidateWhere(field string) (string, error) {
	strategy, ok := userMatchStrategies[field]
	if !ok {
		return "", bizerrors.ErrInvalidParams.WithDetail("match_local_field", field)
	}
	return strategy.where(), nil
}

// findAcrossInstitutions 跨组织查找（同一账号名/邮箱/手机号可能存在于多个组织，故返回列表用于识别歧义）
func (r *UserRepository) findAcrossInstitutions(ctx context.Context, where string, value string, limit int) ([]*repository.User, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	return queryEntities[repository.User](ctx, r.db, userTable, "WHERE "+where, []interface{}{value}, "", 0, limit)
}

// FindByNameAcrossInstitutions 跨组织按账号名查找
func (r *UserRepository) FindByNameAcrossInstitutions(ctx context.Context, name string, limit int) ([]*repository.User, error) {
	return r.findAcrossInstitutions(ctx, `name = $1 AND is_deleted = FALSE`, name, limit)
}

// FindByEmailAcrossInstitutions 跨组织按邮箱查找
func (r *UserRepository) FindByEmailAcrossInstitutions(ctx context.Context, email string, limit int) ([]*repository.User, error) {
	return r.findAcrossInstitutions(ctx, `lower(email) = lower($1) AND email <> '' AND is_deleted = FALSE`, email, limit)
}

// FindByPhoneAcrossInstitutions 跨组织按手机号查找
func (r *UserRepository) FindByPhoneAcrossInstitutions(ctx context.Context, phone string, limit int) ([]*repository.User, error) {
	return r.findAcrossInstitutions(ctx, `phone = $1 AND phone <> '' AND is_deleted = FALSE`, phone, limit)
}

// Update 更新用户。
//
// Identity columns stay immutable: renaming would break external identity links.
// Password, login stats, and registration fields are db:"-" child-table fields,
// so a whole-row update cannot overwrite them.
func (r *UserRepository) Update(ctx context.Context, u *repository.User) error {
	return updateEntity(ctx, r.db, userTable, u, "uid", "name")
}

// UpdatePassword 更新密码并写入密码历史（事务）。
func (r *UserRepository) UpdatePassword(ctx context.Context, u *repository.User, historyLimit int) error {
	return r.writePassword(ctx, u, historyLimit, false)
}

// UpdateInitialPassword 仅在当前密码为空时写入初始密码。
//
// 为什么必须条件更新：服务层"读到 PasswordHash 为空"与"写入新密码"之间若有人（管理员重置 /
// 另一个标签页）刚设置了密码，不加条件的 UPDATE 会让本次请求以"首次设密"身份覆盖真实密码，
// 等于绕过旧密码校验。0 行影响统一按凭证冲突返回，由服务层收敛为 INVALID_CREDENTIALS。
func (r *UserRepository) UpdateInitialPassword(ctx context.Context, u *repository.User, historyLimit int) error {
	return r.writePassword(ctx, u, historyLimit, true)
}

func (r *UserRepository) writePassword(ctx context.Context, u *repository.User, historyLimit int, initialOnly bool) error {
	passwordType := defaultStr(u.PasswordType, "bcrypt")
	updatedAt := time.Now()
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if initialOnly {
			tag, err := tx.Exec(ctx, `INSERT INTO user_passwords
				(user_id, institution_id, password_hash, password_type, password_salt, password_updated_at, created_at, updated_at)
				SELECT id, institution_id, $2, $3, $4, NOW(), NOW(), NOW()
				FROM users WHERE id = $1
				ON CONFLICT (user_id) DO NOTHING`,
				u.ID, u.PasswordHash, passwordType, u.PasswordSalt)
			if err != nil {
				return mapError(err, "user not found")
			}
			if tag.RowsAffected() == 0 {
				return bizerrors.New(bizerrors.CodeConflict, "password already set")
			}
		} else {
			tag, err := tx.Exec(ctx, `UPDATE user_passwords
				SET password_hash=$2, password_type=$3, password_salt=$4,
				    password_updated_at=NOW(), updated_at=NOW()
				WHERE user_id=$1`,
				u.ID, u.PasswordHash, passwordType, u.PasswordSalt)
			if err != nil {
				return mapError(err, "user not found")
			}
			if tag.RowsAffected() == 0 {
				tag, err = tx.Exec(ctx, `INSERT INTO user_passwords
					(user_id, institution_id, password_hash, password_type, password_salt, password_updated_at, created_at, updated_at)
					SELECT id, institution_id, $2, $3, $4, NOW(), NOW(), NOW()
					FROM users WHERE id = $1`,
					u.ID, u.PasswordHash, passwordType, u.PasswordSalt)
				if err != nil {
					return mapError(err, "user not found")
				}
				if tag.RowsAffected() == 0 {
					return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
				}
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO password_history
			(institution_id, user_id, password_hash, password_type, password_salt)
			VALUES ($1,$2,$3,$4,$5)`,
			u.InstitutionID, u.ID, u.PasswordHash, passwordType, u.PasswordSalt); err != nil {
			return mapError(err, "")
		}
		if historyLimit > 0 {
			_, err := tx.Exec(ctx, `DELETE FROM password_history WHERE institution_id=$1 AND user_id=$2
				AND id NOT IN (
					SELECT id FROM password_history WHERE institution_id=$1 AND user_id=$2
					ORDER BY created_at DESC, id DESC LIMIT $3)`, u.InstitutionID, u.ID, historyLimit)
			if err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
	if err == nil {
		u.PasswordUpdatedAt = &updatedAt
		u.PasswordType = passwordType
		u.HasPassword = true
	}
	return err
}

// SoftDelete 软删除用户（保留审计与外部身份绑定）
func (r *UserRepository) SoftDelete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `UPDATE users SET is_deleted=TRUE, updated_at=NOW() WHERE id=$1`, id), "user not found")
}

// List 分页列出用户
func (r *UserRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.User, error) {
	b := userQuery(q)
	items, err := queryEntities[repository.User](ctx, r.db, userTable, b.where(), b.args, "name ASC", q.Offset, q.Limit)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// Count 统计用户数量
func (r *UserRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := userQuery(q)
	return countEntities(ctx, r.db, userTable, b.where(), b.args...)
}

// ListByNames 按账号名批量取用户（限定在指定组织内）。
func (r *UserRepository) ListByNames(ctx context.Context, institutionID int64, names []string) ([]*repository.User, error) {
	if len(names) == 0 {
		return []*repository.User{}, nil
	}
	items, err := queryEntities[repository.User](ctx, r.db, userTable,
		"WHERE institution_id = $1 AND name = ANY($2)", []interface{}{institutionID, names}, "name ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListByPhones returns users matching any of the supplied phone spellings.
func (r *UserRepository) ListByPhones(ctx context.Context, institutionID int64, phones []string) ([]*repository.User, error) {
	if len(phones) == 0 {
		return []*repository.User{}, nil
	}
	items, err := queryEntities[repository.User](ctx, r.db, userTable,
		"WHERE institution_id = $1 AND phone = ANY($2)", []interface{}{institutionID, phones}, "phone ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListByIDs returns users by primary key within the institution.
func (r *UserRepository) ListByIDs(ctx context.Context, institutionID int64, ids []int64) ([]*repository.User, error) {
	if len(ids) == 0 {
		return []*repository.User{}, nil
	}
	items, err := queryEntities[repository.User](ctx, r.db, userTable,
		"WHERE institution_id = $1 AND id = ANY($2)", []interface{}{institutionID, ids}, "name ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.attachUserDetails(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ExistingIDs returns non-deleted user IDs that belong to the institution.
func (r *UserRepository) ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT id FROM users
		WHERE institution_id=$1 AND is_deleted=FALSE AND id=ANY($2)`, institutionID, ids)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = true
	}
	return out, mapError(rows.Err(), "")
}

// CountAdmins 统计组织内仍可登录的管理员数量（"最后一个管理员"防呆用）。
func (r *UserRepository) CountAdmins(ctx context.Context, institutionID int64) (int64, error) {
	return countEntities(ctx, r.db, userTable,
		"WHERE institution_id = $1 AND is_admin = TRUE AND is_deleted = FALSE AND is_forbidden = FALSE",
		institutionID)
}

// UpdateLastSignin 更新最近登录时间/IP
func (r *UserRepository) UpdateLastSignin(ctx context.Context, id int64, at time.Time, ip string) error {
	tag, err := r.db.ExecTag(ctx, `INSERT INTO user_login_stats
		(user_id, institution_id, last_signin_at, last_signin_ip, last_seen_at, updated_at)
		SELECT id, institution_id, $2, $3, $2, NOW()
		FROM users WHERE id = $1
		ON CONFLICT (user_id) DO UPDATE SET
			last_signin_at = EXCLUDED.last_signin_at,
			last_signin_ip = EXCLUDED.last_signin_ip,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = NOW()`, id, at, ip)
	if err != nil {
		return mapError(err, "user not found")
	}
	if tag.RowsAffected() == 0 {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	return nil
}

// ListByRegistrationSyncer returns users created by an inbound syncer; used as
// the soft-delete fallback for historical rows.
func (r *UserRepository) ListByRegistrationSyncer(ctx context.Context, institutionID, syncerID int64, limit int) ([]*repository.User, error) {
	if syncerID <= 0 {
		return []*repository.User{}, nil
	}
	return queryEntities[repository.User](ctx, r.db, userTable,
		`WHERE institution_id = $1 AND is_deleted = FALSE AND id IN (
			SELECT user_id FROM user_registrations
			WHERE institution_id = $1 AND registration_syncer_id = $2
		)`, []interface{}{institutionID, syncerID}, "name ASC", 0, limit)
}

// PasswordHistory returns recent passwords with their historical algorithm and salt.
func (r *UserRepository) PasswordHistory(ctx context.Context, institutionID int64, user string, limit int) ([]repository.PasswordHistoryEntry, error) {
	rows, err := r.db.Query(ctx, `SELECT h.password_hash, h.password_type, h.password_salt FROM password_history h
		JOIN users u ON u.id = h.user_id
		WHERE h.institution_id=$1 AND u.name=$2 ORDER BY h.created_at DESC, h.id DESC LIMIT $3`, institutionID, user, limit)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	items := make([]repository.PasswordHistoryEntry, 0)
	for rows.Next() {
		var item repository.PasswordHistoryEntry
		if err := rows.Scan(&item.PasswordHash, &item.PasswordType, &item.PasswordSalt); err != nil {
			return nil, mapError(err, "")
		}
		items = append(items, item)
	}
	return items, mapError(rows.Err(), "")
}

const identityColumns = `i.id, i.institution_id, i.user_id, i.provider_id, i.provider_user_id, i.email, i.attributes, i.created_at, i.updated_at,
	COALESCE((SELECT u.name FROM users u WHERE u.id = i.user_id), '') AS "user",
	COALESCE((SELECT p.name FROM providers p WHERE p.id = i.provider_id), '') AS provider`

const identitySelect = `SELECT ` + identityColumns + ` FROM user_identities i`

func scanIdentity(row interface{ Scan(...interface{}) error }) (*repository.UserIdentity, error) {
	var i repository.UserIdentity
	var attributes []byte
	err := row.Scan(&i.ID, &i.InstitutionID, &i.UserID, &i.ProviderID, &i.ProviderUserID, &i.Email, &attributes,
		&i.CreatedAt, &i.UpdatedAt, &i.User, &i.Provider)
	if err != nil {
		return nil, err
	}
	i.Attributes = decodeStringMap(attributes)
	return &i, nil
}

// GetIdentity 按 provider + providerUserID 查询外部身份绑定
func (r *UserRepository) GetIdentity(ctx context.Context, institutionID int64, provider, providerUserID string) (*repository.UserIdentity, error) {
	i, err := scanIdentity(r.db.QueryRow(ctx, identitySelect+` WHERE i.institution_id=$1 AND i.provider_id =
		(SELECT id FROM providers WHERE institution_id=$1 AND name=$2) AND i.provider_user_id=$3`,
		institutionID, provider, providerUserID))
	if err != nil {
		return nil, mapError(err, "identity not bound")
	}
	return i, nil
}

// GetIdentitiesByUser 查询用户的外部身份绑定
func (r *UserRepository) GetIdentitiesByUser(ctx context.Context, institutionID int64, user string) ([]*repository.UserIdentity, error) {
	rows, err := r.db.Query(ctx, identitySelect+` WHERE i.institution_id=$1 AND i.user_id =
		(SELECT id FROM users WHERE institution_id=$1 AND name=$2) ORDER BY i.id ASC`, institutionID, user)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.UserIdentity
	for rows.Next() {
		i, err := scanIdentity(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, i)
	}
	return result, mapError(rows.Err(), "")
}

// CreateWithIdentity 事务内创建用户并绑定外部身份（JIT 供给；红线 §3.1-6）
func (r *UserRepository) CreateWithIdentity(ctx context.Context, u *repository.User, identity *repository.UserIdentity) (int64, error) {
	normalizeUser(u)
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = insertReturningID(ctx, tx, userTable, u)
		if err != nil {
			return err
		}
		u.ID = id
		if err := insertUserRelations(ctx, tx, u); err != nil {
			return err
		}
		identity.InstitutionID = u.InstitutionID
		identity.User = u.Name
		if _, err := tx.Exec(ctx, `INSERT INTO user_identities (institution_id, user_id, provider_id, provider_user_id, email, attributes)
			VALUES ($1,$2,(SELECT id FROM providers WHERE institution_id=$1 AND name=$3 LIMIT 1),$4,$5,$6)`,
			identity.InstitutionID, u.ID, identity.Provider, identity.ProviderUserID, identity.Email,
			encodeJSONB(identity.Attributes)); err != nil {
			return mapError(err, "")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	u.ID = id
	return id, nil
}

// BindIdentity 绑定外部身份
func (r *UserRepository) BindIdentity(ctx context.Context, identity *repository.UserIdentity) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO user_identities (institution_id, user_id, provider_id, provider_user_id, email, attributes)
		SELECT $1, u.id, p.id, $4, $5, $6
		FROM users u LEFT JOIN providers p ON p.institution_id = $1 AND p.name = $3
		WHERE u.institution_id = $1 AND u.name = $2
		RETURNING id`, identity.InstitutionID, identity.User, identity.Provider,
		identity.ProviderUserID, identity.Email, encodeJSONB(identity.Attributes)).Scan(&id)
	if err != nil {
		return 0, mapError(err, "")
	}
	identity.ID = id
	return id, nil
}

// UnbindIdentity 解绑外部身份
func (r *UserRepository) UnbindIdentity(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM user_identities WHERE id=$1`, id), "identity not bound")
}

var _ repository.UserRepository = (*UserRepository)(nil)

// CreateManyWithRelations 事务内批量创建用户并写入部门 / 职务 / 角色关联（导入批次）。
//
// 事务内再查一次账号名与邮箱冲突：导入侧的预检与写入之间可能被并发导入插入同名行，
// 唯一约束虽能兜底，但 23505 无法指出是哪一行，按批重试时往返次数不可控。
func (r *UserRepository) CreateManyWithRelations(ctx context.Context, items []repository.UserBatchItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		for i := range items {
			item := &items[i]
			u := item.User
			normalizeUser(u)
			var clash int64
			err := tx.QueryRow(ctx, `SELECT 1 FROM users
				WHERE institution_id=$1
				  AND (name=$2 OR ($3 <> '' AND lower(email)=lower($3)) OR ($4 <> '' AND phone=$4))
				LIMIT 1`,
				u.InstitutionID, u.Name, u.Email, u.Phone).Scan(&clash)
			if err == nil {
				return bizerrors.New(bizerrors.CodeConflict, "user already exists").WithDetail("user", u.Name)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return mapError(err, "")
			}
			id, err := insertReturningID(ctx, tx, userTable, u)
			if err != nil {
				return mapError(err, "")
			}
			u.ID = id
			if err := insertUserRelations(ctx, tx, u); err != nil {
				return err
			}
			seenDepts := make(map[int64]bool, len(item.Departments))
			for _, deptID := range item.Departments {
				if deptID == 0 || seenDepts[deptID] {
					continue
				}
				seenDepts[deptID] = true
				if _, err := tx.Exec(ctx, `INSERT INTO user_departments
					(institution_id, user_id, department_id, is_primary)
					VALUES ($1,$2,$3,$4)`, u.InstitutionID, id, deptID, deptID == item.PrimaryID); err != nil {
					return mapError(err, "")
				}
			}
			for _, titleID := range uniqueIDs(item.JobTitleIDs) {
				if _, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
					VALUES ($1,$2) ON CONFLICT DO NOTHING`, titleID, id); err != nil {
					return mapError(err, "")
				}
			}
			for _, roleID := range uniqueIDs(item.RoleIDs) {
				if _, err := tx.Exec(ctx, `INSERT INTO role_users (role_id, user_id)
					VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, id); err != nil {
					return mapError(err, "")
				}
			}
		}
		return nil
	})
}

func uniqueIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
