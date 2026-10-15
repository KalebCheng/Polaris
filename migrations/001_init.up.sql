-- Polaris IAM 平台初始表结构
-- 说明：组织（organization）是多租户命名空间，也是 Casdoor 中 owner 概念的落地。

CREATE TABLE organizations (
    id                      BIGSERIAL PRIMARY KEY,
    name                    TEXT        NOT NULL UNIQUE,
    display_name            TEXT        NOT NULL DEFAULT '',
    website_url             TEXT        NOT NULL DEFAULT '',
    logo                    TEXT        NOT NULL DEFAULT '',
    favicon                 TEXT        NOT NULL DEFAULT '',
    description             TEXT        NOT NULL DEFAULT '',
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
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id                   BIGSERIAL PRIMARY KEY,
    uid                  TEXT        NOT NULL DEFAULT '',
    organization         TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name                 TEXT        NOT NULL,
    type                 TEXT        NOT NULL DEFAULT 'normal',
    password_hash        TEXT        NOT NULL DEFAULT '',
    password_salt        TEXT        NOT NULL DEFAULT '',
    password_type        TEXT        NOT NULL DEFAULT 'bcrypt',
    display_name         TEXT        NOT NULL DEFAULT '',
    first_name           TEXT        NOT NULL DEFAULT '',
    last_name            TEXT        NOT NULL DEFAULT '',
    avatar               TEXT        NOT NULL DEFAULT '',
    email                TEXT        NOT NULL DEFAULT '',
    email_verified       BOOLEAN     NOT NULL DEFAULT FALSE,
    phone                TEXT        NOT NULL DEFAULT '',
    country_code         TEXT        NOT NULL DEFAULT '',
    location             TEXT        NOT NULL DEFAULT '',
    address              TEXT        NOT NULL DEFAULT '',
    affiliation          TEXT        NOT NULL DEFAULT '',
    title                TEXT        NOT NULL DEFAULT '',
    homepage             TEXT        NOT NULL DEFAULT '',
    bio                  TEXT        NOT NULL DEFAULT '',
    language             TEXT        NOT NULL DEFAULT '',
    gender               TEXT        NOT NULL DEFAULT '',
    birthday             TEXT        NOT NULL DEFAULT '',
    education            TEXT        NOT NULL DEFAULT '',
    score                INTEGER     NOT NULL DEFAULT 0,
    is_admin             BOOLEAN     NOT NULL DEFAULT FALSE,
    is_forbidden         BOOLEAN     NOT NULL DEFAULT FALSE,
    is_deleted           BOOLEAN     NOT NULL DEFAULT FALSE,
    is_online            BOOLEAN     NOT NULL DEFAULT FALSE,
    signup_application   TEXT        NOT NULL DEFAULT '',
    register_type        TEXT        NOT NULL DEFAULT '',
    register_source      TEXT        NOT NULL DEFAULT '',
    created_ip           TEXT        NOT NULL DEFAULT '',
    last_signin_time     TIMESTAMPTZ,
    last_signin_ip       TEXT        NOT NULL DEFAULT '',
    password_updated_at  TIMESTAMPTZ,
    mfa_enabled          BOOLEAN     NOT NULL DEFAULT FALSE,
    mfa_type             TEXT        NOT NULL DEFAULT '',
    mfa_secret           TEXT        NOT NULL DEFAULT '',
    mfa_recovery_codes   TEXT[]      NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_org_name_key UNIQUE (organization, name)
);

CREATE UNIQUE INDEX users_uid_key ON users (uid) WHERE uid <> '';
CREATE UNIQUE INDEX users_org_email_key ON users (organization, lower(email)) WHERE email <> '';
CREATE INDEX users_email_idx ON users (email);
CREATE INDEX users_org_idx ON users (organization);

CREATE TABLE user_identities (
    id               BIGSERIAL PRIMARY KEY,
    organization     TEXT        NOT NULL,
    "user"           TEXT        NOT NULL,
    provider         TEXT        NOT NULL,
    provider_user_id TEXT        NOT NULL,
    email            TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_identities_provider_key UNIQUE (provider, provider_user_id)
);

CREATE INDEX user_identities_user_idx ON user_identities (organization, "user");

