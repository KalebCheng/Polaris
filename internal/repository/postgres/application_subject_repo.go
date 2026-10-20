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

// Replace updates access mode and allowlist in one transaction.
// Existing rows are retained so unchanged subjects keep their original created_at values.
func (r *ApplicationSubjectRepository) Replace(ctx context.Context, app *repository.Application, mode string, subjects []repository.ApplicationSubject) error {
	types := make([]string, 0, len(subjects))
	userIDs := make([]int64, 0, len(subjects))
	jobTitleIDs := make([]int64, 0, len(subjects))
	departmentIDs := make([]int64, 0, len(subjects))
	for _, s := range subjects {
		types = append(types, s.SubjectType)
		userIDs = append(userIDs, subjectIDValue(s.UserID))
		jobTitleIDs = append(jobTitleIDs, subjectIDValue(s.JobTitleID))
		departmentIDs = append(departmentIDs, subjectIDValue(s.DepartmentID))
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE applications SET access_mode = $2, updated_at = NOW() WHERE id = $1`, app.ID, mode); err != nil {
			return mapError(err, "application not found")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM application_subjects s
			WHERE s.application_id = $1
			  AND NOT EXISTS (
			      SELECT 1 FROM unnest($2::text[], $3::bigint[], $4::bigint[], $5::bigint[])
			          AS k(subject_type, user_id, job_title_id, department_id)
			       WHERE k.subject_type = s.subject_type
			         AND k.user_id = COALESCE(s.user_id, 0)
			         AND k.job_title_id = COALESCE(s.job_title_id, 0)
			         AND k.department_id = COALESCE(s.department_id, 0)
			  )`, app.ID, types, userIDs, jobTitleIDs, departmentIDs); err != nil {
			return mapError(err, "")
		}
		for _, s := range subjects {
			if _, err := tx.Exec(ctx, `INSERT INTO application_subjects
				(institution_id, application_id, subject_type, user_id, job_title_id, department_id)
				VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, 0))
				ON CONFLICT DO NOTHING`,
				app.InstitutionID, app.ID, s.SubjectType,
				subjectIDValue(s.UserID), subjectIDValue(s.JobTitleID), subjectIDValue(s.DepartmentID)); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

func subjectIDValue(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// List returns allowlist entries and joins display-only subject metadata.
func (r *ApplicationSubjectRepository) List(ctx context.Context, applicationID int64) ([]repository.ApplicationSubject, error) {
	rows, err := r.db.Query(ctx, `SELECT s.id, s.institution_id, s.application_id, s.subject_type,
			COALESCE(s.user_id, 0), COALESCE(s.job_title_id, 0), COALESCE(s.department_id, 0),
			COALESCE(u.name, jt.name, d.name, ''),
			COALESCE(NULLIF(u.display_name, ''), u.name,
			         NULLIF(jt.display_name, ''), jt.name,
			         NULLIF(d.display_name, ''), d.name, ''),
			CASE WHEN d.id IS NOT NULL THEN d.name_path ELSE COALESCE(jt.name, '') END,
			COALESCE(u.email, ''),
			s.created_at
		FROM application_subjects s
		LEFT JOIN users u ON u.id=s.user_id
		LEFT JOIN job_titles jt ON jt.id=s.job_title_id
		LEFT JOIN (
			WITH RECURSIVE paths AS (
				SELECT id, parent_id, name, display_name, name::TEXT AS name_path
				FROM departments WHERE parent_id IS NULL
				UNION ALL
				SELECT child.id, child.parent_id, child.name, child.display_name,
				       paths.name_path || '/' || child.name
				FROM departments child JOIN paths ON child.parent_id=paths.id
			)
			SELECT id, name, display_name, name_path FROM paths
		) d ON d.id=s.department_id
		WHERE s.application_id=$1
		ORDER BY s.subject_type, 8, s.id`, applicationID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []repository.ApplicationSubject{}
	for rows.Next() {
		var item repository.ApplicationSubject
		var userID, jobTitleID, departmentID int64
		if err := rows.Scan(&item.ID, &item.InstitutionID, &item.ApplicationID, &item.SubjectType,
			&userID, &jobTitleID, &departmentID, &item.SubjectName, &item.SubjectDisplayName,
			&item.SubjectPath, &item.SubjectEmail, &item.CreatedAt); err != nil {
			return nil, mapError(err, "")
		}
		if userID > 0 {
			item.UserID = &userID
		}
		if jobTitleID > 0 {
			item.JobTitleID = &jobTitleID
		}
		if departmentID > 0 {
			item.DepartmentID = &departmentID
		}
		out = append(out, item)
	}
	return out, mapError(rows.Err(), "")
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
