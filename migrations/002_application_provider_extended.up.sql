-- 应用能力补全（对齐 Casdoor Application 的功能性字段）+ 提供商多类别的配置字段。
-- 说明：仅新增平台运行时会真实读取的字段；SAML / WebAuthn / 联合凭证 / 自定义 HTML-CSS 注入等
-- 未实现能力不留空列（避免"配置了但不生效"的死配置）。

ALTER TABLE applications
    ADD COLUMN logo_dark               TEXT    NOT NULL DEFAULT '',
    ADD COLUMN favicon                 TEXT    NOT NULL DEFAULT '',
    ADD COLUMN tags                    TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN enable_link_with_email  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN org_choice_mode         TEXT    NOT NULL DEFAULT '',
    ADD COLUMN ip_restriction          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN ip_whitelist            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN signup_url              TEXT    NOT NULL DEFAULT '',
    ADD COLUMN signin_url              TEXT    NOT NULL DEFAULT '',
    ADD COLUMN forget_url              TEXT    NOT NULL DEFAULT '',
    ADD COLUMN affiliation_url         TEXT    NOT NULL DEFAULT '',
    ADD COLUMN terms_of_use            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN forced_redirect_origin  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN backchannel_logout_uri  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN token_fields            TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN disable_refresh_rotation BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN signin_methods          JSONB   NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN signup_items            JSONB   NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN signin_items            JSONB   NOT NULL DEFAULT '[]'::jsonb;

-- 提供商（多类别）配置字段：
--   Email/OAuth/SMS 连接参数（host/port/client_id/... 已在 001 中）、
--   邮件正文与主题、短信签名与模板、对象存储端点与桶、验证码站点密钥等。
ALTER TABLE providers
    ADD COLUMN ssl_mode      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN from_address  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN title         TEXT    NOT NULL DEFAULT '',
    ADD COLUMN content       TEXT    NOT NULL DEFAULT '',
    ADD COLUMN receiver      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN region_id     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN sign_name     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN template_code TEXT    NOT NULL DEFAULT '',
    ADD COLUMN endpoint      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN domain        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN bucket        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN path_prefix   TEXT    NOT NULL DEFAULT '';

CREATE INDEX providers_category_idx ON providers (organization, category);

-- 用户：手机号与邮箱验证状态（验证码登录/绑定时写入）
ALTER TABLE users
    ADD COLUMN phone_verified BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX users_org_phone_key ON users (organization, phone) WHERE phone <> '';
