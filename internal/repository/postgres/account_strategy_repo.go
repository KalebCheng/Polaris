package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const accountStrategyColumns = `t.id, t.institution_id, t.application_id, t.name, t.display_name, t.is_enabled,
	t.mapping, t.policy, t.created_at, t.updated_at`

// accountStrategySelect：读取时把主键解析回应用标识（code）；存储只认主键（§3.8）
const accountStrategySelect = `SELECT ` + accountStrategyColumns + `,
	COALESCE((SELECT a.code FROM applications a WHERE a.id = t.application_id), '') AS application
	FROM account_strategies t`

// AccountStrategyRepository 账号策略仓库（PostgreSQL 实现）
type AccountStrategyRepository struct{ db *dbclient.Client }

// NewAccountStrategyRepository 创建账号策略仓库
func NewAccountStrategyRepository(db *dbclient.Client) *AccountStrategyRepository {
	return &AccountStrategyRepository{db: db}
}

func scanAccountStrategy(row interface{ Scan(...interface{}) error }) (*repository.AccountStrategy, error) {
	var a repository.AccountStrategy
	err := row.Scan(&a.ID, &a.InstitutionID, &a.ApplicationID, &a.Name, &a.DisplayName, &a.IsEnabled,
		&a.Mapping, &a.Policy, &a.CreatedAt, &a.UpdatedAt, &a.Application)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create 创建账号策略
func (r *AccountStrategyRepository) Create(ctx context.Context, a *repository.AccountStrategy) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO account_strategies (institution_id, application_id, name,
		display_name, is_enabled, mapping, policy) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		a.InstitutionID, a.ApplicationID, a.Name, a.DisplayName, a.IsEnabled,
		orEmptyMap(a.Mapping), orEmptyMap(a.Policy),
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "account strategy not found")
	}
	a.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *AccountStrategyRepository) GetByID(ctx context.Context, id int64) (*repository.AccountStrategy, error) {
	a, err := scanAccountStrategy(r.db.QueryRow(ctx,
		accountStrategySelect+` WHERE t.id=$1`, id))
	if err != nil {
		return nil, mapError(err, "account strategy not found")
	}
	return a, nil
}

// GetByName 按租户 + 名称查询
func (r *AccountStrategyRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.AccountStrategy, error) {
	a, err := scanAccountStrategy(r.db.QueryRow(ctx,
		accountStrategySelect+` WHERE t.institution_id=$1 AND t.name=$2`,
		institutionID, name))
	if err != nil {
		return nil, mapError(err, "account strategy not found")
	}
	return a, nil
}

// Update 更新账号策略
func (r *AccountStrategyRepository) Update(ctx context.Context, a *repository.AccountStrategy) error {
	const q = `UPDATE account_strategies SET display_name=$3, application_id=$4, is_enabled=$5,
		mapping=$6, policy=$7, updated_at=NOW() WHERE institution_id=$1 AND name=$2`
	return mapError(r.db.Exec(ctx, q, a.InstitutionID, a.Name, a.DisplayName, a.ApplicationID,
		a.IsEnabled, orEmptyMap(a.Mapping), orEmptyMap(a.Policy)), "account strategy not found")
}

// Delete 删除账号策略
func (r *AccountStrategyRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM account_strategies WHERE id=$1`, id), "account strategy not found")
}

// List 分页列出账号策略
func (r *AccountStrategyRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.AccountStrategy, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := accountStrategySelect + b.where() + b.orderLimit("t.name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.AccountStrategy
	for rows.Next() {
		a, err := scanAccountStrategy(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, a)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计账号策略数量
func (r *AccountStrategyRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM account_strategies t`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// FindForApplication 返回指定应用启用的策略：应用专属优先，其次租户默认（application_id IS NULL）。
// applicationID 为 0 表示只看租户默认策略。
func (r *AccountStrategyRepository) FindForApplication(ctx context.Context, institutionID int64, applicationID int64) (*repository.AccountStrategy, error) {
	if applicationID != 0 {
		a, err := scanAccountStrategy(r.db.QueryRow(ctx, accountStrategySelect+`
			WHERE t.institution_id=$1 AND t.is_enabled=TRUE AND t.application_id=$2
			ORDER BY t.id ASC LIMIT 1`, institutionID, applicationID))
		if err == nil {
			return a, nil
		}
		if !isNoRows(err) {
			return nil, mapError(err, "")
		}
	}
	a, err := scanAccountStrategy(r.db.QueryRow(ctx, accountStrategySelect+`
		WHERE t.institution_id=$1 AND t.is_enabled=TRUE AND t.application_id IS NULL
		ORDER BY t.id ASC LIMIT 1`, institutionID))
	if err != nil {
		return nil, mapError(err, "account strategy not found")
	}
	return a, nil
}

var _ repository.AccountStrategyRepository = (*AccountStrategyRepository)(nil)
