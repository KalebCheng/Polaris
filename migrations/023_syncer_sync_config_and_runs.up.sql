-- 023：入方向的同步范围/计划/保护 与 运行明细
--
-- 背景（Spec FR-7 / FR-14 / FR-5）：
--   - 「同步配置」需要承载：同步目标节点、增量开关、定时校验（每天多时刻或 Cron + 执行窗口）、同步保护阈值；
--   - 试运行与正式运行共用同一差异计算，需要把触发来源、耗时与失败明细写进运行记录。
--
-- 回滚：023_syncer_sync_config_and_runs.down.sql

-- 同步配置（JSONB）：范围 / 调度 / 保护
ALTER TABLE syncers ADD COLUMN sync_config JSONB NOT NULL DEFAULT '{}'::jsonb;
-- { "incremental": false,
--   "schedule": { "mode": "daily", "times": ["03:00","05:00"], "cron": "", "window_minutes": 30 },
--   "guard":    { "max_user_deletes": 30, "max_dept_deletes": 10 } }

-- 同步目标：外部组织与账号挂到该部门节点之下（为空表示组织根）
ALTER TABLE syncers ADD COLUMN target_department_id BIGINT REFERENCES departments(id) ON DELETE SET NULL;

-- 运行明细：触发来源 / 耗时 / 逐条失败原因
ALTER TABLE syncer_runs ADD COLUMN trigger     TEXT NOT NULL DEFAULT 'manual'; -- manual | schedule | dry-run
ALTER TABLE syncer_runs ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE syncer_runs ADD COLUMN errors      JSONB NOT NULL DEFAULT '[]'::jsonb;
CREATE INDEX syncer_runs_syncer_started_idx ON syncer_runs (syncer_id, started_at DESC);
