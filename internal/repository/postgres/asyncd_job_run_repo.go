package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const asyncdRunColumns = `id, job_id, status, payload, payload_version, priority, attempts, max_attempts,
	next_attempt_at, deadline, unique_key, worker_id, lease_expires_at, last_error, institution_id,
	created_at, updated_at, finished_at`

// AsyncdJobRunRepository 执行实例仓库（PostgreSQL 实现）
type AsyncdJobRunRepository struct{ db *dbclient.Client }

// NewAsyncdJobRunRepository 创建执行实例仓库
func NewAsyncdJobRunRepository(db *dbclient.Client) *AsyncdJobRunRepository {
	return &AsyncdJobRunRepository{db: db}
}

func scanAsyncdJobRun(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJobRun, error) {
	var r repository.AsyncdJobRun
	err := row.Scan(&r.ID, &r.JobID, &r.Status, &r.Payload, &r.PayloadVersion, &r.Priority,
		&r.Attempts, &r.MaxAttempts, &r.NextAttemptAt, &r.Deadline, &r.UniqueKey, &r.WorkerID,
		&r.LeaseExpiresAt, &r.LastError, &r.InstitutionID, &r.CreatedAt, &r.UpdatedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// scanClaimedRun 领取结果多一列 task_type（join 带出）
func scanClaimedRun(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJobRun, error) {
	var r repository.AsyncdJobRun
	err := row.Scan(&r.ID, &r.JobID, &r.Status, &r.Payload, &r.PayloadVersion, &r.Priority,
		&r.Attempts, &r.MaxAttempts, &r.NextAttemptAt, &r.Deadline, &r.UniqueKey, &r.WorkerID,
		&r.LeaseExpiresAt, &r.LastError, &r.InstitutionID, &r.CreatedAt, &r.UpdatedAt, &r.FinishedAt,
		&r.TaskType)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Enqueue 入队。
// 单飞：unique_key 非空时，若已有 queued/running 的同键实例，**不新建**而是返回既有实例 id（existing=true）。
// 调用方通常处在业务事务里（outbox），因此这里不自己开事务。
func (r *AsyncdJobRunRepository) Enqueue(ctx context.Context, run *repository.AsyncdJobRun) (int64, bool, error) {
	if run.UniqueKey != "" {
		var existingID int64
		err := r.db.QueryRow(ctx, `SELECT id FROM asyncd_job_runs
			WHERE unique_key = $1 AND status IN ('queued','running') LIMIT 1`, run.UniqueKey).Scan(&existingID)
		if err == nil {
			return existingID, true, nil
		}
		if !isNoRows(err) {
			return 0, false, mapError(err, "")
		}
	}
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO asyncd_job_runs
		(job_id, payload, payload_version, status, priority, max_attempts, next_attempt_at, deadline, unique_key, institution_id)
		VALUES ($1, COALESCE($2,'{}'::jsonb), $3, 'queued', $4, $5, COALESCE($6, NOW()), $7, $8, $9)
		RETURNING id`,
		run.JobID, nullableJSONB(run.Payload), run.PayloadVersion, run.Priority, run.MaxAttempts,
		run.NextAttemptAt, run.Deadline, run.UniqueKey, run.InstitutionID,
	).Scan(&id)
	if err != nil {
		// 并发入队撞上单飞唯一索引：视为已存在（另一个请求刚入队）
		if isUniqueViolation(err) && run.UniqueKey != "" {
			var existingID int64
			if e := r.db.QueryRow(ctx, `SELECT id FROM asyncd_job_runs
				WHERE unique_key = $1 AND status IN ('queued','running') LIMIT 1`, run.UniqueKey).Scan(&existingID); e == nil {
				return existingID, true, nil
			}
		}
		return 0, false, mapError(err, "")
	}
	run.ID = id
	return id, false, nil
}

// Claim 领取到期实例：SKIP LOCKED 保证多实例并发下互不阻塞且不重复领取。
func (r *AsyncdJobRunRepository) Claim(ctx context.Context, workerID string, leaseTTL time.Duration, limit int) ([]*repository.AsyncdJobRun, error) {
	if limit <= 0 {
		limit = 10
	}
	// 领取与"取处理器类型"一条语句完成：数据修改 CTE 先把实例置为 running，
	// 外层再 join 任务定义带出 task_type（执行器据此找处理器，无需额外查询或缓存）。
	rows, err := r.db.Query(ctx, `WITH claimed AS (
			UPDATE asyncd_job_runs SET
				status = 'running', worker_id = $1, lease_expires_at = NOW() + $2::interval,
				attempts = attempts + 1, updated_at = NOW()
			WHERE id IN (
				SELECT id FROM asyncd_job_runs
				WHERE status = 'queued' AND next_attempt_at <= NOW()
				  AND (deadline IS NULL OR deadline > NOW())
				ORDER BY priority DESC, next_attempt_at ASC
				LIMIT $3
				FOR UPDATE SKIP LOCKED
			)
			RETURNING `+asyncdRunColumns+`
		)
		SELECT c.id, c.job_id, c.status, c.payload, c.payload_version, c.priority, c.attempts,
			c.max_attempts, c.next_attempt_at, c.deadline, c.unique_key, c.worker_id,
			c.lease_expires_at, c.last_error, c.institution_id, c.created_at, c.updated_at,
			c.finished_at, j.task_type
		FROM claimed c JOIN asyncd_jobs j ON j.id = c.job_id`, workerID, leaseInterval(leaseTTL), limit)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var runs []*repository.AsyncdJobRun
	for rows.Next() {
		run, err := scanClaimedRun(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		runs = append(runs, run)
	}
	return runs, mapError(rows.Err(), "")
}

// RenewLease 续租；返回 false 表示租约已被其他实例回收（调用方应停止执行并把结果视为丢失）。
//
// 刻意**不动 updated_at**：它表达"行被状态机改动过"，被心跳刷新后
// "最近活动时间 / 耗时"这类判断就全错了（第一阶段的实际问题）。
func (r *AsyncdJobRunRepository) RenewLease(ctx context.Context, id int64, workerID string, leaseTTL time.Duration) (bool, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_job_runs SET lease_expires_at = NOW() + $3::interval
		WHERE id = $1 AND worker_id = $2 AND status = 'running'`, id, workerID, leaseInterval(leaseTTL))
	if err != nil {
		return false, mapError(err, "")
	}
	return tag.RowsAffected() > 0, nil
}

// Complete 标记成功
func (r *AsyncdJobRunRepository) Complete(ctx context.Context, id int64, workerID string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_job_runs SET status = 'succeeded', worker_id = '',
		lease_expires_at = NULL, finished_at = NOW(), updated_at = NOW(), last_error = ''
		WHERE id = $1 AND worker_id = $2`, id, workerID), "")
}

// Retry 记录失败：未达上限则退避重试（回到 queued），达到上限则入死信。
func (r *AsyncdJobRunRepository) Retry(ctx context.Context, id int64, workerID, cause string, nextAttemptAt time.Time) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_job_runs SET
			status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'queued' END,
			worker_id = '', lease_expires_at = NULL,
			next_attempt_at = COALESCE($3, next_attempt_at),
			last_error = $4, updated_at = NOW(),
			finished_at = CASE WHEN attempts >= max_attempts THEN NOW() ELSE NULL END
		WHERE id = $1 AND worker_id = $2`, id, workerID, nextAttemptAt, cause), "")
}

