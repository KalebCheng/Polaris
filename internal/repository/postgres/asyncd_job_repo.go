package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// asyncdJobColumns 定义表的列清单（写操作用；读取走 asyncdJobSelect 以带上归属连接名）
const asyncdJobColumns = `id, name, task_type, kind, schedule_kind, interval_seconds, daily_times,
	window_minutes, cron_expr, payload, enabled, unique_key, next_run_at, last_run_at, last_status,
	owner_syncer_id, retired_at, created_at, updated_at`

// asyncdJobSelect 读取定义：LEFT JOIN 归属连接带出 owner_name（界面上显示"来源：身份连接 xxx"）
const asyncdJobSelect = `SELECT j.id, j.name, j.task_type, j.kind, j.schedule_kind, j.interval_seconds,
		j.daily_times, j.window_minutes, j.cron_expr, j.payload, j.enabled, j.unique_key,
		j.next_run_at, j.last_run_at, j.last_status, j.owner_syncer_id, j.retired_at,
		j.created_at, j.updated_at, COALESCE(s.name, '')
	FROM asyncd_jobs j
	LEFT JOIN syncers s ON s.id = j.owner_syncer_id`

// AsyncdJobRepository 任务定义仓库（PostgreSQL 实现）
type AsyncdJobRepository struct{ db *dbclient.Client }

// NewAsyncdJobRepository 创建任务定义仓库
func NewAsyncdJobRepository(db *dbclient.Client) *AsyncdJobRepository {
	return &AsyncdJobRepository{db: db}
}

