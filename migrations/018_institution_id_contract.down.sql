-- 018 down：恢复租户名称列（从 institutions 回填），供开发期回退
DO $$
DECLARE
    t text;
    tables text[] := ARRAY[
        'applications', 'users', 'departments', 'groups', 'roles', 'permissions',
        'providers', 'certs', 'syncers', 'connectors', 'account_strategies', 'app_accounts',
        'records', 'tokens', 'user_identities', 'password_history',
        'provision_history', 'syncer_runs'
    ];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS institution TEXT NOT NULL DEFAULT ''''', t);
        EXECUTE format('UPDATE %I x SET institution = COALESCE(i.name, '''') FROM institutions i WHERE i.id = x.institution_id', t);
    END LOOP;
END $$;
