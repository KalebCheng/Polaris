-- 019 down：恢复 6 张表的 institution 名称列
DO $$
DECLARE
    t text;
    tables text[] := ARRAY[
        'records', 'tokens', 'user_identities', 'password_history',
        'provision_history', 'syncer_runs'
    ];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS institution TEXT NOT NULL DEFAULT ''''', t);
        EXECUTE format('UPDATE %I x SET institution = COALESCE(i.name, '''') FROM institutions i WHERE i.id = x.institution_id', t);
    END LOOP;
END $$;
