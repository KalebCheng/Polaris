package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const permissionColumns = `id, institution_id, name, display_name, description, resource_type,
	resources, actions, effect, is_enabled, expire_time, submitter, approver, approve_time, state,
	created_at, updated_at`

// PermissionRepository 权限仓库（PostgreSQL 实现）
type PermissionRepository struct{ db *dbclient.Client }

// NewPermissionRepository 创建权限仓库
func NewPermissionRepository(db *dbclient.Client) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func scanPermission(row interface{ Scan(...interface{}) error }) (*repository.Permission, error) {
	var p repository.Permission
	err := row.Scan(&p.ID, &p.InstitutionID, &p.Name, &p.DisplayName, &p.Description, &p.ResourceType,
		&p.Resources, &p.Actions, &p.Effect, &p.IsEnabled, &p.ExpireTime, &p.Submitter, &p.Approver,
		&p.ApproveTime, &p.State, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create 创建权限
func (r *PermissionRepository) Create(ctx context.Context, p *repository.Permission) (int64, error) {
	return r.createOrSetSubjects(ctx, p, nil, nil, nil)
}

func (r *PermissionRepository) createOrSetSubjects(ctx context.Context, p *repository.Permission, userIDs, jobTitleIDs, roleIDs []int64) (int64, error) {
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO permissions (institution_id, name, display_name, description,
			resource_type, resources, actions, effect, is_enabled, expire_time, submitter, approver,
			approve_time, state)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
			p.InstitutionID, p.Name, p.DisplayName, p.Description, p.ResourceType, orEmpty(p.Resources), orEmpty(p.Actions),
			defaultStr(p.Effect, "Allow"), p.IsEnabled, p.ExpireTime, p.Submitter, p.Approver,
			p.ApproveTime, defaultStr(p.State, "Approved")).Scan(&id); err != nil {
			return mapError(err, "permission not found")
		}
		return insertPermissionSubjects(ctx, tx, id, userIDs, jobTitleIDs, roleIDs)
	})
	if err != nil {
		return 0, err
	}
	p.ID = id
	return id, nil
}

func insertPermissionSubjects(ctx context.Context, tx pgx.Tx, id int64, userIDs, jobTitleIDs, roleIDs []int64) error {
	var institutionID int64
	if err := tx.QueryRow(ctx, `SELECT institution_id FROM permissions WHERE id=$1`, id).Scan(&institutionID); err != nil {
		return mapError(err, "permission not found")
	}
	for _, userID := range userIDs {
		if err := ensureTenantEntity(ctx, tx, "users", institutionID, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO permission_users (permission_id, user_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, userID); err != nil {
			return mapError(err, "")
		}
	}
	for _, jobTitleID := range jobTitleIDs {
		if err := ensureTenantEntity(ctx, tx, "job_titles", institutionID, jobTitleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO permission_job_titles (permission_id, job_title_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, jobTitleID); err != nil {
			return mapError(err, "")
		}
	}
	for _, roleID := range roleIDs {
		if err := ensureTenantEntity(ctx, tx, "roles", institutionID, roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO permission_roles (permission_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, roleID); err != nil {
			return mapError(err, "")
		}
	}
	return nil
}

// GetByID 按 ID 查询（装配主体集合）
func (r *PermissionRepository) GetByID(ctx context.Context, id int64) (*repository.Permission, error) {
	p, err := scanPermission(r.db.QueryRow(ctx, `SELECT `+permissionColumns+` FROM permissions WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "permission not found")
	}
	if err := r.loadSubjects(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetByName 按组织+名称查询
func (r *PermissionRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Permission, error) {
	p, err := scanPermission(r.db.QueryRow(ctx, `SELECT `+permissionColumns+` FROM permissions WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "permission not found")
	}
	if err := r.loadSubjects(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PermissionRepository) references(ctx context.Context, sql string, args ...interface{}) ([]repository.RelationRef, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []repository.RelationRef{}
	for rows.Next() {
		var item repository.RelationRef
		if err := rows.Scan(&item.ID, &item.Name, &item.DisplayName); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, item)
	}
	return out, mapError(rows.Err(), "")
}

func (r *PermissionRepository) loadSubjects(ctx context.Context, p *repository.Permission) error {
	var err error
	if p.Users, err = r.references(ctx, `SELECT u.id, u.name, u.display_name FROM permission_users pu
		JOIN users u ON u.id = pu.user_id WHERE pu.permission_id=$1 ORDER BY u.name`, p.ID); err != nil {
		return err
	}
	if p.JobTitles, err = r.references(ctx, `SELECT jt.id, jt.name, jt.display_name FROM permission_job_titles pjt
		JOIN job_titles jt ON jt.id = pjt.job_title_id
		WHERE pjt.permission_id=$1 ORDER BY jt.name`, p.ID); err != nil {
		return err
	}
	if p.Roles, err = r.references(ctx, `SELECT r.id, r.name, r.display_name FROM permission_roles pr
		JOIN roles r ON r.id = pr.role_id
		WHERE pr.permission_id=$1 ORDER BY r.name`, p.ID); err != nil {
		return err
	}
	return nil
}

// Update 更新权限（含主体集合，事务）
func (r *PermissionRepository) Update(ctx context.Context, p *repository.Permission) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE permissions SET display_name=$2, description=$3, resource_type=$4,
			resources=$5, actions=$6, effect=$7, is_enabled=$8, expire_time=$9, state=$10, updated_at=NOW()
			WHERE id=$1`, p.ID, p.DisplayName, p.Description, p.ResourceType, orEmpty(p.Resources), orEmpty(p.Actions),
			defaultStr(p.Effect, "Allow"), p.IsEnabled, p.ExpireTime, defaultStr(p.State, "Approved")); err != nil {
			return mapError(err, "permission not found")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permission_users WHERE permission_id=$1`, p.ID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permission_job_titles WHERE permission_id=$1`, p.ID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permission_roles WHERE permission_id=$1`, p.ID); err != nil {
			return mapError(err, "")
		}
		return insertPermissionSubjects(ctx, tx, p.ID, relationIDs(p.Users), relationIDs(p.JobTitles), relationIDs(p.Roles))
	})
}

// SetSubjects 整体替换权限绑定的主体集合
func (r *PermissionRepository) SetSubjects(ctx context.Context, permissionID int64, userIDs, jobTitleIDs, roleIDs []int64) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM permission_users WHERE permission_id=$1`, permissionID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permission_job_titles WHERE permission_id=$1`, permissionID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM permission_roles WHERE permission_id=$1`, permissionID); err != nil {
			return mapError(err, "")
		}
		return insertPermissionSubjects(ctx, tx, permissionID, userIDs, jobTitleIDs, roleIDs)
	})
}

func relationIDs(items []repository.RelationRef) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

// Delete 删除权限
func (r *PermissionRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM permissions WHERE id=$1`, id), "permission not found")
}

// List 分页列出权限
func (r *PermissionRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Permission, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + permissionColumns + ` FROM permissions` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, p)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计权限数量
func (r *PermissionRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM permissions`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.PermissionRepository = (*PermissionRepository)(nil)
