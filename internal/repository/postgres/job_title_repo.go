package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// JobTitleRepository 职务仓库（PostgreSQL 实现；表 job_titles / job_title_members）
type JobTitleRepository struct{ db *dbclient.Client }

// NewJobTitleRepository 创建职务仓库
func NewJobTitleRepository(db *dbclient.Client) *JobTitleRepository { return &JobTitleRepository{db: db} }

// Create 创建职务
func (r *JobTitleRepository) Create(ctx context.Context, t *repository.JobTitle) (int64, error) {
	id, err := insertReturningID(ctx, r.db, "job_titles", t)
	if err != nil {
		return 0, err
	}
	t.ID = id
	return id, nil
}

// GetByID 按 ID 查询（装配任职人员）
func (r *JobTitleRepository) GetByID(ctx context.Context, id int64) (*repository.JobTitle, error) {
	t := &repository.JobTitle{}
	if err := selectOne(ctx, r.db, "job_titles", t, "WHERE id=$1", id); err != nil {
		return nil, err
	}
	if err := r.loadMembers(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetByName 按组织+标识查询
func (r *JobTitleRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.JobTitle, error) {
	t := &repository.JobTitle{}
	if err := selectOne(ctx, r.db, "job_titles", t, "WHERE institution_id=$1 AND name=$2", institutionID, name); err != nil {
		return nil, err
	}
	if err := r.loadMembers(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// loadMembers 装配任职人员（职务扁平：直接成员，无祖先展开）
func (r *JobTitleRepository) loadMembers(ctx context.Context, t *repository.JobTitle) error {
	rows, err := r.db.Query(ctx, `SELECT u.name FROM job_title_members m JOIN users u ON u.id = m.user_id
		WHERE m.job_title_id=$1 ORDER BY u.name`, t.ID)
	if err != nil {
		return mapError(err, "")
	}
	defer rows.Close()
	members := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return mapError(err, "")
		}
		members = append(members, u)
	}
	t.Users = members
	return mapError(rows.Err(), "")
}

// Update 更新职务（标识不可改；空值保护由服务层负责）
func (r *JobTitleRepository) Update(ctx context.Context, t *repository.JobTitle) error {
	return updateEntity(ctx, r.db, "job_titles", t)
}

// Delete 删除职务（任职人员与角色/权限关联由外键级联清理）
func (r *JobTitleRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM job_titles WHERE id=$1`, id), "job title not found")
}

// List 分页列出职务
func (r *JobTitleRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.JobTitle, error) {
	where, args := jobTitleWhere(q)
	return queryEntities[repository.JobTitle](ctx, r.db, "job_titles", where, args, "name ASC", q.Offset, q.Limit)
}

// Count 统计职务数量
func (r *JobTitleRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	where, args := jobTitleWhere(q)
	return countEntities(ctx, r.db, "job_titles", where, args...)
}

// jobTitleWhere 组装职务列表的过滤条件（组织 + 关键词）
func jobTitleWhere(q repository.ListQuery) (string, []interface{}) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	return b.where(), b.args
}

// SetMembers 整体替换职务任职人员（事务；用户名必须在组织内存在）
func (r *JobTitleRepository) SetMembers(ctx context.Context, jobTitleID int64, users []string) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM job_title_members WHERE job_title_id=$1`, jobTitleID); err != nil {
			return mapError(err, "")
		}
		for _, u := range users {
			if u == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
				SELECT $1, u.id FROM users u JOIN job_titles t ON t.id=$1
				WHERE u.institution_id = t.institution_id AND u.name = $2
				ON CONFLICT DO NOTHING`, jobTitleID, u); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// AddMembers 追加任职人员（幂等；只增不删）
func (r *JobTitleRepository) AddMembers(ctx context.Context, jobTitleID int64, users []string) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		for _, name := range users {
			if name == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
				SELECT $1, u.id FROM users u JOIN job_titles t ON t.id=$1
				WHERE u.institution_id = t.institution_id AND u.name = $2
				ON CONFLICT DO NOTHING`, jobTitleID, name); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// JobTitlesForUser 返回用户任职的职务标识
func (r *JobTitleRepository) JobTitlesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT t.name FROM job_titles t
		JOIN job_title_members tm ON tm.job_title_id=t.id
		JOIN users u ON u.id = tm.user_id
		WHERE t.institution_id=$1 AND u.name=$2 ORDER BY t.name`, institutionID, user)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, s)
	}
	return out, mapError(rows.Err(), "")
}

// SetJobTitlesForUser 整体替换某用户的职务任职关系（事务；职务标识必须在组织内存在）
func (r *JobTitleRepository) SetJobTitlesForUser(ctx context.Context, institutionID int64, user string, titles []string) error {
	if user == "" {
		return bizerrors.ErrInvalidParams.WithDetail("required", "user")
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM job_title_members WHERE user_id =
			(SELECT id FROM users WHERE institution_id = $1 AND name = $2)
			AND job_title_id IN (SELECT id FROM job_titles WHERE institution_id = $1)`, institutionID, user); err != nil {
			return mapError(err, "")
		}
		for _, name := range titles {
			if name == "" {
				continue
			}
			res, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
				SELECT t.id, u.id FROM job_titles t, users u
				WHERE t.institution_id = $1 AND t.name = $3 AND u.institution_id = $1 AND u.name = $2`, institutionID, user, name)
			if err != nil {
				return mapError(err, "")
			}
			if res.RowsAffected() == 0 {
				return bizerrors.ErrInvalidParams.WithDetail("job_title", name)
			}
		}
		return nil
	})
}

// MemberCounts 返回组织内各职务的任职人数（列表页展示，避免 N 次查询）
func (r *JobTitleRepository) MemberCounts(ctx context.Context, institutionID int64) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT t.name, COUNT(tm.user_id)
		FROM job_titles t
		LEFT JOIN job_title_members tm ON tm.job_title_id = t.id
		WHERE t.institution_id = $1
		GROUP BY t.name`, institutionID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, mapError(err, "")
		}
		out[name] = count
	}
	return out, mapError(rows.Err(), "")
}

// ExpandMembers 展开"这些职务"的任职人员（职务扁平：不展开任何层级）。
// 用途：授权名单变更时计算受影响用户（撤销授权需失效这些人的会话）。
func (r *JobTitleRepository) ExpandMembers(ctx context.Context, institutionID int64, names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{}, nil
	}
	rows, err := r.db.Query(ctx, `SELECT DISTINCT u.name FROM job_title_members tm
		JOIN job_titles t ON t.id = tm.job_title_id
		JOIN users u ON u.id = tm.user_id
		WHERE t.institution_id = $1 AND t.name = ANY($2) ORDER BY 1`, institutionID, names)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, name)
	}
	return out, mapError(rows.Err(), "")
}

// ExistingNames 返回 names 中确实存在于本组织的职务标识集合（授权名单逐项校验用）
func (r *JobTitleRepository) ExistingNames(ctx context.Context, institutionID int64, names []string) (map[string]bool, error) {
	return existingNames(ctx, r.db, "job_titles", institutionID, names)
}

var _ repository.JobTitleRepository = (*JobTitleRepository)(nil)
