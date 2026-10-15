-- 回滚 027（仅供开发环境；被删除列的历史值可从 provider_dropped_columns_backup 取回）
ALTER TABLE providers
    ADD COLUMN IF NOT EXISTS method             TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cert               TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_id2         TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_secret2     TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS email_regex        TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS enable_proxy       BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS http_headers       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS user_mapping       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS custom_logout_url  TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sub_type           TEXT        NOT NULL DEFAULT '';

ALTER TABLE providers
    DROP COLUMN IF EXISTS auto_create,
    DROP COLUMN IF EXISTS link_by_email,
    DROP COLUMN IF EXISTS allowed_domains,
    DROP COLUMN IF EXISTS redirect_uri;

DROP INDEX IF EXISTS providers_institution_category_idx;
DROP TABLE IF EXISTS provider_dropped_columns_backup;
