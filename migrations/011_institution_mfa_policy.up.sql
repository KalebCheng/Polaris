-- 011: 组织级 MFA（二次验证）策略。
-- optional（默认）：不强制，用户自愿绑定（绑定后登录仍需校验）。
-- required：组织强制——已绑定用户登录必须通过二次校验；未绑定用户登录后进入「待绑定」状态，
--           在完成绑定前被会话门禁限制（见 internal/auth/middleware.go）。
-- mfa_remember_hours：勾选「记住此设备」后的免二次校验时长（小时）；0 = 不记住。
ALTER TABLE institutions
    ADD COLUMN mfa_policy         TEXT    NOT NULL DEFAULT 'optional',
    ADD COLUMN mfa_remember_hours INTEGER NOT NULL DEFAULT 0;

ALTER TABLE institutions
    ADD CONSTRAINT institutions_mfa_policy_check CHECK (mfa_policy IN ('optional', 'required')),
    ADD CONSTRAINT institutions_mfa_remember_check CHECK (mfa_remember_hours >= 0);
