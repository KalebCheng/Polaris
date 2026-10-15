-- 回滚：恢复被精简的组织字段（原值已丢失，回滚为默认值）。
ALTER TABLE institutions
    ADD COLUMN languages             TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN ip_whitelist          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN mfa_items             JSONB   NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN enable_soft_deletion  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN is_profile_public     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN master_password_hash  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN theme_data            JSONB;
