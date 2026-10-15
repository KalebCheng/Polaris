package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

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

// userQuery 依据通用查询参数拼装条件（组织 / 字段等值 / 关键词 / 附加过滤）。
func userQuery(q repository.ListQuery) *queryBuilder {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
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
		b.add(columnExpr(f.Field) + ` = ` + b.arg(f.Value))
	}
	return b
}

// Create 创建用户
func (r *UserRepository) Create(ctx context.Context, u *repository.User) (int64, error) {
	normalizeUser(u)
	return insertReturningID(ctx, r.db, userTable, u)
}

// GetByID 按 ID 查询
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*repository.User, error) {
	u := &repository.User{}
	if err := selectOne(ctx, r.db, userTable, u, "WHERE id = $1", id); err != nil {
		return nil, err
	}
	return u, nil
}

// GetByName 按组织+账号名查询
func (r *UserRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.User, error) {
	u := &repository.User{}
	if err := selectOne(ctx, r.db, userTable, u, "WHERE institution_id = $1 AND name = $2", institutionID, name); err != nil {
		return nil, err
	}
	return u, nil
}

// GetByEmail 按组织+邮箱查询（大小写不敏感）
func (r *UserRepository) GetByEmail(ctx context.Context, institutionID int64, email string) (*repository.User, error) {
	u := &repository.User{}
	if err := selectOne(ctx, r.db, userTable, u, "WHERE institution_id = $1 AND lower(email) = lower($2)", institutionID, email); err != nil {
		return nil, err
	}
	return u, nil
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
// 不可变列（标识、凭证、登录痕迹、注册来源）一律不参与 UPDATE：
// 标识改名会断掉外部身份绑定；凭证有专用方法（UpdatePassword）以免被"整行写回"覆盖；
// 历史遗留的审计字段同样只由各自的专用方法写入。
func (r *UserRepository) Update(ctx context.Context, u *repository.User) error {
	return updateEntity(ctx, r.db, userTable, u,
		"uid", "name",
		"password_hash", "password_salt", "password_type", "password_updated_at",
		"last_signin_time", "last_signin_ip", "created_ip",
		"signup_application", "register_type", "register_source",
	)
}

// UpdatePassword 更新密码并写入密码历史（事务）。
func (r *UserRepository) UpdatePassword(ctx context.Context, u *repository.User, historyLimit int) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE users SET password_hash=$2, password_type=$3, password_salt=$4,
			password_updated_at=NOW(), updated_at=NOW() WHERE id=$1`,
			u.ID, u.PasswordHash, defaultStr(u.PasswordType, "bcrypt"), u.PasswordSalt)
		if err != nil {
			return mapError(err, "user not found")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO password_history (institution_id, "user", password_hash)
			VALUES ($1,$2,$3)`, u.InstitutionID, u.Name, u.PasswordHash); err != nil {
			return mapError(err, "")
		}
		if historyLimit > 0 {
			_, err := tx.Exec(ctx, `DELETE FROM password_history WHERE institution_id=$1 AND "user"=$2
				AND id NOT IN (
					SELECT id FROM password_history WHERE institution_id=$1 AND "user"=$2
					ORDER BY created_at DESC LIMIT $3)`, u.InstitutionID, u.Name, historyLimit)
			if err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// SoftDelete 软删除用户（保留审计与外部身份绑定）
func (r *UserRepository) SoftDelete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `UPDATE users SET is_deleted=TRUE, is_online=FALSE, updated_at=NOW() WHERE id=$1`, id), "user not found")
}

// List 分页列出用户
func (r *UserRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.User, error) {
	b := userQuery(q)
	return queryEntities[repository.User](ctx, r.db, userTable, b.where(), b.args, "name ASC", q.Offset, q.Limit)
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
	return queryEntities[repository.User](ctx, r.db, userTable,
		"WHERE institution_id = $1 AND name = ANY($2)", []interface{}{institutionID, names}, "name ASC", 0, 0)
}

// CountAdmins 统计组织内仍可登录的管理员数量（"最后一个管理员"防呆用）。
func (r *UserRepository) CountAdmins(ctx context.Context, institutionID int64) (int64, error) {
	return countEntities(ctx, r.db, userTable,
		"WHERE institution_id = $1 AND is_admin = TRUE AND is_deleted = FALSE AND is_forbidden = FALSE",
		institutionID)
}

// SetDepartment 变更单个用户的部门归属（nil = 移出部门）。
func (r *UserRepository) SetDepartment(ctx context.Context, institutionID int64, name string, departmentID *int64) error {
	return mapError(r.db.Exec(ctx, `UPDATE users SET department_id=$3, updated_at=NOW()
		WHERE institution_id=$1 AND name=$2`, institutionID, name, departmentID), "user not found")
}

