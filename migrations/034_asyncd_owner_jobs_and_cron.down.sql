-- 034 回滚（有损）：删除连接持有的计划任务（其执行历史随外键级联消失），
-- 还原 schedule_kind 值域与到点索引，恢复代码声明的 syncer.schedule。
-- 注意：代码侧（DueForSync / runScheduledSyncers）需要 git 回滚，schema 回滚不会恢复行为。

DELETE FROM asyncd_jobs WHERE owner_syncer_id IS NOT NULL;

UPDATE asyncd_jobs SET retired_at = NULL, enabled = TRUE, updated_at = NOW()
WHERE name = 'syncer.schedule' AND owner_syncer_id IS NULL;

DROP INDEX IF EXISTS asyncd_jobs_owner_syncer_key;
DROP INDEX IF EXISTS asyncd_jobs_owner_syncer_idx;
DROP INDEX IF EXISTS asyncd_jobs_due_idx;
CREATE INDEX asyncd_jobs_due_idx ON asyncd_jobs (next_run_at) WHERE enabled AND kind = 'scheduled';

ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_owner_scheduled_check;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_cron_check;
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_schedule_kind_check;
DELETE FROM asyncd_jobs WHERE schedule_kind = 'cron';  -- 旧值域装不下 cron 定义
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_schedule_kind_check
    CHECK (schedule_kind IN ('','interval','daily'));

ALTER TABLE asyncd_jobs DROP COLUMN retired_at;
ALTER TABLE asyncd_jobs DROP COLUMN owner_syncer_id;
ALTER TABLE asyncd_jobs DROP COLUMN unique_key;
ALTER TABLE asyncd_jobs DROP COLUMN cron_expr;
