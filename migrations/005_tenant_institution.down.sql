-- 005 回滚：institution -> organization，departments -> organizations，删除 institutions。
-- 目标：完全还原到应用 005 之前的结构且不丢数据（ADR D7）。
-- 顺序关键：资源表外键回指 organizations(name) 之前，organizations 表名与唯一约束必须已就绪。

-- ── 1. departments 先补回租户级列并从 institutions 回填 ────────
ALTER TABLE departments
    ADD COLUMN website_url            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN logo                   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN favicon                TEXT    NOT NULL DEFAULT '',
    ADD COLUMN password_type          TEXT    NOT NULL DEFAULT 'bcrypt',
    ADD COLUMN password_options       TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN password_expire_days   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN password_history_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN token_retention_days   INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN record_retention_days  INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN default_application    TEXT    NOT NULL DEFAULT '',
    ADD COLUMN default_password       TEXT    NOT NULL DEFAULT '',
    ADD COLUMN languages              TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN ip_whitelist           TEXT    NOT NULL DEFAULT '',
    ADD COLUMN mfa_items              JSONB   NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN enable_soft_deletion   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN is_profile_public      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN use_email_as_username  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN disable_signin         BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN enable_exclusive_signin BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN max_sessions           INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN master_password_hash   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN theme_data             JSONB;

UPDATE departments d SET
    website_url             = i.website_url,
    logo                    = i.logo,
    favicon                 = i.favicon,
    password_type           = i.password_type,
    password_options        = i.password_options,
    password_expire_days    = i.password_expire_days,
    password_history_count  = i.password_history_count,
    token_retention_days    = i.token_retention_days,
    record_retention_days   = i.record_retention_days,
    default_application     = i.default_application,
    default_password        = i.default_password,
    languages               = i.languages,
    ip_whitelist            = i.ip_whitelist,
    mfa_items               = i.mfa_items,
    enable_soft_deletion    = i.enable_soft_deletion,
    is_profile_public       = i.is_profile_public,
    use_email_as_username   = i.use_email_as_username,
    disable_signin          = i.disable_signin,
    enable_exclusive_signin = i.enable_exclusive_signin,
    max_sessions            = i.max_sessions,
    master_password_hash    = i.master_password_hash,
    theme_data              = i.theme_data
FROM institutions i
WHERE d.institution = i.name;

-- ── 2. 删除 005 新增的部门（非"根部门"），并去掉 users.department_id ──
ALTER TABLE users DROP COLUMN IF EXISTS department_id;
DROP INDEX IF EXISTS users_department_idx;

DELETE FROM departments
WHERE NOT (name = institution AND parent_id IS NULL);

-- ── 3. 重命名约束/索引：inst -> org（仅命名；外键目标稍后统一改回）──
ALTER TABLE users        RENAME CONSTRAINT users_inst_name_key        TO users_org_name_key;
ALTER TABLE applications RENAME CONSTRAINT applications_inst_name_key TO applications_org_name_key;
ALTER TABLE providers    RENAME CONSTRAINT providers_inst_name_key    TO providers_org_name_key;
ALTER TABLE roles        RENAME CONSTRAINT roles_inst_name_key        TO roles_org_name_key;
ALTER TABLE permissions  RENAME CONSTRAINT permissions_inst_name_key  TO permissions_org_name_key;
ALTER TABLE groups       RENAME CONSTRAINT groups_inst_name_key       TO groups_org_name_key;
ALTER TABLE certs        RENAME CONSTRAINT certs_inst_name_key        TO certs_org_name_key;
ALTER TABLE syncers      RENAME CONSTRAINT syncers_inst_name_key      TO syncers_org_name_key;