// ReplaceDepartmentMembers 整体替换部门成员（事务）。
// 未列出的旧成员被移出部门（department_id = NULL），列出的用户被挂到本部门。
func (r *UserRepository) ReplaceDepartmentMembers(ctx context.Context, institutionID, departmentID int64, users []string) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET department_id=NULL, updated_at=NOW()
			WHERE institution_id=$1 AND department_id=$2 AND NOT (name = ANY($3))`,
			institutionID, departmentID, orEmpty(users)); err != nil {
			return mapError(err, "")
		}
		if len(users) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE users SET department_id=$3, updated_at=NOW()
				WHERE institution_id=$1 AND name = ANY($2)`,
				institutionID, users, departmentID); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// UpdateLastSignin 更新最近登录时间/IP
func (r *UserRepository) UpdateLastSignin(ctx context.Context, id int64, at time.Time, ip string) error {
	return mapError(r.db.Exec(ctx, `UPDATE users SET last_signin_time=$2, last_signin_ip=$3, updated_at=NOW() WHERE id=$1`, id, at, ip), "user not found")
}

// SetOnline 设置在线状态
func (r *UserRepository) SetOnline(ctx context.Context, id int64, online bool) error {
	return mapError(r.db.Exec(ctx, `UPDATE users SET is_online=$2, updated_at=NOW() WHERE id=$1`, id, online), "user not found")
}

// PasswordHistory 读取最近 N 条密码哈希
func (r *UserRepository) PasswordHistory(ctx context.Context, institutionID int64, user string, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT password_hash FROM password_history
		WHERE institution_id=$1 AND "user"=$2 ORDER BY created_at DESC LIMIT $3`, institutionID, user, limit)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, mapError(err, "")
		}
		hashes = append(hashes, h)
	}
	return hashes, mapError(rows.Err(), "")
}

const identityColumns = `id, institution_id, "user", provider, provider_user_id, email, created_at, updated_at`

func scanIdentity(row interface{ Scan(...interface{}) error }) (*repository.UserIdentity, error) {
	var i repository.UserIdentity
	err := row.Scan(&i.ID, &i.InstitutionID, &i.User, &i.Provider, &i.ProviderUserID, &i.Email, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// GetIdentity 按 provider + providerUserID 查询外部身份绑定
func (r *UserRepository) GetIdentity(ctx context.Context, provider, providerUserID string) (*repository.UserIdentity, error) {
	i, err := scanIdentity(r.db.QueryRow(ctx, `SELECT `+identityColumns+` FROM user_identities WHERE provider=$1 AND provider_user_id=$2`, provider, providerUserID))
	if err != nil {
		return nil, mapError(err, "identity not bound")
	}
	return i, nil
}

// GetIdentitiesByUser 查询用户的外部身份绑定
func (r *UserRepository) GetIdentitiesByUser(ctx context.Context, institutionID int64, user string) ([]*repository.UserIdentity, error) {
	rows, err := r.db.Query(ctx, `SELECT `+identityColumns+` FROM user_identities WHERE institution_id=$1 AND "user"=$2 ORDER BY id ASC`, institutionID, user)
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
		const q = `INSERT INTO users (uid, institution_id, name, type, password_hash, password_type,
			display_name, avatar, email, email_verified, language, gender, signup_application,
			register_type, register_source, created_ip)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`
		if err := tx.QueryRow(ctx, q, u.UID, u.InstitutionID, u.Name, u.Type,
			u.PasswordHash, u.PasswordType, u.DisplayName, u.Avatar, u.Email,
			u.EmailVerified, u.Language, u.Gender, u.SignupApplication, u.RegisterType,
			u.RegisterSource, u.CreatedIP).Scan(&id); err != nil {
			return mapError(err, "user not found")
		}
		identity.InstitutionID = u.InstitutionID
		identity.User = u.Name
		if _, err := tx.Exec(ctx, `INSERT INTO user_identities (institution_id, "user", provider, provider_user_id, email)
			VALUES ($1,$2,$3,$4,$5)`, identity.InstitutionID, identity.User, identity.Provider,
			identity.ProviderUserID, identity.Email); err != nil {
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
	err := r.db.QueryRow(ctx, `INSERT INTO user_identities (institution_id, "user", provider, provider_user_id, email)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, identity.InstitutionID, identity.User, identity.Provider,
		identity.ProviderUserID, identity.Email).Scan(&id)
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
