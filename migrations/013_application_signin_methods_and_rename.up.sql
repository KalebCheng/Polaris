-- 013: 应用模块第二阶段（登录方式重做 + 标识改名 + 相悖/死配置清除）
-- 依据：docs/specs/261013-application-module-phase2.md §5.4.1
--
-- 三件事，顺序不可颠倒：
--   1) 新增运行时"真正会读取"的字段（自助注册开关、注册后自动登录、各登录方式有效期）；
--   2) 用旧的相悖布尔字段回填 signin_methods / self_signup_enabled（保证存量应用语义不丢）；
--   3) 标识改名（name -> code、display_name -> name），并删除 11 个相悖/死配置列。
--
-- 回滚：013_application_signin_methods_and_rename.down.sql

-- ── 1. 新增列 ────────────────────────────────────────────────────────────
ALTER TABLE applications
    ADD COLUMN self_signup_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN signup_auto_signin     BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN email_code_ttl_seconds INTEGER NOT NULL DEFAULT 300,
    ADD COLUMN sms_code_ttl_seconds   INTEGER NOT NULL DEFAULT 300,
    ADD COLUMN magic_link_ttl_seconds INTEGER NOT NULL DEFAULT 600;

-- ── 2. 回填（必须在删除旧列之前执行）────────────────────────────────────

-- 2.1 登录方式：由旧布尔推导（仅当 signin_methods 为空，避免覆盖已配置数据）
UPDATE applications
   SET signin_methods = (
           SELECT COALESCE(jsonb_agg(m ORDER BY ord), '[]'::jsonb)
             FROM (
                   SELECT 1 AS ord, jsonb_build_object('name','Password','displayName','Password','rule','All') AS m WHERE enable_password
                   UNION ALL
                   SELECT 2, jsonb_build_object('name','EmailCode','displayName','Email code','rule','All') WHERE enable_code_signin
                   UNION ALL
                   SELECT 3, jsonb_build_object('name','Guest','displayName','Guest','rule','All') WHERE enable_guest_signin
                  ) t
       ),
       updated_at = NOW()
 WHERE signin_methods = '[]'::jsonb;

-- 2.2 第三方登录方式：providers 非空时补一条 Provider 方式
UPDATE applications
   SET signin_methods = signin_methods || '[{"name":"Provider","displayName":"Third-party","rule":"All"}]'::jsonb,
       updated_at = NOW()
 WHERE jsonb_array_length(providers) > 0
   AND NOT (signin_methods @> '[{"name":"Provider"}]'::jsonb);

-- 2.3 自助注册开关：旧语义 = enable_sign_up AND NOT disable_self_signup
UPDATE applications
   SET self_signup_enabled = (enable_sign_up AND NOT disable_self_signup),
       updated_at = NOW();

-- ── 3. 标识改名：name -> code（标识），display_name -> name（名称）─────
ALTER TABLE applications RENAME COLUMN name TO code;
ALTER TABLE applications RENAME COLUMN display_name TO name;
ALTER TABLE applications RENAME CONSTRAINT applications_inst_name_key TO applications_inst_code_key;

-- ── 4. 删除相悖 / 死配置列 ──────────────────────────────────────────────
ALTER TABLE applications
    DROP COLUMN IF EXISTS enable_sign_up,
    DROP COLUMN IF EXISTS disable_self_signup,
    DROP COLUMN IF EXISTS enable_password,
    DROP COLUMN IF EXISTS enable_code_signin,
    DROP COLUMN IF EXISTS enable_guest_signin,
    DROP COLUMN IF EXISTS disable_signin,
    DROP COLUMN IF EXISTS enable_auto_signin,
    DROP COLUMN IF EXISTS enable_signin_session,
    DROP COLUMN IF EXISTS enable_link_with_email,
    DROP COLUMN IF EXISTS is_shared,
    DROP COLUMN IF EXISTS signin_items,
    DROP COLUMN IF EXISTS default_tag;
