-- 017 down：撤销 expand（删除触发器、函数与新列）
DO $$
DECLARE
    t text;
    tables text[] := ARRAY[
        'applications', 'users', 'departments', 'groups', 'roles', 'permissions',
        'providers', 'certs', 'syncers', 'connectors', 'account_strategies', 'app_accounts'
    ];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t || '_sync_institution_id', t);
        EXECUTE format('DROP INDEX IF EXISTS %I', t || '_institution_id_idx');
        EXECUTE format('ALTER TABLE %I DROP COLUMN IF EXISTS institution_id', t);
    END LOOP;
END $$;

DROP FUNCTION IF EXISTS polaris_sync_institution_id();