CREATE TABLE password_history (
    id            BIGSERIAL PRIMARY KEY,
    organization  TEXT        NOT NULL,
    "user"        TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX password_history_user_idx ON password_history (organization, "user", created_at DESC);

CREATE TABLE applications (
    id                        BIGSERIAL PRIMARY KEY,
    organization              TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name                      TEXT        NOT NULL,
    display_name              TEXT        NOT NULL DEFAULT '',
    category                  TEXT        NOT NULL DEFAULT 'default',
    type                      TEXT        NOT NULL DEFAULT 'oidc',
    logo                      TEXT        NOT NULL DEFAULT '',
    title                     TEXT        NOT NULL DEFAULT '',
    description               TEXT        NOT NULL DEFAULT '',
    homepage_url              TEXT        NOT NULL DEFAULT '',
    sort_order                INTEGER     NOT NULL DEFAULT 0,
    cert                      TEXT        NOT NULL DEFAULT '',
    default_group             TEXT        NOT NULL DEFAULT '',
    default_tag               TEXT        NOT NULL DEFAULT '',
    enable_password           BOOLEAN     NOT NULL DEFAULT TRUE,
    enable_sign_up            BOOLEAN     NOT NULL DEFAULT FALSE,
    disable_self_signup       BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_guest_signin       BOOLEAN     NOT NULL DEFAULT FALSE,
    disable_signin            BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_signin_session     BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_auto_signin        BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_code_signin        BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_exclusive_signin   BOOLEAN     NOT NULL DEFAULT FALSE,
    max_sessions              INTEGER     NOT NULL DEFAULT 0,
    grant_types               TEXT[]      NOT NULL DEFAULT '{authorization_code,password,refresh_token,client_credentials}',
    redirect_uris             TEXT[]      NOT NULL DEFAULT '{}',
    providers                 JSONB       NOT NULL DEFAULT '[]'::jsonb,
    scopes                    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    client_id                 TEXT        NOT NULL DEFAULT '',
    client_secret             TEXT        NOT NULL DEFAULT '',
    token_format              TEXT        NOT NULL DEFAULT 'jwt',
    token_signing_method      TEXT        NOT NULL DEFAULT 'RS256',
    expire_in_hours           DOUBLE PRECISION NOT NULL DEFAULT 1,
    refresh_expire_in_hours   DOUBLE PRECISION NOT NULL DEFAULT 24,
    cookie_expire_in_hours    BIGINT      NOT NULL DEFAULT 720,
    is_shared                 BOOLEAN     NOT NULL DEFAULT FALSE,
    state                     TEXT        NOT NULL DEFAULT 'active',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT applications_org_name_key UNIQUE (organization, name)
);

CREATE UNIQUE INDEX applications_client_id_key ON applications (client_id) WHERE client_id <> '';

CREATE TABLE providers (
    id                    BIGSERIAL PRIMARY KEY,
    organization          TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name                  TEXT        NOT NULL,
    display_name          TEXT        NOT NULL DEFAULT '',
    category              TEXT        NOT NULL DEFAULT 'OAuth',
    type                  TEXT        NOT NULL DEFAULT 'oidc',
    sub_type              TEXT        NOT NULL DEFAULT '',
    method                TEXT        NOT NULL DEFAULT '',
    client_id             TEXT        NOT NULL DEFAULT '',
    client_secret         TEXT        NOT NULL DEFAULT '',
    client_id2            TEXT        NOT NULL DEFAULT '',
    client_secret2        TEXT        NOT NULL DEFAULT '',
    cert                  TEXT        NOT NULL DEFAULT '',
    custom_auth_url       TEXT        NOT NULL DEFAULT '',
    custom_token_url      TEXT        NOT NULL DEFAULT '',
    custom_user_info_url  TEXT        NOT NULL DEFAULT '',
    custom_logout_url     TEXT        NOT NULL DEFAULT '',
    issuer_url            TEXT        NOT NULL DEFAULT '',
    scopes                TEXT[]      NOT NULL DEFAULT '{}',
    user_mapping          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    http_headers          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    host                  TEXT        NOT NULL DEFAULT '',
    port                  INTEGER     NOT NULL DEFAULT 0,
    email_regex           TEXT        NOT NULL DEFAULT '',
    enable_proxy          BOOLEAN     NOT NULL DEFAULT FALSE,
    enable_pkce           BOOLEAN     NOT NULL DEFAULT FALSE,
    state                 TEXT        NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT providers_org_name_key UNIQUE (organization, name)
);

CREATE TABLE roles (
    id           BIGSERIAL PRIMARY KEY,
    organization TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    display_name TEXT        NOT NULL DEFAULT '',
    description  TEXT        NOT NULL DEFAULT '',
    is_enabled   BOOLEAN     NOT NULL DEFAULT TRUE,
    is_system    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roles_org_name_key UNIQUE (organization, name)
);

CREATE TABLE role_users (
    role_id  BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    "user"   TEXT   NOT NULL,
    PRIMARY KEY (role_id, "user")
);

CREATE TABLE role_groups (
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    "group" TEXT   NOT NULL,
    PRIMARY KEY (role_id, "group")
);

CREATE TABLE role_roles (
    role_id       BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    child_role    TEXT   NOT NULL,
    PRIMARY KEY (role_id, child_role)
);

CREATE TABLE role_permissions (
    role_id    BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT   NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE permissions (
    id            BIGSERIAL PRIMARY KEY,
    organization  TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    description   TEXT        NOT NULL DEFAULT '',
    resource_type TEXT        NOT NULL DEFAULT '',
    resources     TEXT[]      NOT NULL DEFAULT '{}',
    actions       TEXT[]      NOT NULL DEFAULT '{}',
    effect        TEXT        NOT NULL DEFAULT 'Allow',
    is_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    expire_time   TIMESTAMPTZ,
    submitter     TEXT        NOT NULL DEFAULT '',
    approver      TEXT        NOT NULL DEFAULT '',
    approve_time  TIMESTAMPTZ,
    state         TEXT        NOT NULL DEFAULT 'Approved',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT permissions_org_name_key UNIQUE (organization, name)
);

CREATE TABLE permission_users (
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    "user"        TEXT   NOT NULL,
    PRIMARY KEY (permission_id, "user")
);

CREATE TABLE permission_groups (
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    "group"       TEXT   NOT NULL,
    PRIMARY KEY (permission_id, "group")
);

CREATE TABLE permission_roles (
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    role          TEXT   NOT NULL,
    PRIMARY KEY (permission_id, role)
);

CREATE TABLE groups (
    id            BIGSERIAL PRIMARY KEY,
    organization  TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    manager       TEXT        NOT NULL DEFAULT '',
    contact_email TEXT        NOT NULL DEFAULT '',
    type          TEXT        NOT NULL DEFAULT '',
    parent_id     BIGINT      REFERENCES groups(id) ON DELETE SET NULL,
    is_top_group  BOOLEAN     NOT NULL DEFAULT FALSE,
    is_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    properties    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT groups_org_name_key UNIQUE (organization, name)
);

CREATE TABLE group_members (
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    "user"   TEXT   NOT NULL,
    PRIMARY KEY (group_id, "user")
);

CREATE INDEX group_members_user_idx ON group_members ("user");

CREATE TABLE certs (
    id               BIGSERIAL PRIMARY KEY,
    organization     TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name             TEXT        NOT NULL,
    display_name     TEXT        NOT NULL DEFAULT '',
    scope            TEXT        NOT NULL DEFAULT 'JWT',
    type             TEXT        NOT NULL DEFAULT 'x509',
    crypto_algorithm TEXT        NOT NULL DEFAULT 'RSA',
    bit_size         INTEGER     NOT NULL DEFAULT 2048,
    expire_in_years  INTEGER     NOT NULL DEFAULT 10,
    certificate      TEXT        NOT NULL DEFAULT '',
    private_key      TEXT        NOT NULL DEFAULT '',
    expire_time      TIMESTAMPTZ,
    is_system        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT certs_org_name_key UNIQUE (organization, name)
);

CREATE TABLE tokens (
    id                   BIGSERIAL PRIMARY KEY,
    organization         TEXT        NOT NULL,
    application          TEXT        NOT NULL DEFAULT '',
    "user"               TEXT        NOT NULL DEFAULT '',
    name                 TEXT        NOT NULL,
    code                 TEXT        NOT NULL DEFAULT '',
    code_challenge       TEXT        NOT NULL DEFAULT '',
    code_challenge_method TEXT       NOT NULL DEFAULT '',
    code_is_used         BOOLEAN     NOT NULL DEFAULT FALSE,
    code_expire_in       BIGINT      NOT NULL DEFAULT 0,
    code_scope           TEXT        NOT NULL DEFAULT '',
    code_redirect_uri    TEXT        NOT NULL DEFAULT '',
    access_token         TEXT        NOT NULL DEFAULT '',
    refresh_token        TEXT        NOT NULL DEFAULT '',
    id_token             TEXT        NOT NULL DEFAULT '',
    access_token_hash    TEXT        NOT NULL DEFAULT '',
    refresh_token_hash   TEXT        NOT NULL DEFAULT '',
    id_token_hash        TEXT        NOT NULL DEFAULT '',
    expires_in           INTEGER     NOT NULL DEFAULT 0,
    scope                TEXT        NOT NULL DEFAULT '',
    token_type           TEXT        NOT NULL DEFAULT 'Bearer',
    grant_type           TEXT        NOT NULL DEFAULT '',
    resource             TEXT        NOT NULL DEFAULT '',
    session_id           TEXT        NOT NULL DEFAULT '',
    revoked              BOOLEAN     NOT NULL DEFAULT FALSE,
    expire_time          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX tokens_code_idx ON tokens (code) WHERE code <> '';
CREATE UNIQUE INDEX tokens_access_hash_key ON tokens (access_token_hash) WHERE access_token_hash <> '';
CREATE UNIQUE INDEX tokens_refresh_hash_key ON tokens (refresh_token_hash) WHERE refresh_token_hash <> '';
CREATE INDEX tokens_org_user_idx ON tokens (organization, "user");

CREATE TABLE records (
    id           BIGSERIAL PRIMARY KEY,
    organization TEXT        NOT NULL DEFAULT '',
    name         TEXT        NOT NULL DEFAULT '',
    client_ip    TEXT        NOT NULL DEFAULT '',
    "user"       TEXT        NOT NULL DEFAULT '',
    method       TEXT        NOT NULL DEFAULT '',
    request_uri  TEXT        NOT NULL DEFAULT '',
    action       TEXT        NOT NULL DEFAULT '',
    language     TEXT        NOT NULL DEFAULT '',
    object       TEXT        NOT NULL DEFAULT '',
    response     TEXT        NOT NULL DEFAULT '',
    status_code  INTEGER     NOT NULL DEFAULT 0,
    detail       TEXT        NOT NULL DEFAULT '',
    is_triggered BOOLEAN     NOT NULL DEFAULT FALSE,
    trace_id     TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX records_created_idx ON records (created_at DESC);
CREATE INDEX records_org_idx ON records (organization, created_at DESC);

CREATE TABLE syncers (
    id            BIGSERIAL PRIMARY KEY,
    organization  TEXT        NOT NULL REFERENCES organizations(name) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    type          TEXT        NOT NULL,
    sub_type      TEXT        NOT NULL DEFAULT '',
    is_enabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    schedule      TEXT        NOT NULL DEFAULT '',
    host          TEXT        NOT NULL DEFAULT '',
    port          INTEGER     NOT NULL DEFAULT 0,
    "user"        TEXT        NOT NULL DEFAULT '',
    password      TEXT        NOT NULL DEFAULT '',
    database      TEXT        NOT NULL DEFAULT '',
    "table"       TEXT        NOT NULL DEFAULT '',
    database_type TEXT        NOT NULL DEFAULT '',
    ssl_mode      TEXT        NOT NULL DEFAULT '',
    app_id        TEXT        NOT NULL DEFAULT '',
    app_secret    TEXT        NOT NULL DEFAULT '',
    agent_id      TEXT        NOT NULL DEFAULT '',
    corp_id       TEXT        NOT NULL DEFAULT '',
    user_mapping  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    group_mapping JSONB       NOT NULL DEFAULT '{}'::jsonb,
    sync_users    BOOLEAN     NOT NULL DEFAULT TRUE,
    sync_groups   BOOLEAN     NOT NULL DEFAULT FALSE,
    soft_delete   BOOLEAN     NOT NULL DEFAULT FALSE,
    last_run_time TIMESTAMPTZ,
    last_status   TEXT        NOT NULL DEFAULT 'idle',
    last_message  TEXT        NOT NULL DEFAULT '',
    last_counts   JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT syncers_org_name_key UNIQUE (organization, name)
);

CREATE TABLE syncer_runs (
    id           BIGSERIAL PRIMARY KEY,
    syncer_id    BIGINT      NOT NULL REFERENCES syncers(id) ON DELETE CASCADE,
    organization TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT '',
    message      TEXT        NOT NULL DEFAULT '',
    counts       JSONB,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX syncer_runs_syncer_idx ON syncer_runs (syncer_id, started_at DESC);
