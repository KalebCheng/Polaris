DROP INDEX IF EXISTS asyncd_tasks_job_idx;
ALTER TABLE asyncd_tasks DROP COLUMN IF EXISTS job_name;
