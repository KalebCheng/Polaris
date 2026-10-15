-- 018（contract）：租户列正式切到 institutions(id)。
-- 前置：017 已为 12 张业务表补上 institution_id 并回填；本步把**其余带 institution 的表**也切过来，
-- 然后统一删除 `institution`(name) 列与过渡期同步触发器——数据库此后只认主键。
-- 代码同步切换为读写 institution_id（同批提交）。

-- ── 1. 其余表：补 institution_id + 回填 + 约束 ─────────────────────────────
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
        EXECUTE format(
            'UPDATE %I x SET institution_id = i.id FROM institutions i
                 WHERE i.name = x.institution AND x.institution_id IS NULL', t);
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (institution_id)',
                       t || '_institution_id_idx', t);
    END LOOP;
END $$;

-- ── 2. 删除过渡期同步触发器与函数 ───────────────────────────────────────
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
    END LOOP;
END $$;
DROP FUNCTION IF EXISTS polaris_sync_institution_id();

-- ── 3. 删除名称列（外键约束随列一同消失）──────────────────────────────────
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
        EXECUTE format('ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I', t, t || '_institution_fkey');
        EXECUTE format('ALTER TABLE %I DROP COLUMN IF EXISTS institution', t);
    END LOOP;
END $$;
