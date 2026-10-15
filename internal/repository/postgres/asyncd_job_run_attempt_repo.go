package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const asyncdJobRunAttemptColumns = `id, job_run_id, attempt_no, worker_id, status, started_at, finished_at, duration_ms, error`

// AsyncdJobRunAttemptRepository 执行记录仓库（PostgreSQL 实现）
type AsyncdJobRunAttemptRepository struct{ db *dbclient.Client }

// NewAsyncdJobRunAttemptRepository 创建执行记录仓库
func NewAsyncdJobRunAttemptRepository(db *dbclient.Client) *AsyncdJobRunAttemptRepository {
	return &AsyncdJobRunAttemptRepository{db: db}
}

func scanAsyncdJobRunAttempt(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJobRunAttempt, error) {
	var a repository.AsyncdJobRunAttempt
	err := row.Scan(&a.ID, &a.JobRunID, &a.AttemptNo, &a.WorkerID, &a.Status, &a.StartedAt,
		&a.FinishedAt, &a.DurationMs, &a.Error)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Start 记录一次尝试的开始（插一行 running，执行器崩溃时这行会被回收流程标成 lease_lost）
func (r *AsyncdJobRunAttemptRepository) Start(ctx context.Context, attempt *repository.AsyncdJobRunAttempt) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO asyncd_job_run_attempts (job_run_id, attempt_no, worker_id, status)
		VALUES ($1, $2, $3, 'running') RETURNING id`,
		attempt.JobRunID, attempt.AttemptNo, attempt.WorkerID).Scan(&id)
	if err != nil {
		return 0, mapError(err, "")
	}
	attempt.ID = id
	return id, nil
}

// Finish 记录一次尝试的结束。耗时由数据库时间计算（同一时钟源，避免进程间时钟漂移）。
func (r *AsyncdJobRunAttemptRepository) Finish(ctx context.Context, id int64, status, cause string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_job_run_attempts SET status = $2, finished_at = NOW(),
		duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::int),
		error = $3
		WHERE id = $1 AND status = 'running'`, id, status, cause), "尝试记录不存在")
}

// ListByRun 取某次执行的全部尝试（按尝试序号升序 = 时间顺序）
func (r *AsyncdJobRunAttemptRepository) ListByRun(ctx context.Context, jobRunID int64) ([]*repository.AsyncdJobRunAttempt, error) {
	rows, err := r.db.Query(ctx, `SELECT `+asyncdJobRunAttemptColumns+` FROM asyncd_job_run_attempts
		WHERE job_run_id = $1 ORDER BY attempt_no ASC`, jobRunID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdJobRunAttempt
	for rows.Next() {
		attempt, err := scanAsyncdJobRunAttempt(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, attempt)
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.AsyncdJobRunAttemptRepository = (*AsyncdJobRunAttemptRepository)(nil)