// Kill 直接入死信（超期、不可重试的错误）
func (r *AsyncdJobRunRepository) Kill(ctx context.Context, id int64, workerID, cause string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_job_runs SET status = 'dead', worker_id = '',
		lease_expires_at = NULL, last_error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND worker_id = $2`, id, workerID, cause), "")
}

// ReclaimExpired 回收租约过期仍处于 running 的实例（执行器崩溃恢复），并让"执行者失联"在执行记录里留痕。
//
// 三步必须同一事务：①锁住过期实例 ②放回 queued（超限入死信）③把对应的"进行中"尝试标为 lease_lost。
func (r *AsyncdJobRunRepository) ReclaimExpired(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var reclaimed []int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM asyncd_job_runs
			WHERE status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW()
			ORDER BY lease_expires_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return scanErr
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		// 计数已在领取时 +1，这里只把状态放回 queued；超过上限则入死信
		if _, err := tx.Exec(ctx, `UPDATE asyncd_job_runs SET
				status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'queued' END,
				worker_id = '', lease_expires_at = NULL,
				last_error = CASE WHEN attempts >= max_attempts
					THEN 'worker lease expired and attempts exhausted' ELSE last_error END,
				finished_at = CASE WHEN attempts >= max_attempts THEN NOW() ELSE NULL END,
				updated_at = NOW()
			WHERE id = ANY($1)`, ids); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE asyncd_job_run_attempts SET status = 'lease_lost',
				finished_at = NOW(),
				duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::int),
				error = 'worker lease expired'
			WHERE job_run_id = ANY($1) AND status = 'running'`, ids)
		if err != nil {
			return err
		}
		reclaimed = ids
		return nil
	})
	if err != nil {
		return nil, mapError(err, "")
	}
	return reclaimed, nil
}

