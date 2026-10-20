-- ============================================================================
-- sensitive storage hardening：token / client_secret / default_password / cert
--
-- 单向升级：旧 token、client_secret、default_password 和证书私钥会全部丢弃，
-- 不提供 down 脚本恢复（旧明文凭据不可安全恢复）。
--
-- 前提：
--   * 执行前必须停止旧实例，否则旧代码会继续读写本脚本删除的明文列；
--   * 执行前必须备份数据库；
--   * 升级后用户需要重新登录，应用 client_secret / 组织默认密码需要重新设置，
--     内置签名证书会在启动时重新生成并以 AES-GCM 密文落库。
-- ============================================================================

BEGIN;

-- ① 旧访问令牌与授权码全部作废
TRUNCATE TABLE public.tokens;

-- ② 组织默认密码：只保留 AES-GCM 密文字段
ALTER TABLE public.institutions
    DROP COLUMN IF EXISTS default_password;
ALTER TABLE public.institutions
    ADD COLUMN IF NOT EXISTS default_password_enc text DEFAULT ''::text NOT NULL;

-- ③ 应用 client_secret：只保留哈希字段
ALTER TABLE public.applications
    DROP COLUMN IF EXISTS client_secret;
ALTER TABLE public.applications
    ADD COLUMN IF NOT EXISTS client_secret_hash text DEFAULT ''::text NOT NULL;

-- ④ 授权码 / access / refresh / id token：删除明文字段，补齐 hash 字段
ALTER TABLE public.tokens
    DROP COLUMN IF EXISTS code,
    DROP COLUMN IF EXISTS access_token,
    DROP COLUMN IF EXISTS refresh_token,
    DROP COLUMN IF EXISTS id_token;

ALTER TABLE public.tokens
    ADD COLUMN IF NOT EXISTS code_hash text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS access_token_hash text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS refresh_token_hash text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS id_token_hash text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS code_nonce text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS session_id text DEFAULT ''::text NOT NULL;

DROP INDEX IF EXISTS public.tokens_code_idx;
CREATE UNIQUE INDEX IF NOT EXISTS tokens_code_hash_key
    ON public.tokens USING btree (code_hash)
    WHERE code_hash <> ''::text;

-- ⑤ 旧证书私钥全部作废；应用启动时会重新生成加密私钥
UPDATE public.applications SET cert_id = NULL WHERE cert_id IS NOT NULL;
DELETE FROM public.certs;

COMMIT;
