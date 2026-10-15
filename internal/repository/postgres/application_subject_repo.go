package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// ApplicationSubjectRepository 应用授权主体仓库（PostgreSQL 实现）。
//
// 只负责 application_subjects 一张表；"主体画像 / 影响面展开"这类跨表查询分别落在
// 用户组与部门仓库上，由服务层组合（保持每个仓库只自治自己那张表）。
type ApplicationSubjectRepository struct{ db *dbclient.Client }

// NewApplicationSubjectRepository 创建应用授权主体仓库
func NewApplicationSubjectRepository(db *dbclient.Client) *ApplicationSubjectRepository {
	return &ApplicationSubjectRepository{db: db}
}

const applicationSubjectTable = "application_subjects"

// Replace 整体替换某应用的**访问授权配置**（单事务：applications.access_mode + application_subjects）。
//
// 为什么两个表在同一个方法里：应用的"访问模式 + 授权名单"是一次提交的一个配置单元，
// 分开写就会出现"模式已改成 restricted、名单还没落库"的中间态（§3.1-6 禁止非事务多表写入）。
//
// 语义：未出现在新集合中的旧条目被删除，新集合中的条目被插入；
// 仍在集合中的条目**保留原 created_at**（避免"改一次名单整表时间戳全变"）。
// 空集合是合法输入（= 该应用暂时无人可访问，见 Spec Q4）。
func (r *ApplicationSubjectRepository) Replace(ctx context.Context, app *repository.Application, mode string, subjects []repository.ApplicationSubject) error {
	types := make([]string, 0, len(subjects))
	names := make([]string, 0, len(subjects))
	for _, s := range subjects {
		types = append(types, s.SubjectType)
		names = append(names, s.SubjectName)
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE applications SET access_mode = $2, updated_at = NOW() WHERE id = $1`, app.ID, mode); err != nil {
			return mapError(err, "application not found")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM application_subjects s
			WHERE s.application_id = $1
			  AND NOT EXISTS (
			      SELECT 1 FROM unnest($2::text[], $3::text[]) AS k(subject_type, subject_name)
			       WHERE k.subject_type = s.subject_type AND k.subject_name = s.subject_name
			  )`, app.ID, types, names); err != nil {
			return mapError(err, "")
		}
		for _, s := range subjects {
			if _, err := tx.Exec(ctx, `INSERT INTO application_subjects
				(institution_id, application_id, subject_type, subject_name)
				VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
				app.InstitutionID, app.ID, s.SubjectType, s.SubjectName); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// List 列出某应用的授权主体（按类型 + 标识名排序）。
func (r *ApplicationSubjectRepository) List(ctx context.Context, applicationID int64) ([]repository.ApplicationSubject, error) {
	items, err := queryEntities[repository.ApplicationSubject](ctx, r.db, applicationSubjectTable,
		"WHERE application_id = $1", []interface{}{applicationID}, "subject_type ASC, subject_name ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]repository.ApplicationSubject, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out, nil
}

// CountByApplications 批量统计各应用的授权主体数（应用列表「访问范围」列；一次查询，不做 N+1）。
func (r *ApplicationSubjectRepository) CountByApplications(ctx context.Context, applicationIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(applicationIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT application_id, COUNT(*) FROM application_subjects
		WHERE application_id = ANY($1) GROUP BY application_id`, applicationIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = n
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.ApplicationSubjectRepository = (*ApplicationSubjectRepository)(nil)
