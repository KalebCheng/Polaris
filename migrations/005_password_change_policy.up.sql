-- Password history status and forced password changes:
--   1) exactly one active history row per user (invalidated_at IS NULL);
--   2) old rows are retained with invalidated_at set;
--   3) user_passwords.must_change_password drives a restricted post-login session.

ALTER TABLE public.user_passwords
    ADD COLUMN IF NOT EXISTS must_change_password boolean DEFAULT false NOT NULL;

ALTER TABLE public.password_history
    ADD COLUMN IF NOT EXISTS invalidated_at timestamp with time zone;

-- Existing history rows are historical by definition. The current password is
-- copied below as the single active row.
UPDATE public.password_history
SET invalidated_at = NOW()
WHERE invalidated_at IS NULL;

INSERT INTO public.password_history (
    institution_id, user_id, password_hash, password_type, password_salt,
    created_at, invalidated_at
)
SELECT
    p.institution_id,
    p.user_id,
    p.password_hash,
    p.password_type,
    p.password_salt,
    COALESCE(p.password_updated_at, p.created_at, NOW()),
    NULL
FROM public.user_passwords p
WHERE NOT EXISTS (
    SELECT 1 FROM public.password_history h
    WHERE h.user_id = p.user_id AND h.invalidated_at IS NULL
);

DELETE FROM public.password_history h
WHERE NOT EXISTS (
    SELECT 1 FROM public.user_passwords p WHERE p.user_id = h.user_id
);

-- Keep the active row plus at most (password_history_count - 1) invalidated rows.
-- A policy of 0 keeps only the active row; set the organization policy to N>0
-- before upgrading if historical hashes must be retained.
WITH ranked AS (
    SELECT
        h.id,
        i.password_history_count,
        row_number() OVER (
            PARTITION BY h.user_id
            ORDER BY (h.invalidated_at IS NULL) DESC, h.created_at DESC, h.id DESC
        ) AS rn
    FROM public.password_history h
    JOIN public.institutions i ON i.id = h.institution_id
)
DELETE FROM public.password_history h
USING ranked r
WHERE h.id = r.id
  AND r.rn > GREATEST(r.password_history_count, 1);

CREATE UNIQUE INDEX IF NOT EXISTS password_history_active_key
    ON public.password_history (user_id)
    WHERE invalidated_at IS NULL;
