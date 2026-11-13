package repository

import "time"

// 导入记录状态（user_imports.status）
const (
	UserImportQueued    = "queued"    // 已受理，等待执行（含从请求中脱离开的续跑）
	UserImportRunning   = "running"   // 正在执行，计数随批次提交更新
	UserImportSucceeded = "succeeded" // 全部行成功
	UserImportPartial   = "partial"   // 部分成功（含跳过或失败）
	UserImportFailed    = "failed"    // 没有任何一行成功 / 执行中断
)

// 逐行状态（user_import_rows.status）
const (
	UserImportRowCreated = "created"
	UserImportRowSkipped = "skipped"
	UserImportRowFailed  = "failed"
)

// UserImport 一次导入的汇总（不含原始文件与密码）。
type UserImport struct {
	ID              int64  `db:"id" json:"id"`
	InstitutionID   int64  `db:"institution_id" json:"institution_id"`
	FileName        string `db:"file_name" json:"file_name"`
	FileFormat      string `db:"file_format" json:"file_format"`
	Status          string `db:"status" json:"status"`
	TotalRows       int    `db:"total_rows" json:"total_rows"`
	CreatedCount    int    `db:"created_count" json:"created_count"`
	SkippedCount    int    `db:"skipped_count" json:"skipped_count"`
	FailedCount     int    `db:"failed_count" json:"failed_count"`
	AutoCreateDepts bool   `db:"auto_create_departments" json:"auto_create_departments"`
	Operator        string `db:"operator" json:"operator"`
	// CreatedDepartments 本次自动创建的部门路径（仅存在于记录快照与响应，不落列）
	CreatedDepartments []string `db:"-" json:"created_departments"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// UserImportRowIssue 逐行问题（行列名 + 英文说明；前端按列名走 i18n）。
type UserImportRowIssue struct {
	Column  string `json:"column"`
	Message string `json:"message"`
}

// UserImportRow 一次导入中的一行。
type UserImportRow struct {
	ID       int64 `db:"id" json:"id"`
	ImportID int64 `db:"import_id" json:"import_id"`
	// RowNumber 文件行号（表头为第 1 行）
	RowNumber     int                  `db:"row_number" json:"row_number"`
	UserName      string               `db:"user_name" json:"user_name"`
	Email         string               `db:"email" json:"email"`
	Status        string               `db:"status" json:"status"`
	CreatedUserID *int64               `db:"created_user_id" json:"created_user_id,omitempty"`
	Errors        []UserImportRowIssue `db:"errors" json:"errors"` // JSONB
	Warnings      []UserImportRowIssue `db:"warnings" json:"warnings"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// UserImportListQuery 导入记录列表查询条件
type UserImportListQuery struct {
	InstitutionID int64
	Offset        int
	Limit         int
}