// Counts 按状态计数（可同时按归属过滤）
func (r *AsyncdJobRunRepository) Counts(ctx context.Context, query repository.AsyncdJobRunQuery) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT status, COUNT(*) FROM asyncd_job_runs
		WHERE ($1 = 0 OR job_id = $1) AND ($2 = '' OR status = $2) GROUP BY status`,
		query.JobID, query.Status)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, mapError(err, "")
		}
		out[status] = count
	}
	return out, mapError(rows.Err(), "")
}

// List 列出执行实例（执行记录列表）
func (r *AsyncdJobRunRepository) List(ctx context.Context, query repository.AsyncdJobRunQuery, offset, limit int) ([]*repository.AsyncdJobRun, error) {
	b := &queryBuilder{}
	if query.JobID != 0 {
		b.add(`job_id = ` + b.arg(query.JobID))
	}
	if query.Status != "" {
		b.add(`status = ` + b.arg(query.Status))
	}
	sql := `SELECT ` + asyncdRunColumns + ` FROM asyncd_job_runs` + b.where() + b.orderLimit("id DESC", offset, limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdJobRun
	for rows.Next() {
		run, err := scanAsyncdJobRun(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, run)
	}
	return out, mapError(rows.Err(), "")
}

// GetByID 按主键取
func (r *AsyncdJobRunRepository) GetByID(ctx context.Context, id int64) (*repository.AsyncdJobRun, error) {
	row := r.db.QueryRow(ctx, `SELECT `+asyncdRunColumns+` FROM asyncd_job_runs WHERE id = $1`, id)
	run, err := scanAsyncdJobRun(row)
	if err != nil {
		return nil, mapError(err, "执行实例不存在")
	}
	return run, nil
}

// Stats 按任务 × 状态聚合执行计数（任务目录用）。
//
// 一次性把分组行读回来、由服务层合并，避免按任务逐个查（N+1）。
func (r *AsyncdJobRunRepository) Stats(ctx context.Context) ([]repository.AsyncdJobRunStat, error) {
	rows, err := r.db.Query(ctx, `SELECT job_id, status, COUNT(*), MAX(created_at)
		FROM asyncd_job_runs GROUP BY job_id, status`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []repository.AsyncdJobRunStat
	for rows.Next() {
		var stat repository.AsyncdJobRunStat
		if err := rows.Scan(&stat.JobID, &stat.Status, &stat.Count, &stat.LastQueuedAt); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, stat)
	}
	return out, mapError(rows.Err(), "")
}

// RetryDead 人工重试死信实例：回到 queued 并清零尝试计数（保留历史 last_error 便于排查）
func (r *AsyncdJobRunRepository) RetryDead(ctx context.Context, id int64) error {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_job_runs SET status = 'queued', attempts = 0,
		next_attempt_at = NOW(), worker_id = '', lease_expires_at = NULL, finished_at = NULL,
		updated_at = NOW()
		WHERE id = $1 AND status = 'dead'`, id)
	if err != nil {
		return mapError(err, "")
	}
	if tag.RowsAffected() == 0 {
		return mapError(pgx.ErrNoRows, "执行实例不存在或不是死信")
	}
	return nil
}

// PurgeFinishedBefore 清理终态且完成时间早于 before 的实例（尝试记录随外键级联删除）。
// limit 限制单轮删除量，避免一次清理长时间持锁。
func (r *AsyncdJobRunRepository) PurgeFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	tag, err := r.db.ExecTag(ctx, `DELETE FROM asyncd_job_runs WHERE id IN (
		SELECT id FROM asyncd_job_runs
		WHERE status IN ('succeeded','dead') AND finished_at IS NOT NULL AND finished_at < $1
		ORDER BY finished_at ASC
		LIMIT $2
	)`, before, limit)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

var _ repository.AsyncdJobRunRepository = (*AsyncdJobRunRepository)(nil)
