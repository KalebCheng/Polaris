-- 023: 应用级登录失败限制（对齐 Casdoor「安全设置」的两项）：
--   signin_error_limit        登入错误次数限制（单位：次）
--   signin_retry_wait_minutes 登入重试等待时间（单位：分钟）
--
-- 语义：0 = 沿用平台默认（auth.rate_limit.login_max_attempts / login_window_seconds），
--       非 0 = 覆盖平台默认值。
-- 运行时读取点（配置即生效）：AuthService.Login → signinFailurePolicy / countFailure
--   —— 达到次数上限时，把「组织 + 应用 + 账号 + IP」的失败计数窗口重置为等待时间，
--   自达到上限的那一刻起冻结，冻结期内该组合的登录一律 429。
-- 回滚：023_application_signin_failure_limit.down.sql

ALTER TABLE applications
    ADD COLUMN signin_error_limit        INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN signin_retry_wait_minutes INTEGER NOT NULL DEFAULT 0;

ALTER TABLE applications
    ADD CONSTRAINT applications_signin_error_limit_check CHECK (signin_error_limit >= 0),
    ADD CONSTRAINT applications_signin_retry_wait_check CHECK (signin_retry_wait_minutes >= 0);
