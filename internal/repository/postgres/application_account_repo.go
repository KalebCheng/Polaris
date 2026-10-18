package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 注意：口令本体不落库（迁移 026 已删除 related_password，改为 has_password 标记），
// 列清单必须与之保持一致——否则任何读写都会以 42703（列不存在）失败。
const applicationAccountColumns = `t.id, t.institution_id, t.application_id, t.user_id, t.username, t.related_username,
	t.has_password, t.create_type, t.status, t.created_at, t.updated_at`

// applicationAccountSelect：读取时把主键解析回展示名（应用 code / 用户账号名）；存储只认主键（§3.8）
const applicationAccountSelect = `SELECT ` + applicationAccountColumns + `,
	COALESCE((SELECT a.code FROM applications a WHERE a.id = t.application_id), '') AS application,
	COALESCE((SELECT u.name FROM users u WHERE u.id = t.user_id), '') AS "user"
	FROM application_accounts t`

// ApplicationAccountRepository 应用账号仓库（PostgreSQL 实现）
type ApplicationAccountRepository struct{ db *dbclient.Client }

// NewApplicationAccountRepository 创建应用账号仓库
func NewApplicationAccountRepository(db *dbclient.Client) *ApplicationAccountRepository {
	return &ApplicationAccountRepository{db: db}
}

func scanApplicationAccount(row interface{ Scan(...interface{}) error }) (*repository.ApplicationAccount, error) {
	var a repository.ApplicationAccount
	err := row.Scan(&a.ID, &a.InstitutionID, &a.ApplicationID, &a.UserID, &a.Username, &a.RelatedUsername,
		&a.HasPassword, &a.CreateType, &a.Status, &a.CreatedAt, &a.UpdatedAt, &a.Application, &a.User)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create 创建应用账号
func (r *ApplicationAccountRepository) Create(ctx context.Context, a *repository.ApplicationAccount) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO application_accounts (institution_id, application_id, user_id, username,
		related_username, has_password, create_type, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE(NULLIF($8,''),'active')) RETURNING id`,
		a.InstitutionID, a.ApplicationID, a.UserID, a.Username, a.RelatedUsername, a.HasPassword,
		defaultStr(a.CreateType, "automatic"), a.Status,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "app account not found")
	}
	a.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *ApplicationAccountRepository) GetByID(ctx context.Context, id int64) (*repository.ApplicationAccount, error) {
	a, err := scanApplicationAccount(r.db.QueryRow(ctx, applicationAccountSelect+` WHERE t.id=$1`, id))
	if err != nil {
		return nil, mapError(err, "app account not found")
	}
	return a, nil
}

// GetByUserApp 按幂等键 (institutionID, applicationID, userID) 查询
func (r *ApplicationAccountRepository) GetByUserApp(ctx context.Context, institutionID int64, applicationID, userID int64) (*repository.ApplicationAccount, error) {
	a, err := scanApplicationAccount(r.db.QueryRow(ctx,
		applicationAccountSelect+` WHERE t.institution_id=$1 AND t.application_id=$2 AND t.user_id=$3`,
		institutionID, applicationID, userID))
	if err != nil {
		return nil, mapError(err, "app account not found")
	}
	return a, nil
}

// Update 更新应用账号
func (r *ApplicationAccountRepository) Update(ctx context.Context, a *repository.ApplicationAccount) error {
	const q = `UPDATE application_accounts SET username=$3, related_username=$4, has_password=$5,
		create_type=$6, status=$7, updated_at=NOW()
		WHERE institution_id=$1 AND id=$2`
	return mapError(r.db.Exec(ctx, q, a.InstitutionID, a.ID, a.Username, a.RelatedUsername,
		a.HasPassword, defaultStr(a.CreateType, "automatic"), defaultStr(a.Status, "active")),
		"app account not found")
}

// Delete 删除应用账号
func (r *ApplicationAccountRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM application_accounts WHERE id=$1`, id), "app account not found")
}

// List 分页列出应用账号
func (r *ApplicationAccountRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.ApplicationAccount, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.username ILIKE ` + p + ` OR t.related_username ILIKE ` + p + `
			OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	sql := applicationAccountSelect + b.where() + b.orderLimit("t.id DESC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.ApplicationAccount
	for rows.Next() {
		a, err := scanApplicationAccount(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, a)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计应用账号数量
func (r *ApplicationAccountRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.username ILIKE ` + p + ` OR t.related_username ILIKE ` + p + `
			OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM application_accounts t`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.ApplicationAccountRepository = (*ApplicationAccountRepository)(nil)
