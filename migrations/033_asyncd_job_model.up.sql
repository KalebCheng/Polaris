-- 033：asyncd 四维度建模 —— 任务定义 / 执行实例 / 执行记录 / 执行器实例。
--
-- 旧模型（025 + 032）把四个维度压成两张表：asyncd_tasks 既当队列又当历史，
-- asyncd_schedules 只是"定时怎么入队"，异步任务的定义只在进程内注册表里。
-- 本迁移拆成四张表（见 docs/specs/261015-asyncd-task-model.md §4）；
-- 表名遵循仓库的「<主体>_<东西>」惯例（先例：syncer_runs）：
--   asyncd_jobs                任务定义（定时只是 kind=scheduled 的一种）
--   asyncd_job_runs            执行实例（队列 + 状态机 + 租约 + 单飞键）
--   asyncd_job_run_attempts    执行记录（每次尝试一行，append-only）
--   asyncd_workers             执行器实例（注册 + 心跳）
--
-- 状态机改名：pending→queued / active→running / completed→succeeded / archived→dead。
-- 归属收敛：runs.type + runs.job_name → runs.job_id（外键指向定义，消灭双真相）。
--
-- 回滚见 033_asyncd_job_model.down.sql（有损：不回填已删除的 type/job_name，且删除 kind='async' 的定义）。

-- ── 1) asyncd_schedules → asyncd_jobs（补 kind：存量全是定时任务）────────────────
ALTER TABLE asyncd_schedules RENAME TO asyncd_jobs;
ALTER TABLE asyncd_jobs RENAME COLUMN job_name TO name;
ALTER TABLE asyncd_jobs RENAME COLUMN kind TO schedule_kind;
ALTER TABLE asyncd_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'scheduled';

ALTER TABLE asyncd_jobs RENAME CONSTRAINT asyncd_schedules_pkey TO asyncd_jobs_pkey;
ALTER TABLE asyncd_jobs RENAME CONSTRAINT asyncd_schedules_job_name_key TO asyncd_jobs_name_key;

-- 旧 last_status: idle | enqueued | failed；新值域 idle | queued | skipped | failed
UPDATE asyncd_jobs SET last_status = 'queued' WHERE last_status = 'enqueued';

ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_kind_check          CHECK (kind IN ('async','scheduled'));
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_schedule_kind_check CHECK (schedule_kind IN ('','interval','daily'));
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_async_no_rule       CHECK (kind <> 'async' OR schedule_kind = '');
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_scheduled_has_rule  CHECK (kind <> 'scheduled' OR schedule_kind <> '');
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_interval_check      CHECK (schedule_kind <> 'interval' OR interval_seconds > 0);
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_daily_times_check   CHECK (schedule_kind <> 'daily' OR cardinality(daily_times) BETWEEN 1 AND 24);

DROP INDEX IF EXISTS asyncd_schedules_due_idx;
CREATE INDEX asyncd_jobs_due_idx ON asyncd_jobs (next_run_at) WHERE enabled AND kind = 'scheduled';

-- ── 2) asyncd_tasks → asyncd_job_runs（状态改名 + 领取者改名）──────────────────
ALTER TABLE asyncd_tasks RENAME TO asyncd_job_runs;
ALTER TABLE asyncd_job_runs RENAME COLUMN locked_by TO worker_id;
ALTER TABLE asyncd_job_runs RENAME CONSTRAINT asyncd_tasks_pkey TO asyncd_job_runs_pkey;

UPDATE asyncd_job_runs SET status = CASE status
    WHEN 'pending'   THEN 'queued'
    WHEN 'active'    THEN 'running'
    WHEN 'completed' THEN 'succeeded'
    WHEN 'archived'  THEN 'dead'
    ELSE status END;

ALTER TABLE asyncd_job_runs ADD CONSTRAINT asyncd_job_runs_status_check
    CHECK (status IN ('queued','running','succeeded','dead'));

-- 定时任务单飞键前缀随语义改名（调度器现在写 job:<name>）：
-- 不改历史行的话，旧键 schedule:<name> 与新键 job:<name> 会被当作两个键，同一窗口可能跑两次。
UPDATE asyncd_job_runs SET unique_key = 'job:' || substring(unique_key FROM 10)
WHERE unique_key LIKE 'schedule:%';

-- ── 3) 归属收敛：type / job_name → job_id ─────────────────────────────────────
ALTER TABLE asyncd_job_runs ADD COLUMN job_id BIGINT;

