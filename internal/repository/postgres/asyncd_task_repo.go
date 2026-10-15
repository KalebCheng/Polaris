package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const asyncdTaskColumns = `id, type, payload, payload_version, status, priority, attempts, max_attempts,
	next_attempt_at, deadline, unique_key, locked_by, lease_expires_at, last_error, institution_id,
	created_at, updated_at, finished_at`

// AsyncdTaskRepository 异步任务仓库（PostgreSQL 实现）
type AsyncdTaskRepository struct{ db *dbclient.Client }

// NewAsyncdTaskRepository 创建异步任务仓库
func NewAsyncdTaskRepository(db *dbclient.Client) *AsyncdTaskRepository {
	return &AsyncdTaskRepository{db: db}
}

func scanAsyncdTask(row interface{ Scan(...interface{}) error }) (*repository.AsyncdTask, error) {
	var t repository.AsyncdTask
	err := row.Scan(&t.ID, &t.Type, &t.Payload, &t.PayloadVersion, &t.Status, &t.Priority,
		&t.Attempts, &t.MaxAttempts, &t.NextAttemptAt, &t.Deadline, &t.UniqueKey, &t.LockedBy,
		&t.LeaseExpiresAt, &t.LastError, &t.InstitutionID, &t.CreatedAt, &t.UpdatedAt, &t.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Enqueue 入队。
// 单飞：unique_key 非空时，若已有 pending/active 的同键任务，**不新建**而是返回既有任务 id（existing=true）。
// 调用方通常处在业务事务里（outbox），因此这里不自己开事务。
func (r *AsyncdTaskRepository) Enqueue(ctx context.Context, task *repository.AsyncdTask) (int64, bool, error) {
	if task.UniqueKey != "" {
		var existingID int64
		err := r.db.QueryRow(ctx, `SELECT id FROM asyncd_tasks
			WHERE unique_key = $1 AND status IN ('pending','active') LIMIT 1`, task.UniqueKey).Scan(&existingID)
		if err == nil {
			return existingID, true, nil
		}
		if !isNoRows(err) {
			return 0, false, mapError(err, "")
		}
	}
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO asyncd_tasks
		(type, payload, payload_version, status, priority, max_attempts, next_attempt_at, deadline, unique_key, institution_id)
		VALUES ($1, COALESCE($2,'{}'::jsonb), $3, 'pending', $4, $5, COALESCE($6, NOW()), $7, $8, $9)
		RETURNING id`,
		task.Type, nullableJSONB(task.Payload), task.PayloadVersion, task.Priority, task.MaxAttempts,
		task.NextAttemptAt, task.Deadline, task.UniqueKey, task.InstitutionID,
	).Scan(&id)
	if err != nil {
		// 并发入队撞上单飞唯一索引：视为已存在（另一个请求刚入队）
		if isUniqueViolation(err) && task.UniqueKey != "" {
			var existingID int64
			if e := r.db.QueryRow(ctx, `SELECT id FROM asyncd_tasks
				WHERE unique_key = $1 AND status IN ('pending','active') LIMIT 1`, task.UniqueKey).Scan(&existingID); e == nil {
				return existingID, true, nil
			}
		}
		return 0, false, mapError(err, "")
	}
	task.ID = id
	return id, false, nil
}

// Claim 领取到期任务：SKIP LOCKED 保证多实例并发下互不阻塞且不重复领取；
// 同时顺带回收"自己看不到的过期租约"（在 ReclaimExpired 里统一处理）。
func (r *AsyncdTaskRepository) Claim(ctx context.Context, workerID string, leaseTTL time.Duration, limit int) ([]*repository.AsyncdTask, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(ctx, `UPDATE asyncd_tasks SET
			status = 'active', locked_by = $1, lease_expires_at = NOW() + $2::interval,
			attempts = attempts + 1, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM asyncd_tasks
			WHERE status = 'pending' AND next_attempt_at <= NOW()
			  AND (deadline IS NULL OR deadline > NOW())
			ORDER BY priority DESC, next_attempt_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+asyncdTaskColumns, workerID, leaseInterval(leaseTTL), limit)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var tasks []*repository.AsyncdTask
	for rows.Next() {
		task, err := scanAsyncdTask(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		tasks = append(tasks, task)
	}
	return tasks, mapError(rows.Err(), "")
}

// RenewLease 续租；返回 false 表示租约已被其他实例回收（调用方应停止执行并把结果视为丢失）。
func (r *AsyncdTaskRepository) RenewLease(ctx context.Context, id int64, workerID string, leaseTTL time.Duration) (bool, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_tasks SET lease_expires_at = NOW() + $3::interval, updated_at = NOW()
		WHERE id = $1 AND locked_by = $2 AND status = 'active'`, id, workerID, leaseInterval(leaseTTL))
	if err != nil {
		return false, mapError(err, "")
	}
	return tag.RowsAffected() > 0, nil
}

// Complete 标记完成
func (r *AsyncdTaskRepository) Complete(ctx context.Context, id int64, workerID string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_tasks SET status = 'completed', locked_by = '',
		lease_expires_at = NULL, finished_at = NOW(), updated_at = NOW(), last_error = ''
		WHERE id = $1 AND locked_by = $2`, id, workerID), "")
}

