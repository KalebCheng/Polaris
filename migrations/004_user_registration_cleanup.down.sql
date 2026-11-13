ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS signup_application text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS register_type text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS register_source text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS created_ip text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS score integer DEFAULT 0 NOT NULL;

UPDATE public.users u
SET signup_application = r.signup_application_code,
    register_type = r.register_type,
    register_source = r.register_source,
    created_ip = r.created_ip
FROM public.user_registrations r
WHERE r.user_id = u.id;

DROP INDEX IF EXISTS public.users_job_level_idx;
CREATE INDEX users_job_level_idx
    ON public.users (institution_id, job_level_id);

CREATE INDEX IF NOT EXISTS users_email_idx
    ON public.users (email);
CREATE INDEX IF NOT EXISTS users_institution_id_idx
    ON public.users (institution_id);

DROP TABLE IF EXISTS public.user_registrations;
