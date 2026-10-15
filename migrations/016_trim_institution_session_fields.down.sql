-- 016 down：恢复组织级会话策略字段（默认值与 001/005 一致）
ALTER TABLE institutions
    ADD COLUMN enable_exclusive_signin BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN max_sessions           INTEGER NOT NULL DEFAULT 0;