// Retry 记录失败：未达上限则退避重试（回到 pending），达到上限则入死信。
func (r *AsyncdTaskRepository) Retry(ctx context.Context, id int64, workerID, cause string, nextAttemptAt time.Time) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_tasks SET
			status = CASE WHEN attempts >= max_attempts THEN 'archived' ELSE 'pending' END,
			locked_by = '', lease_expires_at = NULL,
			next_attempt_at = COALESCE($3, next_attempt_at),
			last_error = $4, updated_at = NOW(),
			finished_at = CASE WHEN attempts >= max_attempts THEN NOW() ELSE NULL END
		WHERE id = $1 AND locked_by = $2`, id, workerID, nextAttemptAt, cause), "")
}

// Archive 直接入死信（超期、不可重试的错误）
func (r *AsyncdTaskRepository) Archive(ctx context.Context, id int64, workerID, cause string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_tasks SET status = 'archived', locked_by = '',
		lease_expires_at = NULL, last_error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND locked_by = $2`, id, workerID, cause), "")
}

// ReclaimExpired 回收租约过期仍处于 active 的任务（worker 崩溃恢复）。
// 计数已加过（领取时 +1），这里只把状态放回 pending，由后续领取继续处理；超过上限则入死信。
func (r *AsyncdTaskRepository) ReclaimExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_tasks SET
			status = CASE WHEN attempts >= max_attempts THEN 'archived' ELSE 'pending' END,
			locked_by = '', lease_expires_at = NULL,
			last_error = CASE WHEN attempts >= max_attempts
				THEN 'worker lease expired and attempts exhausted' ELSE last_error END,
			finished_at = CASE WHEN attempts >= max_attempts THEN NOW() ELSE NULL END,
			updated_at = NOW()
		WHERE id IN (
			SELECT id FROM asyncd_tasks
			WHERE status = 'active' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW()
			ORDER BY lease_expires_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)`, limit)
	if err != nil {
		return 0, mapError(err, "")
	}
	return int(tag.RowsAffected()), nil
}

// Counts 按状态计数
func (r *AsyncdTaskRepository) Counts(ctx context.Context, taskType string) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT status, COUNT(*) FROM asyncd_tasks
		WHERE ($1 = '' OR type = $1) GROUP BY status`, taskType)
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

// List 列出任务（管理界面）
func (r *AsyncdTaskRepository) List(ctx context.Context, status, taskType string, offset, limit int) ([]*repository.AsyncdTask, error) {
	b := &queryBuilder{}
	if status != "" {
		b.add(`status = ` + b.arg(status))
	}
	if taskType != "" {
		b.add(`type = ` + b.arg(taskType))
	}
	sql := `SELECT ` + asyncdTaskColumns + ` FROM asyncd_tasks` + b.where() + b.orderLimit("id DESC", offset, limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdTask
	for rows.Next() {
		task, err := scanAsyncdTask(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, task)
	}
	return out, mapError(rows.Err(), "")
}

// RetryArchived 人工重试死信任务：回到 pending 并清零重试计数（保留历史 last_error 便于排查）
func (r *AsyncdTaskRepository) RetryArchived(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_tasks SET status = 'pending', attempts = 0,
		next_attempt_at = NOW(), locked_by = '', lease_expires_at = NULL, finished_at = NULL,
		updated_at = NOW()
		WHERE id = $1 AND status = 'archived'`, id), "task not found")
}

var _ repository.AsyncdTaskRepository = (*AsyncdTaskRepository)(nil)
