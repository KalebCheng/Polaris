package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const recordColumns = `id, institution_id, name, client_ip, "user", method, request_uri, action,
	language, object, response, status_code, detail, is_triggered, trace_id, created_at`

// RecordRepository 审计记录仓库（PostgreSQL 实现）
type RecordRepository struct{ db *dbclient.Client }

// NewRecordRepository 创建审计记录仓库
func NewRecordRepository(db *dbclient.Client) *RecordRepository { return &RecordRepository{db: db} }

func scanRecord(row interface{ Scan(...interface{}) error }) (*repository.Record, error) {
	var r repository.Record
	err := row.Scan(&r.ID, &r.InstitutionID, &r.Name, &r.ClientIP, &r.User, &r.Method, &r.RequestURI,
		&r.Action, &r.Language, &r.Object, &r.Response, &r.StatusCode, &r.Detail, &r.IsTriggered,
		&r.TraceID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Create 写入审计记录
func (r *RecordRepository) Create(ctx context.Context, rec *repository.Record) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO records (institution_id, name, client_ip, "user", method,
		request_uri, action, language, object, response, status_code, detail, is_triggered, trace_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		rec.InstitutionID, rec.Name, rec.ClientIP, rec.User, rec.Method, rec.RequestURI, rec.Action,
		rec.Language, rec.Object, rec.Response, rec.StatusCode, rec.Detail, rec.IsTriggered, rec.TraceID,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "")
	}
	rec.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *RecordRepository) GetByID(ctx context.Context, id int64) (*repository.Record, error) {
	rec, err := scanRecord(r.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM records WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "record not found")
	}
	return rec, nil
}

// List 分页列出审计记录
func (r *RecordRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Record, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`("user" ILIKE ` + p + ` OR action ILIKE ` + p + ` OR request_uri ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + recordColumns + ` FROM records` + b.where() + b.orderLimit("created_at DESC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, rec)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计审计记录数量
func (r *RecordRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`("user" ILIKE ` + p + ` OR action ILIKE ` + p + ` OR request_uri ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM records`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// DeleteBefore 清理早于指定时间的记录
func (r *RecordRepository) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.ExecTag(ctx, `DELETE FROM records WHERE created_at < $1`, before)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

var _ repository.RecordRepository = (*RecordRepository)(nil)
