-- 034：身份连接的「同步计划」收敛进 asyncd 定时任务（Spec：261015-syncer-schedule-into-asyncd §3）。
--
-- 目标：仓库里只保留**一套**定时语义。连接的计划（sync_config.schedule）从此不再由
-- syncer.schedule 每分钟扫描 + Go 侧 DueForSync 判定，而是变成 asyncd 的一条定时任务定义：
--   一个连接 → 一条 asyncd_jobs（owner_syncer_id 非空）
--   task_type = 'syncer.run'（复用既有处理器），unique_key = 'syncer:<id>'（与手工触发共用单飞键）
-- 于是 Cron 成为 asyncd 的一等能力（任务中心直接可见），而"到点即同步"只需一跳。
--
-- 本迁移做四件事：
--   1. asyncd_jobs 扩容：cron_expr / unique_key / owner_syncer_id / retired_at；
--   2. 放开 schedule_kind 到 interval|daily|cron，并补齐自洽性约束与索引；
--   3. **存量回填**：把已有连接的计划翻译成任务定义（漏了这一步 = 升级后所有连接的定时同步静默停摆）；
--   4. 退役代码声明的 syncer.schedule（不删行：它的历史执行记录要留）。
--
-- 回滚见 034_asyncd_owner_jobs_and_cron.down.sql（会删除连接的计划任务及其执行历史，属有损回滚）。

-- ── 1) 定义表扩容 ─────────────────────────────────────────────────────────────
ALTER TABLE asyncd_jobs
    ADD COLUMN cron_expr       TEXT        NOT NULL DEFAULT '',
    ADD COLUMN unique_key      TEXT        NOT NULL DEFAULT '',
    ADD COLUMN owner_syncer_id BIGINT      REFERENCES syncers(id) ON DELETE CASCADE,
    ADD COLUMN retired_at      TIMESTAMPTZ;

COMMENT ON COLUMN asyncd_jobs.cron_expr IS 'schedule_kind=cron 时的 5 段表达式（分 时 日 月 周）';
COMMENT ON COLUMN asyncd_jobs.unique_key IS '声明式单飞键（空 = 调度器回落 job:<name>）；连接的计划写 syncer:<id> 与手工触发共用';
COMMENT ON COLUMN asyncd_jobs.owner_syncer_id IS '归属的身份连接（非空 = 该定义由连接持有，界面不提供启停）';
COMMENT ON COLUMN asyncd_jobs.retired_at IS '代码声明退役时间（保留历史；目录默认不列）';

-- ── 2) 值域与自洽性约束 ───────────────────────────────────────────────────────
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_schedule_kind_check;
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_schedule_kind_check
    CHECK (schedule_kind IN ('','interval','daily','cron'));
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_cron_check
    CHECK (schedule_kind <> 'cron' OR cron_expr <> '');
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_owner_scheduled_check
    CHECK (owner_syncer_id IS NULL OR kind = 'scheduled');

-- 一个连接最多一个计划任务
CREATE UNIQUE INDEX asyncd_jobs_owner_syncer_key ON asyncd_jobs (owner_syncer_id)
    WHERE owner_syncer_id IS NOT NULL;
CREATE INDEX asyncd_jobs_owner_syncer_idx ON asyncd_jobs (owner_syncer_id) WHERE retired_at IS NULL;

-- 到点扫描只认活跃定义（退役的代码声明不再被领取）
DROP INDEX IF EXISTS asyncd_jobs_due_idx;
CREATE INDEX asyncd_jobs_due_idx ON asyncd_jobs (next_run_at)
    WHERE enabled AND kind = 'scheduled' AND retired_at IS NULL;

-- ── 3) 存量回填：连接的计划 → 任务定义 ────────────────────────────────────────
--
-- 规则映射（与 internal/service/syncer_service.go 的 validateSyncSchedule 同源）：
--   mode=cron           → schedule_kind=cron（cron_expr 必填）
--   mode=daily          → schedule_kind=daily（daily_times 1..24 个 HH:MM）
--   mode=interval       → schedule_kind=interval（interval_seconds > 0）
--   mode='' + 历史 syncers.schedule 是纯数字且 > 0 → schedule_kind=interval（旧字段兼容）
--   其余（没有计划）    → 不生成任务
--
-- next_run_at 按 id 抖动 0..59 秒：避免升级瞬间 N 条计划同时到点。每条计划会在抖动窗口内先跑一次
-- （用于验证新链路），之后严格按自己的规则推进。
WITH planned AS (
    SELECT
        s.id,
        s.institution_id,
        s.is_enabled,
        COALESCE(s.sync_config->'schedule'->>'mode', '')                                   AS mode,
        COALESCE(NULLIF(s.sync_config->'schedule'->>'interval_seconds', '')::int, 0)        AS interval_seconds,
        COALESCE(s.sync_config->'schedule'->>'cron', '')                                   AS cron_expr,
        COALESCE(NULLIF(s.sync_config->'schedule'->>'window_minutes', '')::int, 30)         AS window_minutes,
        COALESCE((
            SELECT array_agg(t ORDER BY t)
            FROM jsonb_array_elements_text(s.sync_config->'schedule'->'times') AS t
        ), '{}'::text[])                                                                   AS daily_times,
        CASE WHEN s.schedule ~ '^[0-9]+$' THEN s.schedule::bigint ELSE 0 END               AS legacy_seconds
    FROM syncers s
), resolved AS (
    SELECT
        p.*,
        CASE
            WHEN p.mode = 'cron' THEN 'cron'
            WHEN p.mode = 'daily' THEN 'daily'
            WHEN p.mode = 'interval' THEN 'interval'
            WHEN p.mode = '' AND p.legacy_seconds > 0 THEN 'interval'
            ELSE ''
        END AS schedule_kind,
        CASE
            WHEN p.mode = 'interval' THEN p.interval_seconds
            WHEN p.mode = '' THEN p.legacy_seconds::int
            ELSE 0
        END AS effective_interval
    FROM planned p
)
INSERT INTO asyncd_jobs (
    name, task_type, kind, schedule_kind, interval_seconds, daily_times, cron_expr,
    window_minutes, payload, enabled, unique_key, next_run_at, owner_syncer_id)
SELECT
    'syncer.schedule:' || r.id,
    'syncer.run',
    'scheduled',
    r.schedule_kind,
    r.effective_interval,
    r.daily_times,
    r.cron_expr,
    r.window_minutes,
    jsonb_build_object('syncer_id', r.id, 'institution_id', r.institution_id, 'trigger', 'schedule'),
    r.is_enabled,
    'syncer:' || r.id,
    NOW() + make_interval(secs => (r.id % 60)::int),
    r.id
FROM resolved r
WHERE (r.schedule_kind = 'cron'     AND r.cron_expr <> '')
   OR (r.schedule_kind = 'daily'    AND cardinality(r.daily_times) BETWEEN 1 AND 24)
   OR (r.schedule_kind = 'interval' AND r.effective_interval > 0)
ON CONFLICT (name) DO NOTHING;

-- ── 4) 退役代码声明的扫描任务（不删行：保留历史执行记录）──────────────────────
UPDATE asyncd_jobs SET retired_at = NOW(), enabled = FALSE, updated_at = NOW()
WHERE name = 'syncer.schedule' AND owner_syncer_id IS NULL AND retired_at IS NULL;
