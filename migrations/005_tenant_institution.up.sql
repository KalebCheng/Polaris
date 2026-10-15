-- 005: 引入租户层（institution），把 organizations 降级为部门树（departments）。
-- 依据：docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md §2.1 ADR（D1/D8）与 §3.1。
--
-- 一次性结构迁移：表名/列名一次到位，不留旧名残迹。
-- 可逆：见 005_tenant_institution.down.sql（无损回滚到迁移前结构）。
--
-- 归属维度映射：资源表 organization -> institution（外键指向 institutions(name)）；
-- 用户另增 department_id 指向部门树节点（本轮仅作用户归属，不参与权限）。
--
-- 执行顺序要点：资源表仍引用 organizations(name)（依赖 organizations_name_key 唯一约束），
-- 因此必须先完成"资源表外键改指向 institutions"，才能删除 organizations_name_key。

-- ── 1. 租户表 institutions ────────────────────────────────────
CREATE TABLE institutions (
    id                      BIGSERIAL   PRIMARY KEY,
    name                    TEXT        NOT NULL UNIQUE,
    display_name            TEXT        NOT NULL DEFAULT '',
    domain                  TEXT        NOT NULL DEFAULT '',
    website_url             TEXT        NOT NULL DEFAULT '',
    logo                    TEXT        NOT NULL DEFAULT '',
    favicon                 TEXT        NOT NULL DEFAULT '',
    description             TEXT        NOT NULL DEFAULT '',
    captcha                 TEXT        NOT NULL DEFAULT '',
    password_type           TEXT        NOT NULL DEFAULT 'bcrypt',
    password_options        TEXT[]      NOT NULL DEFAULT '{}',
    password_expire_days    INTEGER     NOT NULL DEFAULT 0,
    password_history_count  INTEGER     NOT NULL DEFAULT 0,
    token_retention_days    INTEGER     NOT NULL DEFAULT 30,
    record_retention_days   INTEGER     NOT NULL DEFAULT 30,
    default_application     TEXT        NOT NULL DEFAULT '',
    default_password        TEXT        NOT NULL DEFAULT '',
    languages               TEXT[]      NOT NULL DEFAULT '{}',
    ip_whitelist            TEXT        NOT NULL DEFAULT '',
    mfa_items               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    enable_soft_deletion    BOOLEAN     NOT NULL DEFAULT FALSE,
    is_profile_public       BOOLEAN     NOT NULL DEFAULT FALSE,
    use_email_as_username   BOOLEAN     NOT NULL DEFAULT FALSE,
    disable_signin          BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_exclusive_signin BOOLEAN     NOT NULL DEFAULT FALSE,
    max_sessions            INTEGER     NOT NULL DEFAULT 0,
    master_password_hash    TEXT        NOT NULL DEFAULT '',
    theme_data              JSONB,
    status                  TEXT        NOT NULL DEFAULT 'active',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 域名唯一（仅非空时约束），用于按 Host 解析租户（ADR D3）
CREATE UNIQUE INDEX institutions_domain_key ON institutions (lower(domain)) WHERE domain <> '';

-- 1:1 复制现有组织为租户：租户级属性（品牌/策略/保留设置）迁往 institutions
INSERT INTO institutions (
    name, display_name, website_url, logo, favicon, description,
    password_type, password_options, password_expire_days, password_history_count,
    token_retention_days, record_retention_days, default_application, default_password,
    languages, ip_whitelist, mfa_items, enable_soft_deletion, is_profile_public,
    use_email_as_username, disable_signin, enable_exclusive_signin, max_sessions,
    master_password_hash, theme_data, created_at, updated_at)
SELECT
    name, display_name, website_url, logo, favicon, description,
    password_type, password_options, password_expire_days, password_history_count,
    token_retention_days, record_retention_days, default_application, default_password,
    languages, ip_whitelist, mfa_items, enable_soft_deletion, is_profile_public,
    use_email_as_username, disable_signin, enable_exclusive_signin, max_sessions,
    master_password_hash, theme_data, created_at, updated_at
FROM organizations;

-- ── 2. organizations -> departments（降级为部门树）──────────────
-- 先改名并重命名主键；暂不删除 organizations_name_key（资源表外键仍依赖它）。
ALTER TABLE organizations RENAME TO departments;
ALTER TABLE departments RENAME CONSTRAINT organizations_pkey TO departments_pkey;

ALTER TABLE departments
    ADD COLUMN institution TEXT,
    ADD COLUMN parent_id   BIGINT,
    ADD COLUMN code_path   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN name_path   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN level       INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN has_child   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN sort_index  INTEGER NOT NULL DEFAULT 0;

-- 每个旧组织即其租户下的根部门（ADR D8）
UPDATE departments SET
    institution = name,
    code_path   = name,
    name_path   = COALESCE(NULLIF(display_name, ''), name);

-- ── 3. 资源表：organization -> institution ────────────────────
-- 3.1 带 organizations 外键的表：先删旧外键，改列名，再指向 institutions

-- users
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_organization_fkey;
ALTER TABLE users RENAME COLUMN organization TO institution;

-- applications
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_organization_fkey;
ALTER TABLE applications RENAME COLUMN organization TO institution;

-- providers
ALTER TABLE providers DROP CONSTRAINT IF EXISTS providers_organization_fkey;
ALTER TABLE providers RENAME COLUMN organization TO institution;

-- roles
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_organization_fkey;
ALTER TABLE roles RENAME COLUMN organization TO institution;

-- permissions
ALTER TABLE permissions DROP CONSTRAINT IF EXISTS permissions_organization_fkey;
ALTER TABLE permissions RENAME COLUMN organization TO institution;

-- groups
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_organization_fkey;
ALTER TABLE groups RENAME COLUMN organization TO institution;

-- certs
ALTER TABLE certs DROP CONSTRAINT IF EXISTS certs_organization_fkey;
ALTER TABLE certs RENAME COLUMN organization TO institution;

-- syncers
ALTER TABLE syncers DROP CONSTRAINT IF EXISTS syncers_organization_fkey;
ALTER TABLE syncers RENAME COLUMN organization TO institution;

-- 3.2 无外键的表：仅改列名
ALTER TABLE tokens            RENAME COLUMN organization TO institution;
ALTER TABLE records           RENAME COLUMN organization TO institution;
ALTER TABLE user_identities   RENAME COLUMN organization TO institution;
ALTER TABLE password_history  RENAME COLUMN organization TO institution;
ALTER TABLE syncer_runs       RENAME COLUMN organization TO institution;

-- 3.3 现在资源表已不再引用 organizations_name_key，可安全删除该唯一约束
ALTER TABLE departments DROP CONSTRAINT organizations_name_key;

-- 3.4 补 departments 的归属外键、唯一约束、索引与自引用
ALTER TABLE departments
    ALTER COLUMN institution SET NOT NULL,
    ADD CONSTRAINT departments_institution_fkey FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE,
    ADD CONSTRAINT departments_parent_fkey      FOREIGN KEY (parent_id)   REFERENCES departments(id) ON DELETE SET NULL,
    ADD CONSTRAINT departments_inst_name_key    UNIQUE (institution, name);
CREATE INDEX departments_inst_idx   ON departments (institution);
CREATE INDEX departments_parent_idx ON departments (parent_id);

-- 3.5 资源表补 institution 外键与索引，并重命名旧约束/索引
ALTER TABLE users        ADD CONSTRAINT users_institution_fkey        FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE applications ADD CONSTRAINT applications_institution_fkey FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE providers    ADD CONSTRAINT providers_institution_fkey    FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE roles        ADD CONSTRAINT roles_institution_fkey        FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE permissions  ADD CONSTRAINT permissions_institution_fkey  FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE groups       ADD CONSTRAINT groups_institution_fkey       FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE certs        ADD CONSTRAINT certs_institution_fkey        FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;
ALTER TABLE syncers      ADD CONSTRAINT syncers_institution_fkey      FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE;

ALTER TABLE users        RENAME CONSTRAINT users_org_name_key        TO users_inst_name_key;
ALTER TABLE applications RENAME CONSTRAINT applications_org_name_key TO applications_inst_name_key;
ALTER TABLE providers    RENAME CONSTRAINT providers_org_name_key    TO providers_inst_name_key;
ALTER TABLE roles        RENAME CONSTRAINT roles_org_name_key        TO roles_inst_name_key;
ALTER TABLE permissions  RENAME CONSTRAINT permissions_org_name_key  TO permissions_inst_name_key;
ALTER TABLE groups       RENAME CONSTRAINT groups_org_name_key       TO groups_inst_name_key;
ALTER TABLE certs        RENAME CONSTRAINT certs_org_name_key        TO certs_inst_name_key;
ALTER TABLE syncers      RENAME CONSTRAINT syncers_org_name_key      TO syncers_inst_name_key;

ALTER INDEX users_org_email_key RENAME TO users_inst_email_key;
ALTER INDEX users_org_idx       RENAME TO users_inst_idx;
ALTER INDEX users_org_phone_key RENAME TO users_inst_phone_key;
ALTER INDEX providers_category_idx RENAME TO providers_inst_category_idx;
ALTER INDEX tokens_org_user_idx          RENAME TO tokens_inst_user_idx;
ALTER INDEX records_org_idx              RENAME TO records_inst_idx;
ALTER INDEX user_identities_user_idx     RENAME TO user_identities_inst_user_idx;
ALTER INDEX password_history_user_idx    RENAME TO password_history_inst_user_idx;

-- ── 4. users.department_id（回填到所属租户的根部门）────────────
ALTER TABLE users ADD COLUMN department_id BIGINT REFERENCES departments(id) ON DELETE SET NULL;
UPDATE users u SET department_id = d.id
    FROM departments d
    WHERE d.institution = u.institution AND d.name = u.institution AND d.parent_id IS NULL;
CREATE INDEX users_department_idx ON users (department_id);

-- ── 5. departments 移除已迁往 institutions 的租户级列 ──────────
ALTER TABLE departments
    DROP COLUMN website_url,
    DROP COLUMN logo,
    DROP COLUMN favicon,
    DROP COLUMN password_type,
    DROP COLUMN password_options,
    DROP COLUMN password_expire_days,
    DROP COLUMN password_history_count,
    DROP COLUMN token_retention_days,
    DROP COLUMN record_retention_days,
    DROP COLUMN default_application,
    DROP COLUMN default_password,
    DROP COLUMN languages,
    DROP COLUMN ip_whitelist,
    DROP COLUMN mfa_items,
    DROP COLUMN enable_soft_deletion,
    DROP COLUMN is_profile_public,
    DROP COLUMN use_email_as_username,
    DROP COLUMN disable_signin,
    DROP COLUMN enable_exclusive_signin,
    DROP COLUMN max_sessions,
    DROP COLUMN master_password_hash,
    DROP COLUMN theme_data;
