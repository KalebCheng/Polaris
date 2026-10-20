package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// JobLevelRepository 职级仓库（PostgreSQL 实现；表 job_levels）
type JobLevelRepository struct{ db *dbclient.Client }

// NewJobLevelRepository 创建职级仓库
func NewJobLevelRepository(db *dbclient.Client) *JobLevelRepository {
	return &JobLevelRepository{db: db}
}

// Create 创建职级
func (r *JobLevelRepository) Create(ctx context.Context, l *repository.JobLevel) (int64, error) {
	id, err := insertReturningID(ctx, r.db, "job_levels", l)
	if err != nil {
		return 0, err
	}
	l.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *JobLevelRepository) GetByID(ctx context.Context, id int64) (*repository.JobLevel, error) {
	l := &repository.JobLevel{}
	if err := selectOne(ctx, r.db, "job_levels", l, "WHERE id=$1", id); err != nil {
		return nil, err
	}
	return l, nil
}

// GetByName 按组织+名称查询
func (r *JobLevelRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.JobLevel, error) {
	l := &repository.JobLevel{}
	if err := selectOne(ctx, r.db, "job_levels", l, "WHERE institution_id=$1 AND name=$2", institutionID, name); err != nil {
		return nil, err
	}
	return l, nil
}

// Update 更新职级
func (r *JobLevelRepository) Update(ctx context.Context, l *repository.JobLevel) error {
	return updateEntity(ctx, r.db, "job_levels", l)
}

// Delete 删除职级（引用它的用户由 ON DELETE SET NULL 自动置空）
func (r *JobLevelRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM job_levels WHERE id=$1`, id), "job level not found")
}

// List 按 sort_order 升序列出职级
func (r *JobLevelRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.JobLevel, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR description ILIKE ` + p + `)`)
	}
	return queryEntities[repository.JobLevel](ctx, r.db, "job_levels", b.where(), b.args, "sort_order ASC, name ASC", q.Offset, q.Limit)
}

// Count 统计职级数量
func (r *JobLevelRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR description ILIKE ` + p + `)`)
	}
	return countEntities(ctx, r.db, "job_levels", b.where(), b.args...)
}

// CountByLevel 返回组织内各职级的用户数（组织概览的职级分布；零人职级也要出现）
func (r *JobLevelRepository) CountByLevel(ctx context.Context, institutionID int64) (map[int64]int, error) {
	rows, err := r.db.Query(ctx, `SELECT l.id, COUNT(u.id) FROM job_levels l
		LEFT JOIN users u ON u.job_level_id = l.id AND u.is_deleted = FALSE
		WHERE l.institution_id = $1 GROUP BY l.id`, institutionID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = count
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.JobLevelRepository = (*JobLevelRepository)(nil)
