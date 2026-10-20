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
func NewJobTitleRepository(db *dbclient.Client) *JobTitleRepository {
	return &JobTitleRepository{db: db}
}

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
	rows, err := r.db.Query(ctx, `SELECT u.id, u.name FROM job_title_members m JOIN users u ON u.id = m.user_id
		WHERE m.job_title_id=$1 ORDER BY u.name, u.id`, t.ID)
	if err != nil {
		return mapError(err, "")
	}
	defer rows.Close()
	members := []string{}
	memberIDs := []int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return mapError(err, "")
		}
		memberIDs = append(memberIDs, id)
		members = append(members, name)
	}
	t.Users = members
	t.UserIDs = memberIDs
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

// SetMemberIDs replaces job-title membership by user primary key.
func (r *JobTitleRepository) SetMemberIDs(ctx context.Context, jobTitleID int64, userIDs []int64) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM job_title_members WHERE job_title_id=$1`, jobTitleID); err != nil {
			return mapError(err, "")
		}
		if len(userIDs) == 0 {
			return nil
		}
		result, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
			SELECT t.id, u.id FROM users u JOIN job_titles t ON t.id=$1
			WHERE u.institution_id=t.institution_id AND u.is_deleted=FALSE AND u.id=ANY($2)
			ON CONFLICT DO NOTHING`, jobTitleID, userIDs)
		if err != nil {
			return mapError(err, "")
		}
		if result.RowsAffected() != int64(len(userIDs)) {
			return bizerrors.ErrInvalidParams.WithDetail("user_ids", userIDs)
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

// JobTitleIDsForUser returns the job-title primary keys assigned to a user.
func (r *JobTitleRepository) JobTitleIDsForUser(ctx context.Context, institutionID, userID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT t.id FROM job_titles t
		JOIN job_title_members tm ON tm.job_title_id=t.id
		WHERE t.institution_id=$1 AND tm.user_id=$2 ORDER BY t.id`, institutionID, userID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, id)
	}
	return out, mapError(rows.Err(), "")
}

// SetJobTitleIDsForUser replaces a user's job-title assignments by primary key.
func (r *JobTitleRepository) SetJobTitleIDsForUser(ctx context.Context, institutionID, userID int64, jobTitleIDs []int64) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM job_title_members
			WHERE user_id=$2
			  AND job_title_id IN (SELECT id FROM job_titles WHERE institution_id=$1)`,
			institutionID, userID); err != nil {
			return mapError(err, "")
		}
		if len(jobTitleIDs) == 0 {
			return nil
		}
		result, err := tx.Exec(ctx, `INSERT INTO job_title_members (job_title_id, user_id)
			SELECT t.id, u.id FROM job_titles t, users u
			WHERE t.institution_id=$1 AND t.id=ANY($3)
			  AND u.institution_id=$1 AND u.id=$2 AND u.is_deleted=FALSE
			ON CONFLICT DO NOTHING`, institutionID, userID, jobTitleIDs)
		if err != nil {
			return mapError(err, "")
		}
		if result.RowsAffected() != int64(len(jobTitleIDs)) {
			return bizerrors.ErrInvalidParams.WithDetail("job_title_ids", jobTitleIDs)
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

// MemberIDsByJobTitleIDs returns distinct member user IDs for the job titles.
func (r *JobTitleRepository) MemberIDsByJobTitleIDs(ctx context.Context, institutionID int64, jobTitleIDs []int64) ([]int64, error) {
	if len(jobTitleIDs) == 0 {
		return []int64{}, nil
	}
	rows, err := r.db.Query(ctx, `SELECT DISTINCT tm.user_id FROM job_title_members tm
		JOIN job_titles t ON t.id=tm.job_title_id
		WHERE t.institution_id=$1 AND t.id=ANY($2) ORDER BY tm.user_id`, institutionID, jobTitleIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, id)
	}
	return out, mapError(rows.Err(), "")
}

// ExistingIDs returns job-title IDs that belong to the institution.
func (r *JobTitleRepository) ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error) {
	return existingIDs(ctx, r.db, "job_titles", institutionID, ids)
}

var _ repository.JobTitleRepository = (*JobTitleRepository)(nil)
