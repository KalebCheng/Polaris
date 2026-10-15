package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 注意：口令本体不落库（迁移 026 已删除 related_password，改为 has_password 标记），
// 列清单必须与之保持一致——否则任何读写都会以 42703（列不存在）失败。
const appAccountColumns = `id, institution_id, application, "user", username, related_username,
	has_password, create_type, status, created_at, updated_at`

// AppAccountRepository 应用账号仓库（PostgreSQL 实现）
type AppAccountRepository struct{ db *dbclient.Client }

// NewAppAccountRepository 创建应用账号仓库
func NewAppAccountRepository(db *dbclient.Client) *AppAccountRepository {
	return &AppAccountRepository{db: db}
}

func scanAppAccount(row interface{ Scan(...interface{}) error }) (*repository.AppAccount, error) {
	var a repository.AppAccount
	err := row.Scan(&a.ID, &a.InstitutionID, &a.Application, &a.User, &a.Username, &a.RelatedUsername,
		&a.HasPassword, &a.CreateType, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create 创建应用账号
func (r *AppAccountRepository) Create(ctx context.Context, a *repository.AppAccount) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO app_accounts (institution_id, application, "user", username,
		related_username, has_password, create_type, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE(NULLIF($8,''),'active')) RETURNING id`,
		a.InstitutionID, a.Application, a.User, a.Username, a.RelatedUsername, a.HasPassword,
		defaultStr(a.CreateType, "automatic"), a.Status,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "app account not found")
	}
	a.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *AppAccountRepository) GetByID(ctx context.Context, id int64) (*repository.AppAccount, error) {
	a, err := scanAppAccount(r.db.QueryRow(ctx, `SELECT `+appAccountColumns+` FROM app_accounts WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "app account not found")
	}
	return a, nil
}

// GetByUserApp 按幂等键 (institutionID, application, user) 查询
func (r *AppAccountRepository) GetByUserApp(ctx context.Context, institutionID int64, application, user string) (*repository.AppAccount, error) {
	a, err := scanAppAccount(r.db.QueryRow(ctx,
		`SELECT `+appAccountColumns+` FROM app_accounts WHERE institution_id=$1 AND application=$2 AND "user"=$3`,
		institutionID, application, user))
	if err != nil {
		return nil, mapError(err, "app account not found")
	}
	return a, nil
}

// Update 更新应用账号
func (r *AppAccountRepository) Update(ctx context.Context, a *repository.AppAccount) error {
	const q = `UPDATE app_accounts SET username=$3, related_username=$4, has_password=$5,
		create_type=$6, status=$7, updated_at=NOW()
		WHERE institution_id=$1 AND id=$2`
	return mapError(r.db.Exec(ctx, q, a.InstitutionID, a.ID, a.Username, a.RelatedUsername,
		a.HasPassword, defaultStr(a.CreateType, "automatic"), defaultStr(a.Status, "active")),
		"app account not found")
}

// Delete 删除应用账号
func (r *AppAccountRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM app_accounts WHERE id=$1`, id), "app account not found")
}

// List 分页列出应用账号
func (r *AppAccountRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.AppAccount, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(username ILIKE ` + p + ` OR related_username ILIKE ` + p + ` OR "user" ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + appAccountColumns + ` FROM app_accounts` + b.where() + b.orderLimit("id DESC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.AppAccount
	for rows.Next() {
		a, err := scanAppAccount(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, a)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计应用账号数量
func (r *AppAccountRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(username ILIKE ` + p + ` OR related_username ILIKE ` + p + ` OR "user" ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM app_accounts`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.AppAccountRepository = (*AppAccountRepository)(nil)
