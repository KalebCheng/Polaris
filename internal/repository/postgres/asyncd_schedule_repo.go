package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const asyncdScheduleColumns = `id, job_name, task_type, payload, kind, interval_seconds, daily_times,
	window_minutes, enabled, next_run_at, last_run_at, last_status, created_at, updated_at`

// AsyncdScheduleRepository 定时调度仓库（PostgreSQL 实现）
type AsyncdScheduleRepository struct{ db *dbclient.Client }

// NewAsyncdScheduleRepository 创建调度仓库
func NewAsyncdScheduleRepository(db *dbclient.Client) *AsyncdScheduleRepository {
	return &AsyncdScheduleRepository{db: db}
}

func scanAsyncdSchedule(row interface{ Scan(...interface{}) error }) (*repository.AsyncdSchedule, error) {
	var s repository.AsyncdSchedule
	err := row.Scan(&s.ID, &s.JobName, &s.TaskType, &s.Payload, &s.Kind, &s.IntervalSeconds,
		&s.DailyTimes, &s.WindowMinutes, &s.Enabled, &s.NextRunAt, &s.LastRunAt, &s.LastStatus,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Upsert 注册/更新调度项（启动时按代码声明对齐；next_run_at 仅在新建时初始化，避免重启打乱节奏）。
func (r *AsyncdScheduleRepository) Upsert(ctx context.Context, s *repository.AsyncdSchedule) error {
	next := s.NextRunAt
	if next.IsZero() {
		next = time.Now()
	}
	return mapError(r.db.Exec(ctx, `INSERT INTO asyncd_schedules
			(job_name, task_type, payload, kind, interval_seconds, daily_times, window_minutes, enabled, next_run_at)
		VALUES ($1,$2,COALESCE($3,'{}'::jsonb),$4,$5,$6,$7,$8,$9)
		ON CONFLICT (job_name) DO UPDATE SET
			task_type = EXCLUDED.task_type,
			payload = EXCLUDED.payload,
			kind = EXCLUDED.kind,
			interval_seconds = EXCLUDED.interval_seconds,
			daily_times = EXCLUDED.daily_times,
			window_minutes = EXCLUDED.window_minutes,
			updated_at = NOW()`,
		s.JobName, s.TaskType, nullableJSONB(s.Payload), s.Kind, s.IntervalSeconds, orEmpty(s.DailyTimes),
		s.WindowMinutes, s.Enabled, next), "")
}

// ClaimDue 抢占到点的调度项（多实例下同一时刻只有一个实例能拿到）。
//
// 机制：把被抢到的行 next_run_at 临时推进到一个"占位值"（now + inFlight），
// 抢占与读回在同一条 UPDATE ... RETURNING 里完成；随后由调用方按调度规则算出
// 真正的下次执行时间并调用 MarkRun 写回。若调用方在两步之间崩溃，
// 该调度项会在 inFlight 之后被重新抢占（不会永久卡住）。
func (r *AsyncdScheduleRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]*repository.AsyncdSchedule, error) {
	if limit <= 0 {
		limit = 20
	}
	const inFlight = 10 * time.Minute
	rows, err := r.db.Query(ctx, `UPDATE asyncd_schedules
		SET next_run_at = $2::timestamptz + $4::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM asyncd_schedules
			WHERE enabled AND next_run_at <= $1
			ORDER BY next_run_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+asyncdScheduleColumns, now, now, limit, intervalString(inFlight))
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdSchedule
	for rows.Next() {
		s, err := scanAsyncdSchedule(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, s)
	}
	return out, mapError(rows.Err(), "")
}

// MarkRun 记录本次入队结果并推进到下一次执行时间
func (r *AsyncdScheduleRepository) MarkRun(ctx context.Context, id int64, status string, nextRunAt time.Time, at time.Time) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_schedules SET last_run_at = $3, last_status = $2,
		next_run_at = $4, updated_at = NOW() WHERE id = $1`, id, status, at, nextRunAt), "schedule not found")
}

// List 列出全部调度项
func (r *AsyncdScheduleRepository) List(ctx context.Context) ([]*repository.AsyncdSchedule, error) {
	rows, err := r.db.Query(ctx, `SELECT `+asyncdScheduleColumns+` FROM asyncd_schedules ORDER BY job_name ASC`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.AsyncdSchedule
	for rows.Next() {
		s, err := scanAsyncdSchedule(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, s)
	}
	return out, mapError(rows.Err(), "")
}

// SetEnabled 启停调度项
func (r *AsyncdScheduleRepository) SetEnabled(ctx context.Context, jobName string, enabled bool) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_schedules SET enabled = $2, updated_at = NOW()
		WHERE job_name = $1`, jobName, enabled), "schedule not found")
}

var _ repository.AsyncdScheduleRepository = (*AsyncdScheduleRepository)(nil)
