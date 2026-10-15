-- 回滚：移除应用级登录失败限制字段（失败计数不可逆，Redis 计数键会自然过期）。
ALTER TABLE applications
    DROP CONSTRAINT IF EXISTS applications_signin_retry_wait_check,
    DROP CONSTRAINT IF EXISTS applications_signin_error_limit_check,
    DROP COLUMN signin_error_limit,
    DROP COLUMN signin_retry_wait_minutes;
