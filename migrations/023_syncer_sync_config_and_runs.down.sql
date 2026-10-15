DROP INDEX IF EXISTS syncer_runs_syncer_started_idx;
ALTER TABLE syncer_runs DROP COLUMN errors, DROP COLUMN duration_ms, DROP COLUMN trigger;
ALTER TABLE syncers DROP COLUMN target_department_id, DROP COLUMN sync_config;