-- 先为"跑过但定义表里没有"的归属名补一份异步任务定义（名字取 job_name，其次 type）
INSERT INTO asyncd_jobs (name, task_type, kind, schedule_kind, enabled)
SELECT DISTINCT ON (COALESCE(NULLIF(job_name, ''), type))
       COALESCE(NULLIF(job_name, ''), type), type, 'async', '', TRUE
FROM asyncd_job_runs
WHERE COALESCE(NULLIF(job_name, ''), type) NOT IN (SELECT name FROM asyncd_jobs)
ORDER BY COALESCE(NULLIF(job_name, ''), type), type;

UPDATE asyncd_job_runs r SET job_id = j.id
FROM asyncd_jobs j
WHERE j.name = COALESCE(NULLIF(r.job_name, ''), r.type);

-- 归属解析不出来的行必须让迁移失败，而不是留下 NULL 归属
ALTER TABLE asyncd_job_runs ALTER COLUMN job_id SET NOT NULL;
ALTER TABLE asyncd_job_runs ADD CONSTRAINT asyncd_job_runs_job_id_fkey
    FOREIGN KEY (job_id) REFERENCES asyncd_jobs(id) ON DELETE CASCADE;

ALTER TABLE asyncd_job_runs DROP COLUMN type;
ALTER TABLE asyncd_job_runs DROP COLUMN job_name;

-- ── 4) 执行实例的索引按新表名/新状态重建 ───────────────────────────────────────
DROP INDEX IF EXISTS asyncd_tasks_ready_idx;
DROP INDEX IF EXISTS asyncd_tasks_lease_idx;
DROP INDEX IF EXISTS asyncd_tasks_unique_key_idx;
DROP INDEX IF EXISTS asyncd_tasks_type_status_idx;
DROP INDEX IF EXISTS asyncd_tasks_job_idx;

CREATE INDEX asyncd_job_runs_ready_idx ON asyncd_job_runs (priority DESC, next_attempt_at ASC) WHERE status = 'queued';
CREATE INDEX asyncd_job_runs_lease_idx ON asyncd_job_runs (lease_expires_at)                 WHERE status = 'running';
CREATE UNIQUE INDEX asyncd_job_runs_unique_key_idx ON asyncd_job_runs (unique_key)
    WHERE unique_key <> '' AND status IN ('queued','running');
CREATE INDEX asyncd_job_runs_job_id_idx   ON asyncd_job_runs (job_id, status, id DESC);
CREATE INDEX asyncd_job_runs_worker_idx   ON asyncd_job_runs (worker_id, status) WHERE worker_id <> '';
CREATE INDEX asyncd_job_runs_finished_idx ON asyncd_job_runs (finished_at)       WHERE finished_at IS NOT NULL;

-- ── 5) 执行记录（每次尝试一行）────────────────────────────────────────────────
CREATE TABLE asyncd_job_run_attempts (
    id          BIGSERIAL PRIMARY KEY,
    job_run_id  BIGINT      NOT NULL REFERENCES asyncd_job_runs(id) ON DELETE CASCADE,
    attempt_no  INTEGER     NOT NULL,                                  -- 第几次尝试（与 job_runs.attempts 对齐）
    worker_id   TEXT        NOT NULL,                                  -- 执行它的槽位
    status      TEXT        NOT NULL DEFAULT 'running',                -- running | succeeded | failed | lease_lost
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    duration_ms INTEGER     NOT NULL DEFAULT 0,
    error       TEXT        NOT NULL DEFAULT '',
    CONSTRAINT asyncd_job_run_attempts_status_check
        CHECK (status IN ('running','succeeded','failed','lease_lost')),
    CONSTRAINT asyncd_job_run_attempts_no_key UNIQUE (job_run_id, attempt_no)
);
CREATE INDEX asyncd_job_run_attempts_job_run_idx ON asyncd_job_run_attempts (job_run_id, attempt_no);
CREATE INDEX asyncd_job_run_attempts_open_idx    ON asyncd_job_run_attempts (started_at) WHERE status = 'running';

-- ── 6) 执行器实例（注册 + 心跳；在线状态按心跳计算，不存列）────────────────────
CREATE TABLE asyncd_workers (
    id                TEXT        PRIMARY KEY,                -- <host>-<pid>#<slot>
    instance_id       TEXT        NOT NULL,                   -- <host>-<pid>（一个服务进程）
    host              TEXT        NOT NULL,
    pid               INTEGER     NOT NULL,
    slot_index        INTEGER     NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at        TIMESTAMPTZ,                            -- 优雅退出时写入
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX asyncd_workers_heartbeat_idx ON asyncd_workers (last_heartbeat_at DESC);
