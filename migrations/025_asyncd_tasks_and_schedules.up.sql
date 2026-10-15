-- 024：异步任务与定时调度的基础（DB 版 asynq；多实例安全）
--
-- 设计要点（见 Spec §5.7）：
--   1. 任务入队与业务写入**同事务**（outbox）——Redis/外部 MQ 给不了这一点；
--   2. 领取用 FOR UPDATE SKIP LOCKED，多实例并发安全且不重复；
--   3. 租约（lease_expires_at）解决"worker 崩溃后 running 永久卡住"，到期由其他实例回收；
--   4. unique_key 部分唯一索引实现单飞（同一资源同时只跑一个任务）；
--   5. 定时任务**只负责入队**，执行路径唯一（worker 领取 → 处理器）。
--
-- 回滚：024_asyncd_tasks_and_schedules.down.sql

CREATE TABLE asyncd_tasks (
    id               BIGSERIAL PRIMARY KEY,
    type             TEXT        NOT NULL,
    payload          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    payload_version  INTEGER     NOT NULL DEFAULT 1,
    status           TEXT        NOT NULL DEFAULT 'pending', -- pending | active | completed | archived
    priority         INTEGER     NOT NULL DEFAULT 0,
    attempts         INTEGER     NOT NULL DEFAULT 0,
    max_attempts     INTEGER     NOT NULL DEFAULT 8,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),     -- 可见时间（延迟执行/退避）
    deadline         TIMESTAMPTZ,                            -- 任务级超时（超期按失败处理）
    unique_key       TEXT        NOT NULL DEFAULT '',        -- 单飞键（空 = 不去重）
    locked_by        TEXT        NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    last_error       TEXT        NOT NULL DEFAULT '',
    institution_id   BIGINT      REFERENCES institutions(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ
);

-- 领取索引：按状态 + 可见时间 + 优先级（部分索引，避免归档行拖慢领取）
CREATE INDEX asyncd_tasks_ready_idx
    ON asyncd_tasks (priority DESC, next_attempt_at ASC)
    WHERE status = 'pending';
-- 租约回收：找出过期仍在 active 的任务
CREATE INDEX asyncd_tasks_lease_idx
    ON asyncd_tasks (lease_expires_at)
    WHERE status = 'active';
-- 单飞：同一 unique_key 在 pending/active 下唯一
CREATE UNIQUE INDEX asyncd_tasks_unique_key_idx
    ON asyncd_tasks (unique_key)
    WHERE unique_key <> '' AND status IN ('pending', 'active');
-- 管理界面：按类型/状态/租户查看
CREATE INDEX asyncd_tasks_type_status_idx ON asyncd_tasks (type, status, id DESC);

CREATE TABLE asyncd_schedules (
    id            BIGSERIAL PRIMARY KEY,
    job_name      TEXT        NOT NULL UNIQUE, -- 处理器/任务名（唯一）
    task_type     TEXT        NOT NULL,        -- 入队时写入 asyncd_tasks.type
    payload       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    kind          TEXT        NOT NULL,        -- interval | daily
    interval_seconds INTEGER  NOT NULL DEFAULT 0,
    daily_times   TEXT[]      NOT NULL DEFAULT '{}', -- 形如 {03:00,05:00}（每天多时刻）
    window_minutes INTEGER    NOT NULL DEFAULT 30,   -- 执行窗口（到点前后该窗口内只执行一次）
    enabled       BOOLEAN     NOT NULL DEFAULT TRUE,
    next_run_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_run_at   TIMESTAMPTZ,
    last_status   TEXT        NOT NULL DEFAULT 'idle', -- idle | enqueued | failed
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX asyncd_schedules_due_idx ON asyncd_schedules (next_run_at) WHERE enabled;