func scanAsyncdJob(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJob, error) {
	var j repository.AsyncdJob
	err := row.Scan(&j.ID, &j.Name, &j.TaskType, &j.Kind, &j.ScheduleKind, &j.IntervalSeconds,
		&j.DailyTimes, &j.WindowMinutes, &j.CronExpr, &j.Payload, &j.Enabled, &j.UniqueKey,
		&j.NextRunAt, &j.LastRunAt, &j.LastStatus, &j.OwnerSyncerID, &j.RetiredAt,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// scanAsyncdJobWithOwnerName 读取路径多一列 owner_name（join 带出）
func scanAsyncdJobWithOwnerName(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJob, error) {
	var j repository.AsyncdJob
	err := row.Scan(&j.ID, &j.Name, &j.TaskType, &j.Kind, &j.ScheduleKind, &j.IntervalSeconds,
		&j.DailyTimes, &j.WindowMinutes, &j.CronExpr, &j.Payload, &j.Enabled, &j.UniqueKey,
		&j.NextRunAt, &j.LastRunAt, &j.LastStatus, &j.OwnerSyncerID, &j.RetiredAt,
		&j.CreatedAt, &j.UpdatedAt, &j.OwnerName)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// Upsert 注册/更新**代码声明**的任务定义。
//
// 刻意不覆盖 enabled / next_run_at / last_* —— 启停是运维在界面上的决定，重启不该把它掰回代码默认值；
// next_run_at 只在新建时初始化，避免重启打乱节奏。retired_at 会被清空（代码重新声明 = 该任务回来了）。
func (r *AsyncdJobRepository) Upsert(ctx context.Context, job *repository.AsyncdJob) error {
	next := job.NextRunAt
	if next.IsZero() {
		next = time.Now()
	}
	return mapError(r.db.Exec(ctx, `INSERT INTO asyncd_jobs
			(name, task_type, kind, schedule_kind, interval_seconds, daily_times, window_minutes, cron_expr, payload, enabled, unique_key, next_run_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9,'{}'::jsonb),$10,$11,$12)
		ON CONFLICT (name) DO UPDATE SET
			task_type = EXCLUDED.task_type,
			kind = EXCLUDED.kind,
			schedule_kind = EXCLUDED.schedule_kind,
			interval_seconds = EXCLUDED.interval_seconds,
			daily_times = EXCLUDED.daily_times,
			window_minutes = EXCLUDED.window_minutes,
			cron_expr = EXCLUDED.cron_expr,
			payload = EXCLUDED.payload,
			unique_key = EXCLUDED.unique_key,
			retired_at = NULL,
			updated_at = NOW()`,
		job.Name, job.TaskType, job.Kind, job.ScheduleKind, job.IntervalSeconds, orEmpty(job.DailyTimes),
		job.WindowMinutes, job.CronExpr, nullableJSONB(job.Payload), job.Enabled, job.UniqueKey, next), "")
}

// UpsertOwned 写入**数据声明**的任务定义（身份连接的同步计划）。
//
// 与代码声明的区别：这里 enabled 是连接状态的投影、规则会被覆盖、
// resetNextRunAt=true 时按新规则重置 next_run_at（否则"改了计划要等旧的下次时间才生效"）。
func (r *AsyncdJobRepository) UpsertOwned(ctx context.Context, job *repository.AsyncdJob, resetNextRunAt bool) error {
	next := job.NextRunAt
	if next.IsZero() {
		next = time.Now()
	}
	return mapError(r.db.Exec(ctx, `INSERT INTO asyncd_jobs
			(name, task_type, kind, schedule_kind, interval_seconds, daily_times, window_minutes, cron_expr,
			 payload, enabled, unique_key, next_run_at, owner_syncer_id)
		VALUES ($1,$2,'scheduled',$3,$4,$5,$6,$7,COALESCE($8,'{}'::jsonb),$9,$10,$11,$12)
		ON CONFLICT (name) DO UPDATE SET
			task_type = EXCLUDED.task_type,
			kind = 'scheduled',
			schedule_kind = EXCLUDED.schedule_kind,
			interval_seconds = EXCLUDED.interval_seconds,
			daily_times = EXCLUDED.daily_times,
			window_minutes = EXCLUDED.window_minutes,
			cron_expr = EXCLUDED.cron_expr,
			payload = EXCLUDED.payload,
			enabled = EXCLUDED.enabled,
			unique_key = EXCLUDED.unique_key,
			owner_syncer_id = EXCLUDED.owner_syncer_id,
			retired_at = NULL,
			next_run_at = CASE WHEN $13 THEN EXCLUDED.next_run_at ELSE asyncd_jobs.next_run_at END,
			updated_at = NOW()`,
		job.Name, job.TaskType, job.ScheduleKind, job.IntervalSeconds, orEmpty(job.DailyTimes),
		job.WindowMinutes, job.CronExpr, nullableJSONB(job.Payload), job.Enabled, job.UniqueKey,
		next, job.OwnerSyncerID, resetNextRunAt), "")
}

// SetEnabledByOwner 按归属连接启停其计划任务
func (r *AsyncdJobRepository) SetEnabledByOwner(ctx context.Context, ownerSyncerID int64, enabled bool) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_jobs SET enabled = $2, updated_at = NOW()
		WHERE owner_syncer_id = $1`, ownerSyncerID, enabled), "")
}

// RetireUndeclared 把"本次启动未声明且无归属"的定义标记退役：保留历史执行记录，但不再被调度、不再出现在目录里。
// 代码里删掉一个任务声明时，走这条路而不是 DELETE —— 删定义会 CASCADE 掉它的执行历史。
func (r *AsyncdJobRepository) RetireUndeclared(ctx context.Context, activeNames []string, at time.Time) (int64, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_jobs
		SET retired_at = $2, enabled = FALSE, updated_at = NOW()
		WHERE owner_syncer_id IS NULL AND retired_at IS NULL AND name <> ALL($1::text[])`,
		activeNames, at)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

// GetByName 按定义名取（入队时解析处理器与默认策略）
func (r *AsyncdJobRepository) GetByName(ctx context.Context, name string) (*repository.AsyncdJob, error) {
	row := r.db.QueryRow(ctx, asyncdJobSelect+` WHERE j.name = $1`, name)
	job, err := scanAsyncdJobWithOwnerName(row)
	if err != nil {
		return nil, mapError(err, "任务定义不存在")
	}
	return job, nil
}

// GetByID 按主键取定义
func (r *AsyncdJobRepository) GetByID(ctx context.Context, id int64) (*repository.AsyncdJob, error) {
	row := r.db.QueryRow(ctx, asyncdJobSelect+` WHERE j.id = $1`, id)
	job, err := scanAsyncdJobWithOwnerName(row)
	if err != nil {
		return nil, mapError(err, "任务定义不存在")
	}
	return job, nil
}

// ClaimDue 抢占到点的定时任务（多实例下同一时刻只有一个实例能拿到）。
//
// 机制：把被抢到的行 next_run_at 临时推进到一个"占位值"（now + inFlight），
// 抢占与读回在同一条 UPDATE ... RETURNING 里完成；随后由调用方算出真正的下次执行时间
// 并调用 MarkRun 写回。若调用方在两步之间崩溃，该任务会在 inFlight 之后被重新抢占。
// 退役的定义不参与（用 retired_at IS NULL 过滤，见 asyncd_jobs_due_idx）。
func (r *AsyncdJobRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*repository.AsyncdJob, error) {
	if limit <= 0 {
		limit = 20
	}
	const inFlight = 10 * time.Minute
	rows, err := r.db.Query(ctx, `UPDATE asyncd_jobs
		SET next_run_at = $2::timestamptz + $4::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM asyncd_jobs
			WHERE enabled AND kind = 'scheduled' AND retired_at IS NULL AND next_run_at <= $1
			ORDER BY next_run_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+asyncdJobColumns, now, now, limit, intervalString(inFlight))
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdJob
	for rows.Next() {
		job, err := scanAsyncdJob(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, job)
	}
	return out, mapError(rows.Err(), "")
}

// MarkRun 记录本次调度结果（queued|skipped|failed）并推进到下一次执行时间
func (r *AsyncdJobRepository) MarkRun(ctx context.Context, id int64, status string, nextRunAt time.Time, at time.Time) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_jobs SET last_run_at = $3, last_status = $2,
		next_run_at = $4, updated_at = NOW() WHERE id = $1`, id, status, at, nextRunAt), "任务定义不存在")
}

// List 列出任务定义（includeRetired=true 时含已退役的，供执行记录解析归属）
func (r *AsyncdJobRepository) List(ctx context.Context, includeRetired bool) ([]*repository.AsyncdJob, error) {
	condition := ` WHERE j.retired_at IS NULL`
	if includeRetired {
		condition = ""
	}
	rows, err := r.db.Query(ctx, asyncdJobSelect+condition+` ORDER BY j.kind ASC, j.name ASC`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdJob
	for rows.Next() {
		job, err := scanAsyncdJobWithOwnerName(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, job)
	}
	return out, mapError(rows.Err(), "")
}

// SetEnabled 启停定时任务（按主键；仅代码声明的定义——归属连接的由连接的计划控制）
func (r *AsyncdJobRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_jobs SET enabled = $2, updated_at = NOW()
		WHERE id = $1 AND kind = 'scheduled' AND owner_syncer_id IS NULL AND retired_at IS NULL`, id, enabled)
	if err != nil {
		return mapError(err, "")
	}
	if tag.RowsAffected() == 0 {
		return mapError(pgx.ErrNoRows, "任务定义不存在、不是定时任务，或由身份连接控制")
	}
	return nil
}

var _ repository.AsyncdJobRepository = (*AsyncdJobRepository)(nil)
