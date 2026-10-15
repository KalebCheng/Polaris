-- 020：回填审计记录的「结果」状态码。
-- 背景：业务审计此前从不写 status_code，列表「结果」列恒为空；现由 AuditRecorder 统一写入
-- （成功 200、失败类动作如 auth.login_failed 记 401）。本迁移把历史行回填成同一口径。
UPDATE records SET status_code = 401 WHERE status_code = 0 AND action LIKE '%\_failed' ESCAPE '\';
UPDATE records SET status_code = 200 WHERE status_code = 0;
