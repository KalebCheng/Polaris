-- 用户认证数据归一化：
--   1) 密码迁至 user_passwords；
--   2) MFA 只保留 user_mfa_credentials；
--   3) 最近登录迁至 user_login_stats；
--   4) 删除在线快照列。
--
-- MFA 旧列无法安全映射到 user_mfa_credentials（旧数据没有 provider_id，且可能只有
-- 历史 secret）。若发现“只存在于 users.mfa_*、未落到新表”的绑定，直接中止迁移，
-- 由运维先完成重绑定或数据迁移，避免静默丢失第二因子。

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.users u
        WHERE (u.mfa_enabled
               OR u.mfa_secret <> ''
               OR cardinality(u.mfa_recovery_codes) > 0)
          AND NOT EXISTS (
              SELECT 1
              FROM public.user_mfa_credentials c
              WHERE c.user_id = u.id AND c.status = 'active'
          )
    ) THEN
        RAISE EXCEPTION 'legacy MFA enrollment exists without user_mfa_credentials; migrate or re-enroll before applying 003';
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS public.user_passwords (
    user_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    password_hash text NOT NULL,
    password_type text DEFAULT 'bcrypt'::text NOT NULL,
    password_salt text DEFAULT ''::text NOT NULL,
    password_updated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_passwords_pkey PRIMARY KEY (user_id),
    CONSTRAINT user_passwords_user_fk FOREIGN KEY (user_id, institution_id)
        REFERENCES public.users(id, institution_id) ON DELETE CASCADE
);

INSERT INTO public.user_passwords (
    user_id, institution_id, password_hash, password_type, password_salt,
    password_updated_at, created_at, updated_at
)
SELECT
    u.id, u.institution_id, u.password_hash,
    COALESCE(NULLIF(u.password_type, ''), 'bcrypt'),
    COALESCE(u.password_salt, ''),
    u.password_updated_at, u.created_at, u.updated_at
FROM public.users u
WHERE u.password_hash <> ''
ON CONFLICT (user_id) DO NOTHING;

ALTER TABLE public.password_history
    ADD COLUMN IF NOT EXISTS password_type text DEFAULT 'bcrypt'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS password_salt text DEFAULT ''::text NOT NULL;

UPDATE public.password_history h
SET institution_id = u.institution_id
FROM public.users u
WHERE h.user_id = u.id AND h.institution_id IS NULL;

-- 历史表没有算法/盐列，只能用用户当前快照回填；无法还原历史算法切换前的组合。
UPDATE public.password_history h
SET password_type = COALESCE(NULLIF(u.password_type, ''), 'bcrypt'),
    password_salt = COALESCE(u.password_salt, '')
FROM public.users u
WHERE h.user_id = u.id;

ALTER TABLE public.password_history ALTER COLUMN institution_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS public.user_login_stats (
    user_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    last_signin_at timestamp with time zone,
    last_signin_ip text DEFAULT ''::text NOT NULL,
    last_seen_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_login_stats_pkey PRIMARY KEY (user_id),
    CONSTRAINT user_login_stats_user_fk FOREIGN KEY (user_id, institution_id)
        REFERENCES public.users(id, institution_id) ON DELETE CASCADE
);

INSERT INTO public.user_login_stats (
    user_id, institution_id, last_signin_at, last_signin_ip, last_seen_at, updated_at
)
SELECT
    u.id, u.institution_id, u.last_signin_time, COALESCE(u.last_signin_ip, ''),
    u.last_signin_time, now()
FROM public.users u
WHERE u.last_signin_time IS NOT NULL OR COALESCE(u.last_signin_ip, '') <> ''
ON CONFLICT (user_id) DO NOTHING;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS password_salt,
    DROP COLUMN IF EXISTS password_type,
    DROP COLUMN IF EXISTS password_updated_at,
    DROP COLUMN IF EXISTS last_signin_time,
    DROP COLUMN IF EXISTS last_signin_ip,
    DROP COLUMN IF EXISTS is_online,
    DROP COLUMN IF EXISTS mfa_enabled,
    DROP COLUMN IF EXISTS mfa_type,
    DROP COLUMN IF EXISTS mfa_secret,
    DROP COLUMN IF EXISTS mfa_recovery_codes;
