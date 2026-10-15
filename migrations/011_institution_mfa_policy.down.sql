-- 回滚：移除组织级 MFA 策略字段。
ALTER TABLE institutions
    DROP CONSTRAINT IF EXISTS institutions_mfa_remember_check,
    DROP CONSTRAINT IF EXISTS institutions_mfa_policy_check,
    DROP COLUMN mfa_remember_hours,
    DROP COLUMN mfa_policy;
