-- 013 down：回滚应用模块第二阶段的库结构变更。
-- 注意：撤销改名必须"先腾出 name 再占用 name"——先把 name 改回 display_name，再把 code 改回 name。

-- ── 1. 恢复 11 个旧列 ───────────────────────────────────────────────────
ALTER TABLE applications
    ADD COLUMN enable_sign_up         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN disable_self_signup    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_password        BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN enable_code_signin     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_guest_signin    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN disable_signin         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_auto_signin     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_signin_session  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_link_with_email BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN is_shared              BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN signin_items           JSONB   NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN default_tag            TEXT    NOT NULL DEFAULT '';

-- ── 2. 由 signin_methods / self_signup_enabled 反推旧布尔（有损，仅用于回滚）──
UPDATE applications
   SET enable_password     = signin_methods @> '[{"name":"Password"}]'::jsonb,
       enable_code_signin  = (signin_methods @> '[{"name":"EmailCode"}]'::jsonb)
                          OR (signin_methods @> '[{"name":"SMSCode"}]'::jsonb),
       enable_guest_signin = signin_methods @> '[{"name":"Guest"}]'::jsonb,
       enable_sign_up      = self_signup_enabled,
       disable_self_signup = FALSE,
       disable_signin      = (state <> 'active'),
       updated_at          = NOW();

-- ── 3. 撤销改名 ─────────────────────────────────────────────────────────
ALTER TABLE applications RENAME COLUMN name TO display_name;
ALTER TABLE applications RENAME COLUMN code TO name;
ALTER TABLE applications RENAME CONSTRAINT applications_inst_code_key TO applications_inst_name_key;

-- ── 4. 删除本阶段新增列 ─────────────────────────────────────────────────
ALTER TABLE applications
    DROP COLUMN IF EXISTS self_signup_enabled,
    DROP COLUMN IF EXISTS signup_auto_signin,
    DROP COLUMN IF EXISTS email_code_ttl_seconds,
    DROP COLUMN IF EXISTS sms_code_ttl_seconds,
    DROP COLUMN IF EXISTS magic_link_ttl_seconds;
