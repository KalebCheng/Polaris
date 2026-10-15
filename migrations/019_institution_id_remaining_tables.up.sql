-- 019：补齐 018 未覆盖的 6 张表（records/tokens/user_identities/password_history/provision_history/syncer_runs）。
-- 背景：018 在开发库上先以"仅 12 张租户表"的版本被应用并记录，导致这 6 张表仍只有 institution(name)。
-- 本迁移幂等：补 institution_id、回填、清列——使任何库的终态一致（数据库只认主键）。
DO $$
DECLARE
    t text;
    tables text[] := ARRAY[
        'records', 'tokens', 'user_identities', 'password_history',
        'provision_history', 'syncer_runs'
    ];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS institution_id BIGINT', t);
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = t AND column_name = 'institution'
        ) THEN
            EXECUTE format(
                'UPDATE %I x SET institution_id = i.id FROM institutions i
                     WHERE i.name = x.institution AND x.institution_id IS NULL', t);
            EXECUTE format('ALTER TABLE %I DROP COLUMN institution', t);
        END IF;
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (institution_id)',
                       t || '_institution_id_idx', t);
    END LOOP;
END $$;
