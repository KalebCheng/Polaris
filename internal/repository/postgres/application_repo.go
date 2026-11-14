package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// ApplicationRepository 应用仓库（PostgreSQL 实现）。
// 列清单由 mapper.go 依据 db tag 自动推导，避免新增字段时漏列。
type ApplicationRepository struct{ db *dbclient.Client }

// NewApplicationRepository 创建应用仓库
func NewApplicationRepository(db *dbclient.Client) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

const applicationTable = "applications"

// normalizeApplication 兜底：adapter 为空时归一化为 "default"，避免写入违反
// applications_adapter_check 约束（反射映射器总会写出该列，DB 默认值不生效）。
func normalizeApplication(a *repository.Application) {
	if a.Adapter != "custom" {
		a.Adapter = "default"
		a.AdapterConfig = nil
	}
}

// Create 创建应用
func (r *ApplicationRepository) Create(ctx context.Context, a *repository.Application) (int64, error) {
	return r.CreateWithSigninMethods(ctx, a, nil)
}

// CreateWithSigninMethods 单事务创建应用并写入登录方式选择。
func (r *ApplicationRepository) CreateWithSigninMethods(ctx context.Context, a *repository.Application, methodIDs []int64) (int64, error) {
	normalizeApplication(a)
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		next, err := insertReturningID(ctx, tx, applicationTable, a)
		if err != nil {
			return err
		}
		id = next
		a.ID = next
		if len(methodIDs) == 0 {
			return nil
		}
		return replaceApplicationSelectionTx(ctx, tx, a.InstitutionID, next, methodIDs)
	})
	return id, err
}

// GetByID 按 ID 查询
func (r *ApplicationRepository) GetByID(ctx context.Context, id int64) (*repository.Application, error) {
	a := &repository.Application{}
	if err := selectOne(ctx, r.db, applicationTable, a, "WHERE id = $1", id); err != nil {
		return nil, err
	}
	return a, nil
}

// GetByCode 按组织+标识查询
func (r *ApplicationRepository) GetByCode(ctx context.Context, institutionID int64, code string) (*repository.Application, error) {
	a := &repository.Application{}
	if err := selectOne(ctx, r.db, applicationTable, a, "WHERE institution_id = $1 AND code = $2", institutionID, code); err != nil {
		return nil, err
	}
	return a, nil
}

// GetByClientID 按 client_id 查询
func (r *ApplicationRepository) GetByClientID(ctx context.Context, clientID string) (*repository.Application, error) {
	a := &repository.Application{}
	if err := selectOne(ctx, r.db, applicationTable, a, "WHERE client_id = $1", clientID); err != nil {
		return nil, err
	}
	return a, nil
}

// Update 更新应用（组织/名称/client_id 为不可变标识）
func (r *ApplicationRepository) Update(ctx context.Context, a *repository.Application) error {
	return r.UpdateWithSigninMethods(ctx, a, nil)
}

// UpdateWithSigninMethods 单事务更新应用；methodIDs 为 nil 时保留既有选择。
func (r *ApplicationRepository) UpdateWithSigninMethods(ctx context.Context, a *repository.Application, methodIDs *[]int64) error {
	normalizeApplication(a)
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := updateEntityTx(ctx, tx, applicationTable, a, "institution_id", "code", "client_id"); err != nil {
			return err
		}
		if methodIDs == nil {
			return nil
		}
		return replaceApplicationSelectionTx(ctx, tx, a.InstitutionID, a.ID, *methodIDs)
	})
}

// Delete 删除应用
func (r *ApplicationRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM applications WHERE id = $1`, id), "application not found")
}

// List 分页列出应用
func (r *ApplicationRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Application, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	return queryEntities[repository.Application](ctx, r.db, applicationTable, b.where(), b.args, "sort_order ASC, name ASC", q.Offset, q.Limit)
}

// Count 统计应用数量
func (r *ApplicationRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	return countEntities(ctx, r.db, applicationTable, b.where(), b.args...)
}

var _ repository.ApplicationRepository = (*ApplicationRepository)(nil)
