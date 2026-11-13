-- 注册来源归一化 + 索引/死字段清理。
-- 历史展示值保留快照（application code / 连接名 / 文件名），实时关联使用主键。

CREATE TABLE IF NOT EXISTS public.user_registrations (
    user_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    signup_application_id bigint,
    signup_application_code text DEFAULT ''::text NOT NULL,
    register_type text DEFAULT ''::text NOT NULL,
    register_source text DEFAULT ''::text NOT NULL,
    registration_syncer_id bigint,
    created_ip text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_registrations_pkey PRIMARY KEY (user_id),
    CONSTRAINT user_registrations_user_fk FOREIGN KEY (user_id, institution_id)
        REFERENCES public.users(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT user_registrations_application_fk FOREIGN KEY (signup_application_id)
        REFERENCES public.applications(id) ON DELETE SET NULL,
    CONSTRAINT user_registrations_syncer_fk FOREIGN KEY (registration_syncer_id)
        REFERENCES public.syncers(id) ON DELETE SET NULL
);

INSERT INTO public.user_registrations (
    user_id, institution_id, signup_application_id, signup_application_code,
    register_type, register_source, registration_syncer_id, created_ip,
    created_at, updated_at
)
SELECT
    u.id,
    u.institution_id,
    a.id,
    COALESCE(u.signup_application, ''),
    COALESCE(u.register_type, ''),
    COALESCE(u.register_source, ''),
    s.id,
    COALESCE(u.created_ip, ''),
    u.created_at,
    u.updated_at
FROM public.users u
LEFT JOIN public.applications a
    ON a.institution_id = u.institution_id
   AND a.code = u.signup_application
   AND u.signup_application <> ''
LEFT JOIN public.syncers s
    ON s.institution_id = u.institution_id
   AND s.name = u.register_source
   AND u.register_type = 'syncer'
WHERE COALESCE(u.signup_application, '') <> ''
   OR COALESCE(u.register_type, '') <> ''
   OR COALESCE(u.register_source, '') <> ''
   OR COALESCE(u.created_ip, '') <> ''
ON CONFLICT (user_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS user_registrations_inst_syncer_idx
    ON public.user_registrations (institution_id, registration_syncer_id)
    WHERE registration_syncer_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS user_registrations_inst_application_idx
    ON public.user_registrations (institution_id, signup_application_id)
    WHERE signup_application_id IS NOT NULL;

ALTER TABLE public.users
    DROP COLUMN IF EXISTS signup_application,
    DROP COLUMN IF EXISTS register_type,
    DROP COLUMN IF EXISTS register_source,
    DROP COLUMN IF EXISTS created_ip,
    DROP COLUMN IF EXISTS score;

DROP INDEX IF EXISTS public.users_email_idx;
DROP INDEX IF EXISTS public.users_institution_id_idx;

DROP INDEX IF EXISTS public.users_job_level_idx;
CREATE INDEX users_job_level_idx
    ON public.users (institution_id, job_level_id)
    WHERE job_level_id IS NOT NULL;
