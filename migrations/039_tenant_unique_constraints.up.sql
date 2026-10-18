-- 039：补回被迁移 018/019 连带删除的「租户内唯一」约束（Spec 第六阶段 FR-2 / Q1）
--
-- 事故复盘：018 用 `ALTER TABLE ... DROP COLUMN institution` 删列时，PostgreSQL 把**依赖该列的约束/索引**
-- 一并删除（users/applications/providers/roles/permissions/groups/certs/syncers/departments/connectors/
-- account_strategies/app_accounts 的租户内唯一键），此后再无迁移补回 → 全平台的"组织内同名"静默失效
-- （实测：钉钉同步一次产生 324 个用户、同一人 6 行；部门出现多个同名「市场部」/「财务部」）。
--
-- 本迁移：① 先做存量去重（不破坏数据：命名冲突改名为 `<name>-<id>`，重复用户删保留最早的一条）；
-- ② 再补回全部唯一约束。真实库上必须能空跑通过（幂等：IF NOT EXISTS / 先 DROP 再 ADD）。

-- ── 1. 存量去重 ────────────────────────────────────────────────
-- 1.1 重复账号：同名（同组织）只保留最早创建的一条（同步重复的产物）
DELETE FROM users a USING users b
 WHERE a.institution_id = b.institution_id AND a.name = b.name AND a.id > b.id;

-- 1.2 命名冲突：改名为 `<name>-<id>`（保留行与其全部关联，不删业务数据）
UPDATE departments d SET name = d.name || '-' || d.id
 WHERE EXISTS (SELECT 1 FROM departments p
                WHERE p.institution_id = d.institution_id AND p.name = d.name AND p.id < d.id);

-- 1.3 部门路径按改名结果重算（code_path / name_path / level 是派生列）
WITH RECURSIVE tree AS (
    SELECT id, parent_id, name, display_name, name AS cpath, display_name AS npath, 0 AS lvl
      FROM departments WHERE parent_id IS NULL
    UNION ALL
    SELECT d.id, d.parent_id, d.name, d.display_name,
           t.cpath || '/' || d.name, t.npath || '/' || d.display_name, t.lvl + 1
      FROM departments d JOIN tree t ON d.parent_id = t.id
)
UPDATE departments d SET code_path = t.cpath, name_path = t.npath, level = t.lvl FROM tree t WHERE t.id = d.id;

UPDATE job_titles g SET name = g.name || '-' || g.id
 WHERE EXISTS (SELECT 1 FROM job_titles p
                WHERE p.institution_id = g.institution_id AND p.name = g.name AND p.id < g.id);
UPDATE roles r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM roles p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE permissions r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM permissions p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE providers r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM providers p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE certs r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM certs p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE syncers r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM syncers p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE connectors r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM connectors p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE account_strategies r SET name = r.name || '-' || r.id
 WHERE EXISTS (SELECT 1 FROM account_strategies p WHERE p.institution_id = r.institution_id AND p.name = r.name AND p.id < r.id);
UPDATE applications a SET code = a.code || '-' || a.id
 WHERE EXISTS (SELECT 1 FROM applications p
                WHERE p.institution_id = a.institution_id AND p.code = a.code AND p.id < a.id);

DELETE FROM app_accounts a USING app_accounts b
 WHERE a.institution_id = b.institution_id AND a.application = b.application AND a."user" = b."user" AND a.id > b.id;

ALTER TABLE user_identities DROP CONSTRAINT IF EXISTS user_identities_provider_key;
DELETE FROM user_identities a USING user_identities b
 WHERE a.institution_id = b.institution_id AND a.provider = b.provider
   AND a.provider_user_id = b.provider_user_id AND a.id > b.id;

-- ── 2. 补回租户内唯一约束 ─────────────────────────────────────
ALTER TABLE users        ADD CONSTRAINT users_inst_name_key      UNIQUE (institution_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS users_inst_email_key ON users (institution_id, lower(email)) WHERE email <> '';
CREATE UNIQUE INDEX IF NOT EXISTS users_inst_phone_key ON users (institution_id, phone)        WHERE phone <> '';
ALTER TABLE applications ADD CONSTRAINT applications_inst_code_key UNIQUE (institution_id, code);
ALTER TABLE providers    ADD CONSTRAINT providers_inst_name_key  UNIQUE (institution_id, name);
ALTER TABLE roles        ADD CONSTRAINT roles_inst_name_key      UNIQUE (institution_id, name);
ALTER TABLE permissions  ADD CONSTRAINT permissions_inst_name_key UNIQUE (institution_id, name);
ALTER TABLE job_titles   ADD CONSTRAINT job_titles_inst_name_key UNIQUE (institution_id, name);
ALTER TABLE certs        ADD CONSTRAINT certs_inst_name_key      UNIQUE (institution_id, name);
ALTER TABLE syncers      ADD CONSTRAINT syncers_inst_name_key    UNIQUE (institution_id, name);
ALTER TABLE departments  ADD CONSTRAINT departments_inst_name_key UNIQUE (institution_id, name);
ALTER TABLE connectors   ADD CONSTRAINT connectors_inst_name_key UNIQUE (institution_id, name);
ALTER TABLE account_strategies ADD CONSTRAINT account_strategies_inst_name_key UNIQUE (institution_id, name);
ALTER TABLE app_accounts ADD CONSTRAINT app_accounts_inst_app_user_key UNIQUE (institution_id, application, "user");
ALTER TABLE user_identities ADD CONSTRAINT user_identities_inst_provider_key
    UNIQUE (institution_id, provider, provider_user_id);
