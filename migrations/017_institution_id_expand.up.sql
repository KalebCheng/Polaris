-- 017（expand 阶段）：把"租户列"从组织名切到组织主键 id —— 先扩，后收。
--
-- 背景（2026-10 复盘）：12 张业务表的租户列是 `institution TEXT REFERENCES institutions(name)`，
-- 即"按组织名做外键"。组织名一旦要改（重命名/多语言/大小写），所有引用都要级联跟着改；
-- 也无法用主键做稳定的机器标识。正确做法是 `institution_id BIGINT REFERENCES institutions(id)`。
--
-- 本迁移只做**结构 + 数据**（零行为变更）：
--   1) 每张表新增 institution_id，从 institutions.name 回填；
--   2) 加外键（ON DELETE CASCADE，与原先 name 外键一致）+ 索引 + NOT NULL；
--   3) 过渡期用触发器保持同步——代码仍在写 `institution`(name)，institution_id 由触发器跟随，
--      因此新列从此刻起始终正确。**下一阶段**把代码读写切到 institution_id 后，
--      再删除 `institution` 列与这些触发器（见 docs/specs 的迁移计划）。
--
-- 回滚：017_institution_id_expand.down.sql

-- ── 1. 新增列 + 回填 + 约束 ─────────────────────────────────────────────
DO $$
DECLARE
    t text;
    tables text[] := ARRAY[
        'applications', 'users', 'departments', 'groups', 'roles', 'permissions',
        'providers', 'certs', 'syncers', 'connectors', 'account_strategies', 'app_accounts'
    ];
BEGIN
    FOREACH t IN ARRAY tables LOOP
        EXECUTE format(
            'ALTER TABLE %I ADD COLUMN IF NOT EXISTS institution_id BIGINT
                 REFERENCES institutions(id) ON DELETE CASCADE', t);
        EXECUTE format(
            'UPDATE %I x SET institution_id = i.id FROM institutions i
                 WHERE i.name = x.institution AND x.institution_id IS NULL', t);
        EXECUTE format('ALTER TABLE %I ALTER COLUMN institution_id SET NOT NULL', t);
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (institution_id)',
                       t || '_institution_id_idx', t);
    END LOOP;
END $$;

-- ── 2. 过渡期同步触发器（代码仍写 institution 名字时，自动维护 institution_id）──
CREATE OR REPLACE FUNCTION polaris_sync_institution_id() RETURNS trigger AS $$
BEGIN
    IF NEW.institution IS DISTINCT FROM OLD.institution OR NEW.institution_id IS NULL THEN
        SELECT id INTO NEW.institution_id FROM institutions WHERE name = NEW.institution;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

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
        EXECUTE format(
            'CREATE TRIGGER %I BEFORE INSERT OR UPDATE ON %I
                 FOR EACH ROW EXECUTE FUNCTION polaris_sync_institution_id()',
            t || '_sync_institution_id', t);
    END LOOP;
END $$;