ALTER INDEX users_inst_email_key      RENAME TO users_org_email_key;
ALTER INDEX users_inst_idx            RENAME TO users_org_idx;
ALTER INDEX users_inst_phone_key      RENAME TO users_org_phone_key;
ALTER INDEX providers_inst_category_idx RENAME TO providers_category_idx;
ALTER INDEX tokens_inst_user_idx          RENAME TO tokens_org_user_idx;
ALTER INDEX records_inst_idx              RENAME TO records_org_idx;
ALTER INDEX user_identities_inst_user_idx RENAME TO user_identities_user_idx;
ALTER INDEX password_history_inst_user_idx RENAME TO password_history_user_idx;

-- ── 4. 资源表：去掉 institution 外键，列改回 organization ────────
ALTER TABLE users        DROP CONSTRAINT IF EXISTS users_institution_fkey;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_institution_fkey;
ALTER TABLE providers    DROP CONSTRAINT IF EXISTS providers_institution_fkey;
ALTER TABLE roles        DROP CONSTRAINT IF EXISTS roles_institution_fkey;
ALTER TABLE permissions  DROP CONSTRAINT IF EXISTS permissions_institution_fkey;
ALTER TABLE groups       DROP CONSTRAINT IF EXISTS groups_institution_fkey;
ALTER TABLE certs        DROP CONSTRAINT IF EXISTS certs_institution_fkey;
ALTER TABLE syncers      DROP CONSTRAINT IF EXISTS syncers_institution_fkey;

ALTER TABLE users        RENAME COLUMN institution TO organization;
ALTER TABLE applications RENAME COLUMN institution TO organization;
ALTER TABLE providers    RENAME COLUMN institution TO organization;
ALTER TABLE roles        RENAME COLUMN institution TO organization;
ALTER TABLE permissions  RENAME COLUMN institution TO organization;
ALTER TABLE groups       RENAME COLUMN institution TO organization;
ALTER TABLE certs        RENAME COLUMN institution TO organization;
ALTER TABLE syncers      RENAME COLUMN institution TO organization;

ALTER TABLE tokens           RENAME COLUMN institution TO organization;
ALTER TABLE records          RENAME COLUMN institution TO organization;
ALTER TABLE user_identities  RENAME COLUMN institution TO organization;
ALTER TABLE password_history RENAME COLUMN institution TO organization;
ALTER TABLE syncer_runs      RENAME COLUMN institution TO organization;

-- ── 5. departments 去掉部门树列与约束，改名回 organizations ──────
ALTER TABLE departments
    DROP CONSTRAINT IF EXISTS departments_parent_fkey,
    DROP CONSTRAINT IF EXISTS departments_institution_fkey,
    DROP CONSTRAINT IF EXISTS departments_inst_name_key,
    DROP COLUMN institution,
    DROP COLUMN parent_id,
    DROP COLUMN code_path,
    DROP COLUMN name_path,
    DROP COLUMN level,
    DROP COLUMN has_child,
    DROP COLUMN sort_index;

DROP INDEX IF EXISTS departments_inst_idx;
DROP INDEX IF EXISTS departments_parent_idx;

ALTER TABLE departments RENAME TO organizations;
ALTER TABLE organizations RENAME CONSTRAINT departments_pkey TO organizations_pkey;
ALTER TABLE organizations ADD CONSTRAINT organizations_name_key UNIQUE (name);

-- ── 6. 资源表外键回指 organizations(name) ───────────────────────
ALTER TABLE users        ADD CONSTRAINT users_organization_fkey        FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE applications ADD CONSTRAINT applications_organization_fkey FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE providers    ADD CONSTRAINT providers_organization_fkey    FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE roles        ADD CONSTRAINT roles_organization_fkey        FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE permissions  ADD CONSTRAINT permissions_organization_fkey  FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE groups       ADD CONSTRAINT groups_organization_fkey       FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE certs        ADD CONSTRAINT certs_organization_fkey        FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;
ALTER TABLE syncers      ADD CONSTRAINT syncers_organization_fkey      FOREIGN KEY (organization) REFERENCES organizations(name) ON DELETE CASCADE;

-- ── 7. 删除 institutions ─────────────────────────────────────
DROP INDEX IF EXISTS institutions_domain_key;
DROP TABLE institutions;
