package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// UserImportRepository 导入记录仓库（PostgreSQL 实现；表 user_imports / user_import_rows）
type UserImportRepository struct{ db *dbclient.Client }

// NewUserImportRepository 创建导入记录仓库
func NewUserImportRepository(db *dbclient.Client) *UserImportRepository {
	return &UserImportRepository{db: db}
}

// Create 写入导入记录
func (r *UserImportRepository) Create(ctx context.Context, item *repository.UserImport) error {
	if item.CreatedDepartments == nil {
		item.CreatedDepartments = []string{}
	}
	return mapError(r.db.QueryRow(ctx, `INSERT INTO user_imports
			(institution_id, file_name, file_format, status, total_rows, created_count, skipped_count,
			 failed_count, auto_create_departments, operator)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		item.InstitutionID, item.FileName, item.FileFormat, item.Status, item.TotalRows, item.CreatedCount,
		item.SkippedCount, item.FailedCount, item.AutoCreateDepts, item.Operator,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt), "")
}

// Update 刷新汇总计数与状态
func (r *UserImportRepository) Update(ctx context.Context, item *repository.UserImport) error {
	return mapError(r.db.Exec(ctx, `UPDATE user_imports SET
			status=$3, total_rows=$4, created_count=$5, skipped_count=$6, failed_count=$7, updated_at=NOW()
		WHERE id=$1 AND institution_id=$2`,
		item.ID, item.InstitutionID, item.Status, item.TotalRows, item.CreatedCount,
		item.SkippedCount, item.FailedCount), "import not found")
}

// Get 按主键取（校验组织归属）
func (r *UserImportRepository) Get(ctx context.Context, institutionID, id int64) (*repository.UserImport, error) {
	item := &repository.UserImport{}
	err := r.db.QueryRow(ctx, `SELECT id, institution_id, file_name, file_format, status, total_rows,
			created_count, skipped_count, failed_count, auto_create_departments, operator, created_at, updated_at
		FROM user_imports WHERE id=$1 AND institution_id=$2`, id, institutionID).Scan(
		&item.ID, &item.InstitutionID, &item.FileName, &item.FileFormat, &item.Status, &item.TotalRows,
		&item.CreatedCount, &item.SkippedCount, &item.FailedCount, &item.AutoCreateDepts, &item.Operator,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, mapError(err, "import not found")
	}
	item.CreatedDepartments = []string{}
	return item, nil
}

// List 导入记录分页（最新在前）
func (r *UserImportRepository) List(ctx context.Context, q repository.UserImportListQuery) ([]*repository.UserImport, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(ctx, `SELECT id, institution_id, file_name, file_format, status, total_rows,
			created_count, skipped_count, failed_count, auto_create_departments, operator, created_at, updated_at
		FROM user_imports WHERE institution_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`,
		q.InstitutionID, limit, q.Offset)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := make([]*repository.UserImport, 0, limit)
	for rows.Next() {
		item := &repository.UserImport{CreatedDepartments: []string{}}
		if err := rows.Scan(&item.ID, &item.InstitutionID, &item.FileName, &item.FileFormat, &item.Status,
			&item.TotalRows, &item.CreatedCount, &item.SkippedCount, &item.FailedCount,
			&item.AutoCreateDepts, &item.Operator, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, item)
	}
	return out, mapError(rows.Err(), "")
}

// Count 导入记录总数
func (r *UserImportRepository) Count(ctx context.Context, institutionID int64) (int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM user_imports WHERE institution_id=$1`, institutionID).Scan(&total)
	return total, mapError(err, "")
}

// CreateRows 批量写入逐行结果（续写幂等：(import_id, row_number) 冲突时忽略）
func (r *UserImportRepository) CreateRows(ctx context.Context, rows []*repository.UserImportRow) error {
	if len(rows) == 0 {
		return nil
	}
	importID := rows[0].ImportID
	numbers := make([]int, 0, len(rows))
	names := make([]string, 0, len(rows))
	emails := make([]string, 0, len(rows))
	statuses := make([]string, 0, len(rows))
	createdIDs := make([]*int64, 0, len(rows))
	errorsJSON := make([][]repository.UserImportRowIssue, 0, len(rows))
	warningsJSON := make([][]repository.UserImportRowIssue, 0, len(rows))
	for _, row := range rows {
		numbers = append(numbers, row.RowNumber)
		names = append(names, row.UserName)
		emails = append(emails, row.Email)
		statuses = append(statuses, row.Status)
		createdIDs = append(createdIDs, row.CreatedUserID)
		errorsJSON = append(errorsJSON, orEmptyIssues(row.Errors))
		warningsJSON = append(warningsJSON, orEmptyIssues(row.Warnings))
	}
	return mapError(r.db.Exec(ctx, `INSERT INTO user_import_rows
			(import_id, row_number, user_name, email, status, created_user_id, errors, warnings)
		SELECT $1, n, nm, em, st, cid, ej, wj
		FROM unnest($2::int[], $3::text[], $4::text[], $5::text[], $6::bigint[], $7::jsonb[], $8::jsonb[])
			AS t(n, nm, em, st, cid, ej, wj)
		ON CONFLICT (import_id, row_number) DO NOTHING`,
		importID, numbers, names, emails, statuses, createdIDs, errorsJSON, warningsJSON), "")
}

// ListRows 按行号升序列出逐行结果
func (r *UserImportRepository) ListRows(ctx context.Context, importID int64) ([]*repository.UserImportRow, error) {
	rows, err := r.db.Query(ctx, `SELECT id, import_id, row_number, user_name, email, status,
			created_user_id, errors, warnings, created_at
		FROM user_import_rows WHERE import_id=$1 ORDER BY row_number ASC`, importID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := make([]*repository.UserImportRow, 0, 32)
	for rows.Next() {
		item := &repository.UserImportRow{}
		if err := rows.Scan(&item.ID, &item.ImportID, &item.RowNumber, &item.UserName, &item.Email,
			&item.Status, &item.CreatedUserID, &item.Errors, &item.Warnings, &item.CreatedAt); err != nil {
			return nil, mapError(err, "")
		}
		if item.Errors == nil {
			item.Errors = []repository.UserImportRowIssue{}
		}
		if item.Warnings == nil {
			item.Warnings = []repository.UserImportRowIssue{}
		}
		out = append(out, item)
	}
	return out, mapError(rows.Err(), "")
}

// MarkStaleFailed 把久未更新的 queued/running 记录标记失败
func (r *UserImportRepository) MarkStaleFailed(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE user_imports SET status='failed', updated_at=NOW()
		WHERE status IN ('queued','running') AND updated_at < $1`, before)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

// PurgeBefore 删除过期导入记录（行随外键级联删除）
func (r *UserImportRepository) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.ExecTag(ctx, `DELETE FROM user_imports WHERE created_at < $1`, before)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

func orEmptyIssues(v []repository.UserImportRowIssue) []repository.UserImportRowIssue {
	if v == nil {
		return []repository.UserImportRowIssue{}
	}
	return v
}

var _ repository.UserImportRepository = (*UserImportRepository)(nil)
