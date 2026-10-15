package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// ProviderRepository 提供方仓库（PostgreSQL 实现）。
// 覆盖多类别：OAuth / OIDC / Email / SMS / Captcha / Storage。
type ProviderRepository struct{ db *dbclient.Client }

// NewProviderRepository 创建提供方仓库
func NewProviderRepository(db *dbclient.Client) *ProviderRepository {
	return &ProviderRepository{db: db}
}

const providerTable = "providers"

// Create 创建提供方
func (r *ProviderRepository) Create(ctx context.Context, p *repository.Provider) (int64, error) {
	id, err := insertReturningID(ctx, r.db, providerTable, p)
	if err != nil {
		return 0, err
	}
	p.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *ProviderRepository) GetByID(ctx context.Context, id int64) (*repository.Provider, error) {
	p := &repository.Provider{}
	if err := selectOne(ctx, r.db, providerTable, p, "WHERE id = $1", id); err != nil {
		return nil, err
	}
	return p, nil
}

// GetByName 按组织+名称查询
func (r *ProviderRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Provider, error) {
	p := &repository.Provider{}
	if err := selectOne(ctx, r.db, providerTable, p, "WHERE institution_id = $1 AND name = $2", institutionID, name); err != nil {
		return nil, err
	}
	return p, nil
}

// Update 更新提供方（组织/名称为不可变标识）
func (r *ProviderRepository) Update(ctx context.Context, p *repository.Provider) error {
	return updateEntity(ctx, r.db, providerTable, p, "institution_id", "name")
}

// Delete 删除提供方
func (r *ProviderRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM providers WHERE id = $1`, id), "provider not found")
}

// List 分页列出提供方
func (r *ProviderRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Provider, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + ` = ` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	return queryEntities[repository.Provider](ctx, r.db, providerTable, b.where(), b.args, "category ASC, name ASC", q.Offset, q.Limit)
}

// Count 统计提供方数量
func (r *ProviderRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + ` = ` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	return countEntities(ctx, r.db, providerTable, b.where(), b.args...)
}

var _ repository.ProviderRepository = (*ProviderRepository)(nil)
