-- 032：任务中心按「任务」维度聚合 —— asyncd_tasks 增加调度来源作业名。
--
-- 背景：调度器入队时只写了 unique_key = 'schedule:<job_name>'（作业名藏在字符串前缀里），
-- 任务中心的「定时任务 → 执行记录」需要一个**明确的归属列**：
--   * 定时任务：job_name = asyncd_schedules.job_name（由调度器写入）
--   * 异步任务：job_name = ''（由业务动作入队，按 type 归属）
-- 依赖字符串前缀做过滤属于"约定即契约"，改一次格式就断链，故补列并回填历史行。

ALTER TABLE asyncd_tasks ADD COLUMN job_name TEXT NOT NULL DEFAULT '';

-- 回填：'schedule:' 长 9 个字符，故从第 10 个字符起即作业名
UPDATE asyncd_tasks
SET job_name = substring(unique_key FROM 10)
WHERE job_name = '' AND unique_key LIKE 'schedule:%';

-- 任务中心「某任务的执行记录」按 (job_name, id DESC) 取页；异步任务仍走 (type, status, id DESC)
CREATE INDEX asyncd_tasks_job_idx ON asyncd_tasks (job_name, id DESC);
