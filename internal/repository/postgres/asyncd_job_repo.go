package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const asyncdJobColumns = `id, name, task_type, kind, schedule_kind, interval_seconds, daily_times,
	window_minutes, payload, enabled, next_run_at, last_run_at, last_status, created_at, updated_at`

// AsyncdJobRepository 任务定义仓库（PostgreSQL 实现）
type AsyncdJobRepository struct{ db *dbclient.Client }

// NewAsyncdJobRepository 创建任务定义仓库
func NewAsyncdJobRepository(db *dbclient.Client) *AsyncdJobRepository {
	return &AsyncdJobRepository{db: db}
}

func scanAsyncdJob(row interface{ Scan(...interface{}) error }) (*repository.AsyncdJob, error) {
	var j repository.AsyncdJob
	err := row.Scan(&j.ID, &j.Name, &j.TaskType, &j.Kind, &j.ScheduleKind, &j.IntervalSeconds,
		&j.DailyTimes, &j.WindowMinutes, &j.Payload, &j.Enabled, &j.NextRunAt, &j.LastRunAt,
		&j.LastStatus, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// Upsert 注册/更新任务定义（启动时按代码声明对齐；next_run_at 仅在新建时初始化，避免重启打乱节奏）。
//
// 注意：**不覆盖 enabled**——启停是运维在界面上的决定，重启不该把它掰回代码声明的默认值
// （与 next_run_at 同理）。last_run_at/last_status 亦属运行态，同样不动。
func (r *AsyncdJobRepository) Upsert(ctx context.Context, job *repository.AsyncdJob) error {
	next := job.NextRunAt
	if next.IsZero() {
		next = time.Now()
	}
	return mapError(r.db.Exec(ctx, `INSERT INTO asyncd_jobs
			(name, task_type, kind, schedule_kind, interval_seconds, daily_times, window_minutes, payload, enabled, next_run_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8,'{}'::jsonb),$9,$10)
		ON CONFLICT (name) DO UPDATE SET
			task_type = EXCLUDED.task_type,
			kind = EXCLUDED.kind,
			schedule_kind = EXCLUDED.schedule_kind,
			interval_seconds = EXCLUDED.interval_seconds,
			daily_times = EXCLUDED.daily_times,
			window_minutes = EXCLUDED.window_minutes,
			payload = EXCLUDED.payload,
			updated_at = NOW()`,
		job.Name, job.TaskType, job.Kind, job.ScheduleKind, job.IntervalSeconds, orEmpty(job.DailyTimes),
		job.WindowMinutes, nullableJSONB(job.Payload), job.Enabled, next), "")
}

// GetByName 按定义名取（入队时解析处理器与默认策略）
func (r *AsyncdJobRepository) GetByName(ctx context.Context, name string) (*repository.AsyncdJob, error) {
	row := r.db.QueryRow(ctx, `SELECT `+asyncdJobColumns+` FROM asyncd_jobs WHERE name = $1`, name)
	job, err := scanAsyncdJob(row)
	if err != nil {
		return nil, mapError(err, "任务定义不存在")
	}
	return job, nil
}

// GetByID 按主键取定义
func (r *AsyncdJobRepository) GetByID(ctx context.Context, id int64) (*repository.AsyncdJob, error) {
	row := r.db.QueryRow(ctx, `SELECT `+asyncdJobColumns+` FROM asyncd_jobs WHERE id = $1`, id)
	job, err := scanAsyncdJob(row)
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
func (r *AsyncdJobRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*repository.AsyncdJob, error) {
	if limit <= 0 {
		limit = 20
	}
	const inFlight = 10 * time.Minute
	rows, err := r.db.Query(ctx, `UPDATE asyncd_jobs
		SET next_run_at = $2::timestamptz + $4::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM asyncd_jobs
			WHERE enabled AND kind = 'scheduled' AND next_run_at <= $1
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

// List 列出全部任务定义（任务目录）
func (r *AsyncdJobRepository) List(ctx context.Context) ([]*repository.AsyncdJob, error) {
	rows, err := r.db.Query(ctx, `SELECT `+asyncdJobColumns+` FROM asyncd_jobs
		ORDER BY kind ASC, name ASC`)
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

// SetEnabled 启停定时任务（异步任务的定义不接受停用，故限定 kind=scheduled；按主键）
func (r *AsyncdJobRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	tag, err := r.db.ExecTag(ctx, `UPDATE asyncd_jobs SET enabled = $2, updated_at = NOW()
		WHERE id = $1 AND kind = 'scheduled'`, id, enabled)
	if err != nil {
		return mapError(err, "")
	}
	if tag.RowsAffected() == 0 {
		return mapError(pgx.ErrNoRows, "任务定义不存在或不是定时任务")
	}
	return nil
}

var _ repository.AsyncdJobRepository = (*AsyncdJobRepository)(nil)
