package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const provisionHistoryColumns = `id, institution_id, connector, application, "user", action, status,
	message, created_at`

// ProvisionHistoryRepository 供给历史仓库（PostgreSQL 实现）
type ProvisionHistoryRepository struct{ db *dbclient.Client }

// NewProvisionHistoryRepository 创建供给历史仓库
func NewProvisionHistoryRepository(db *dbclient.Client) *ProvisionHistoryRepository {
	return &ProvisionHistoryRepository{db: db}
}

func scanProvisionHistory(row interface{ Scan(...interface{}) error }) (*repository.ProvisionHistory, error) {
	var h repository.ProvisionHistory
	err := row.Scan(&h.ID, &h.InstitutionID, &h.Connector, &h.Application, &h.User, &h.Action,
		&h.Status, &h.Message, &h.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// Create 写入供给历史
func (r *ProvisionHistoryRepository) Create(ctx context.Context, h *repository.ProvisionHistory) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO provision_history (institution_id, connector, application,
		"user", action, status, message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		h.InstitutionID, h.Connector, h.Application, h.User, h.Action, h.Status, h.Message,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "")
	}
	h.ID = id
	return id, nil
}

// List 分页列出供给历史
func (r *ProvisionHistoryRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.ProvisionHistory, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`("user" ILIKE ` + p + ` OR action ILIKE ` + p + ` OR connector ILIKE ` + p + ` OR application ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + provisionHistoryColumns + ` FROM provision_history` + b.where() + b.orderLimit("created_at DESC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.ProvisionHistory
	for rows.Next() {
		h, err := scanProvisionHistory(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, h)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计供给历史数量
func (r *ProvisionHistoryRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`("user" ILIKE ` + p + ` OR action ILIKE ` + p + ` OR connector ILIKE ` + p + ` OR application ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM provision_history`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.ProvisionHistoryRepository = (*ProvisionHistoryRepository)(nil)
