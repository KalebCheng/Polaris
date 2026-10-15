-- 033 回滚：四张表还原成 025 + 032 的形态。
--
-- **有损**：asyncd_job_runs.type / job_name 已删除，这里按 job_id 反查定义回填；
-- kind='async' 的任务定义会被删除（旧模型没有"异步任务定义"这一维度）；
-- 执行记录（asyncd_job_run_attempts）与执行器（asyncd_workers）整表丢弃。

DROP TABLE IF EXISTS asyncd_job_run_attempts;
DROP TABLE IF EXISTS asyncd_workers;

ALTER TABLE asyncd_job_runs ADD COLUMN type     TEXT NOT NULL DEFAULT '';
ALTER TABLE asyncd_job_runs ADD COLUMN job_name TEXT NOT NULL DEFAULT '';

UPDATE asyncd_job_runs r
SET type = j.task_type,
    job_name = CASE WHEN j.kind = 'scheduled' THEN j.name ELSE '' END
FROM asyncd_jobs j
WHERE j.id = r.job_id;

ALTER TABLE asyncd_job_runs DROP CONSTRAINT asyncd_job_runs_job_id_fkey;
ALTER TABLE asyncd_job_runs DROP COLUMN job_id;
ALTER TABLE asyncd_job_runs DROP CONSTRAINT asyncd_job_runs_status_check;

UPDATE asyncd_job_runs SET status = CASE status
    WHEN 'queued'    THEN 'pending'
    WHEN 'running'   THEN 'active'
    WHEN 'succeeded' THEN 'completed'
    WHEN 'dead'      THEN 'archived'
    ELSE status END;

-- 单飞键前缀还原
UPDATE asyncd_job_runs SET unique_key = 'schedule:' || substring(unique_key FROM 5)
WHERE unique_key LIKE 'job:%';

ALTER TABLE asyncd_job_runs RENAME COLUMN worker_id TO locked_by;
ALTER TABLE asyncd_job_runs RENAME CONSTRAINT asyncd_job_runs_pkey TO asyncd_tasks_pkey;

DROP INDEX IF EXISTS asyncd_job_runs_ready_idx;
DROP INDEX IF EXISTS asyncd_job_runs_lease_idx;
DROP INDEX IF EXISTS asyncd_job_runs_unique_key_idx;
DROP INDEX IF EXISTS asyncd_job_runs_job_id_idx;
DROP INDEX IF EXISTS asyncd_job_runs_worker_idx;
DROP INDEX IF EXISTS asyncd_job_runs_finished_idx;

ALTER TABLE asyncd_job_runs RENAME TO asyncd_tasks;

CREATE INDEX asyncd_tasks_ready_idx ON asyncd_tasks (priority DESC, next_attempt_at ASC) WHERE status = 'pending';
CREATE INDEX asyncd_tasks_lease_idx ON asyncd_tasks (lease_expires_at)                   WHERE status = 'active';
CREATE UNIQUE INDEX asyncd_tasks_unique_key_idx ON asyncd_tasks (unique_key)
    WHERE unique_key <> '' AND status IN ('pending','active');
CREATE INDEX asyncd_tasks_type_status_idx ON asyncd_tasks (type, status, id DESC);
CREATE INDEX asyncd_tasks_job_idx         ON asyncd_tasks (job_name, id DESC);

DELETE FROM asyncd_jobs WHERE kind = 'async';

ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_kind_check;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_schedule_kind_check;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_async_no_rule;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_scheduled_has_rule;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_interval_check;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_daily_times_check;

DROP INDEX IF EXISTS asyncd_jobs_due_idx;
CREATE INDEX asyncd_schedules_due_idx ON asyncd_jobs (next_run_at) WHERE enabled;

UPDATE asyncd_jobs SET last_status = 'enqueued' WHERE last_status = 'queued';
ALTER TABLE asyncd_jobs DROP COLUMN kind;
ALTER TABLE asyncd_jobs RENAME COLUMN schedule_kind TO kind;
ALTER TABLE asyncd_jobs RENAME COLUMN name TO job_name;
ALTER TABLE asyncd_jobs RENAME CONSTRAINT asyncd_jobs_pkey TO asyncd_schedules_pkey;
ALTER TABLE asyncd_jobs RENAME CONSTRAINT asyncd_jobs_name_key TO asyncd_schedules_job_name_key;
ALTER TABLE asyncd_jobs RENAME TO asyncd_schedules;
