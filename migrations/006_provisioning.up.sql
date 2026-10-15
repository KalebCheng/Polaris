-- 006: 应用账号供给与生命周期（P2）。
-- 依据：docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md §3.3。
--
-- 新增四张表：
--   connectors        下游应用连接器（推送型供给；RESTful principal/credentials）
--   account_strategies 账号策略（何时建号 / 何时回收 + 本地→下游属性映射）
--   app_accounts      应用账号（本地用户 × 应用 → 下游账号；幂等键 (institution, application, user)）
--   provision_history 供给历史（建号/改密/禁用/删除/重试的审计轨迹）
--
-- 多租户隔离：均以 institution 为归属维度并对 institutions(name) 建外键（ON DELETE CASCADE）。
-- 敏感字段（下游口令、连接器凭证）由应用层 AES-GCM 加密后落库（列存密文）。
-- 可逆：见 006_provisioning.down.sql（新表整体删除，无损回滚到迁移前结构）。

-- ── 1. 下游应用连接器 connectors ──────────────────────────────
CREATE TABLE connectors (
    id            BIGSERIAL   PRIMARY KEY,
    institution   TEXT        NOT NULL,
    application   TEXT        NOT NULL DEFAULT '',  -- 关联应用名（applications.name，可空表示通用连接器）
    name          TEXT        NOT NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    provider_url  TEXT        NOT NULL DEFAULT '',  -- 下游供给 API 基地址
    principal     TEXT        NOT NULL DEFAULT '',  -- 调用主体（如 API 账号）
    credentials   TEXT        NOT NULL DEFAULT '',  -- 敏感，AES-GCM 加密存储
    filters       JSONB       NOT NULL DEFAULT '[]'::jsonb, -- 供给范围过滤规则
    just_in_time  BOOLEAN     NOT NULL DEFAULT FALSE, -- 是否在首次登录时即时建号
    schedule      TEXT        NOT NULL DEFAULT '',  -- 空 = 不自动；否则为间隔秒数（与 syncer 一致）
    state         TEXT        NOT NULL DEFAULT 'active', -- active | disabled
    last_run_time TIMESTAMPTZ,
    last_status   TEXT        NOT NULL DEFAULT 'idle',   -- idle | success | failed
    last_message  TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT connectors_institution_fkey FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE,
    CONSTRAINT connectors_inst_name_key    UNIQUE (institution, name)
);
CREATE INDEX connectors_inst_idx            ON connectors (institution);
CREATE INDEX connectors_inst_application_idx ON connectors (institution, application);

-- ── 2. 账号策略 account_strategies ────────────────────────────
CREATE TABLE account_strategies (
    id           BIGSERIAL   PRIMARY KEY,
    institution  TEXT        NOT NULL,
    application  TEXT        NOT NULL DEFAULT '',  -- 关联应用名（空 = 租户内默认策略）
    name         TEXT        NOT NULL,
    display_name TEXT        NOT NULL DEFAULT '',
    is_enabled   BOOLEAN     NOT NULL DEFAULT TRUE,
    mapping      JSONB       NOT NULL DEFAULT '{}'::jsonb,  -- 本地属性 → 下游属性
    policy       JSONB       NOT NULL DEFAULT '{}'::jsonb,  -- 建号/回收触发策略
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_strategies_institution_fkey FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE,
    CONSTRAINT account_strategies_inst_name_key    UNIQUE (institution, name)
);
CREATE INDEX account_strategies_inst_idx             ON account_strategies (institution);
CREATE INDEX account_strategies_inst_application_idx ON account_strategies (institution, application);

-- ── 3. 应用账号 app_accounts ──────────────────────────────────
CREATE TABLE app_accounts (
    id               BIGSERIAL   PRIMARY KEY,
    institution      TEXT        NOT NULL,
    application      TEXT        NOT NULL,
    "user"           TEXT        NOT NULL,
    username         TEXT        NOT NULL DEFAULT '',  -- 本地用户名（= users.name）
    related_username TEXT        NOT NULL DEFAULT '',  -- 下游应用中的账号名
    related_password TEXT        NOT NULL DEFAULT '',  -- 敏感，AES-GCM 加密存储
    create_type      TEXT        NOT NULL DEFAULT 'automatic', -- automatic | manual
    status           TEXT        NOT NULL DEFAULT 'active',    -- active | disabled | pending | failed
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT app_accounts_institution_fkey FOREIGN KEY (institution) REFERENCES institutions(name) ON DELETE CASCADE,
    CONSTRAINT app_accounts_inst_app_user_key UNIQUE (institution, application, "user")
);
CREATE INDEX app_accounts_inst_idx            ON app_accounts (institution);
CREATE INDEX app_accounts_inst_user_idx        ON app_accounts (institution, "user");
CREATE INDEX app_accounts_inst_application_idx ON app_accounts (institution, application);

-- ── 4. 供给历史 provision_history ─────────────────────────────
CREATE TABLE provision_history (
    id          BIGSERIAL   PRIMARY KEY,
    institution TEXT        NOT NULL,
    connector   TEXT        NOT NULL DEFAULT '',  -- 执行供给的连接器名
    application TEXT        NOT NULL DEFAULT '',
    "user"      TEXT        NOT NULL DEFAULT '',  -- 目标本地用户
    action      TEXT        NOT NULL,             -- create | update | disable | delete | retry
    status      TEXT        NOT NULL,             -- success | failed | pending
    message     TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX provision_history_inst_idx          ON provision_history (institution);
CREATE INDEX provision_history_inst_connector_idx ON provision_history (institution, connector);
CREATE INDEX provision_history_inst_user_idx      ON provision_history (institution, "user");
CREATE INDEX provision_history_created_idx        ON provision_history (created_at DESC);
