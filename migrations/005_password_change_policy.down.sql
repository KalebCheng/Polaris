DROP INDEX IF EXISTS public.password_history_active_key;

ALTER TABLE public.password_history
    DROP COLUMN IF EXISTS invalidated_at;

ALTER TABLE public.user_passwords
    DROP COLUMN IF EXISTS must_change_password;
