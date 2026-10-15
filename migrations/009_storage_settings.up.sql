-- 009: 系统级默认对象存储配置（单行）。
-- 目的：新增「平台默认存储」，组织未单独配置 Storage 提供商时回落使用（见 docs/specs/261013-org-module-phase1.md §5）。
-- secret_key 以密文落库（SecretCipher 加密），接口不回传明文（write-only）。
CREATE TABLE storage_settings (
    id             SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    type           TEXT        NOT NULL DEFAULT '',  -- '' 未配置 | local | s3 | minio
    endpoint       TEXT        NOT NULL DEFAULT '',
    region         TEXT        NOT NULL DEFAULT '',
    bucket         TEXT        NOT NULL DEFAULT '',
    access_key     TEXT        NOT NULL DEFAULT '',
    secret_key_enc TEXT        NOT NULL DEFAULT '',
    path_prefix    TEXT        NOT NULL DEFAULT '',
    domain         TEXT        NOT NULL DEFAULT '',
    root_path      TEXT        NOT NULL DEFAULT '',
    public_base    TEXT        NOT NULL DEFAULT '',
    path_style     BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 保证单行配置始终存在
INSERT INTO storage_settings (id) VALUES (1);
