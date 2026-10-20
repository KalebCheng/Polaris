package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const provisionHistoryColumns = `t.id, t.institution_id, t.connector, t.application_id, t.user_id, t.action, t.status,
	t.message, t.created_at`

// provisionHistorySelect：读取时把主键解析回展示名；存储只认主键（§3.8）
const provisionHistorySelect = `SELECT ` + provisionHistoryColumns + `,
	COALESCE((SELECT a.code FROM applications a WHERE a.id = t.application_id), '') AS application,
	COALESCE((SELECT u.name FROM users u WHERE u.id = t.user_id), '') AS "user"
	FROM provision_history t`

// ProvisionHistoryRepository 供给历史仓库（PostgreSQL 实现）
type ProvisionHistoryRepository struct{ db *dbclient.Client }

// NewProvisionHistoryRepository 创建供给历史仓库
func NewProvisionHistoryRepository(db *dbclient.Client) *ProvisionHistoryRepository {
	return &ProvisionHistoryRepository{db: db}
}

func scanProvisionHistory(row interface{ Scan(...interface{}) error }) (*repository.ProvisionHistory, error) {
	var h repository.ProvisionHistory
	err := row.Scan(&h.ID, &h.InstitutionID, &h.Connector, &h.ApplicationID, &h.UserID, &h.Action,
		&h.Status, &h.Message, &h.CreatedAt, &h.Application, &h.User)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// Create 写入供给历史
func (r *ProvisionHistoryRepository) Create(ctx context.Context, h *repository.ProvisionHistory) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO provision_history (institution_id, connector, application_id,
		user_id, action, status, message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		h.InstitutionID, h.Connector, h.ApplicationID, h.UserID, h.Action, h.Status, h.Message,
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
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		// 过滤字段允许 application_id / user_id（主键）；名称过滤走关键词
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.action ILIKE ` + p + ` OR t.connector ILIKE ` + p + `
			OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	sql := provisionHistorySelect + b.where() + b.orderLimit("t.created_at DESC", q.Offset, q.Limit)
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
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	applyQueryFilters(b, q.Filters)
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.action ILIKE ` + p + ` OR t.connector ILIKE ` + p + `
			OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM provision_history t`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.ProvisionHistoryRepository = (*ProvisionHistoryRepository)(nil)
