-- 014 down：恢复三个外部跳转地址列（默认值与 002 迁移一致）
ALTER TABLE applications
    ADD COLUMN signin_url     TEXT NOT NULL DEFAULT '',
    ADD COLUMN forget_url     TEXT NOT NULL DEFAULT '',
    ADD COLUMN affiliation_url TEXT NOT NULL DEFAULT '';
