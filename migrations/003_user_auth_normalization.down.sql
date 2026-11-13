-- 回滚认证数据归一化。注意：MFA 旧列只能恢复“是否绑定/类型”，
-- 旧 secret 已不再保存；凭证仍完整保留在 user_mfa_credentials。

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS password_hash text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS password_salt text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS password_type text DEFAULT 'bcrypt'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS password_updated_at timestamp with time zone,
    ADD COLUMN IF NOT EXISTS last_signin_time timestamp with time zone,
    ADD COLUMN IF NOT EXISTS last_signin_ip text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS is_online boolean DEFAULT false NOT NULL,
    ADD COLUMN IF NOT EXISTS mfa_enabled boolean DEFAULT false NOT NULL,
    ADD COLUMN IF NOT EXISTS mfa_type text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS mfa_secret text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS mfa_recovery_codes text[] DEFAULT '{}'::text[] NOT NULL;

UPDATE public.users u
SET password_hash = p.password_hash,
    password_type = p.password_type,
    password_salt = p.password_salt,
    password_updated_at = p.password_updated_at
FROM public.user_passwords p
WHERE p.user_id = u.id;

UPDATE public.users u
SET last_signin_time = s.last_signin_at,
    last_signin_ip = COALESCE(s.last_signin_ip, '')
FROM public.user_login_stats s
WHERE s.user_id = u.id;

UPDATE public.users u
SET mfa_enabled = EXISTS (
        SELECT 1 FROM public.user_mfa_credentials c
        WHERE c.user_id = u.id AND c.status = 'active'
    ),
    mfa_type = COALESCE((
        SELECT string_agg(DISTINCT c.type, ',' ORDER BY c.type)
        FROM public.user_mfa_credentials c
        WHERE c.user_id = u.id AND c.status = 'active'
    ), '');

DROP TABLE IF EXISTS public.user_login_stats;
DROP TABLE IF EXISTS public.user_passwords;

ALTER TABLE public.password_history ALTER COLUMN institution_id DROP NOT NULL;
ALTER TABLE public.password_history
    DROP COLUMN IF EXISTS password_type,
    DROP COLUMN IF EXISTS password_salt;
