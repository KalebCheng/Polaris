package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// AsyncdWorkerRepository 执行器实例仓库（PostgreSQL 实现）
type AsyncdWorkerRepository struct{ db *dbclient.Client }

// NewAsyncdWorkerRepository 创建执行器实例仓库
func NewAsyncdWorkerRepository(db *dbclient.Client) *AsyncdWorkerRepository {
	return &AsyncdWorkerRepository{db: db}
}

// Register 登记/复活一个执行器槽位（进程启动时调用；同一 id 重复登记只刷新心跳与启动信息）
func (r *AsyncdWorkerRepository) Register(ctx context.Context, worker *repository.AsyncdWorker) error {
	return mapError(r.db.Exec(ctx, `INSERT INTO asyncd_workers
			(id, instance_id, host, pid, slot_index, started_at, last_heartbeat_at)
		VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
		ON CONFLICT (id) DO UPDATE SET
			instance_id = EXCLUDED.instance_id,
			host = EXCLUDED.host,
			pid = EXCLUDED.pid,
			slot_index = EXCLUDED.slot_index,
			started_at = NOW(),
			last_heartbeat_at = NOW(),
			stopped_at = NULL,
			updated_at = NOW()`,
		worker.ID, worker.InstanceID, worker.Host, worker.PID, worker.SlotIndex), "")
}

// Heartbeat 心跳续期（固定间隔调用）
func (r *AsyncdWorkerRepository) Heartbeat(ctx context.Context, workerID string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_workers SET last_heartbeat_at = NOW(), updated_at = NOW()
		WHERE id = $1`, workerID), "执行器槽位不存在")
}

// Stop 标记优雅退出
func (r *AsyncdWorkerRepository) Stop(ctx context.Context, workerID string) error {
	return mapError(r.db.Exec(ctx, `UPDATE asyncd_workers SET stopped_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND stopped_at IS NULL`, workerID), "")
}

// List 列出执行器槽位与实时在跑任务数（在跑数按 runs.worker_id 聚合，不存列，避免漂移）
func (r *AsyncdWorkerRepository) List(ctx context.Context) ([]repository.AsyncdWorkerStat, error) {
	rows, err := r.db.Query(ctx, `SELECT w.id, w.instance_id, w.host, w.pid, w.slot_index,
			w.started_at, w.last_heartbeat_at, w.stopped_at, w.created_at, w.updated_at,
			COALESCE(active.running, 0)
		FROM asyncd_workers w
		LEFT JOIN (
			SELECT worker_id, COUNT(*) AS running FROM asyncd_job_runs
			WHERE status = 'running' GROUP BY worker_id
		) active ON active.worker_id = w.id
		ORDER BY w.instance_id ASC, w.slot_index ASC`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []repository.AsyncdWorkerStat
	for rows.Next() {
		var stat repository.AsyncdWorkerStat
		if err := rows.Scan(&stat.Worker.ID, &stat.Worker.InstanceID, &stat.Worker.Host, &stat.Worker.PID,
			&stat.Worker.SlotIndex, &stat.Worker.StartedAt, &stat.Worker.LastHeartbeatAt,
			&stat.Worker.StoppedAt, &stat.Worker.CreatedAt, &stat.Worker.UpdatedAt,
			&stat.Running); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, stat)
	}
	return out, mapError(rows.Err(), "")
}

// CountOnline 在线槽位数（阈值内的心跳且未停止）
func (r *AsyncdWorkerRepository) CountOnline(ctx context.Context, staleAfter time.Duration) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM asyncd_workers
		WHERE stopped_at IS NULL AND last_heartbeat_at > NOW() - $1::interval`,
		intervalString(staleAfter)).Scan(&count)
	if err != nil {
		return 0, mapError(err, "")
	}
	return count, nil
}

// PurgeStaleBefore 清理停止/失联早于 before 的执行器行（历史任务的执行者记录不受影响：不加外键）
func (r *AsyncdWorkerRepository) PurgeStaleBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.ExecTag(ctx, `DELETE FROM asyncd_workers
		WHERE COALESCE(stopped_at, last_heartbeat_at) < $1`, before)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

var _ repository.AsyncdWorkerRepository = (*AsyncdWorkerRepository)(nil)
