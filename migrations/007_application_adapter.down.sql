-- 007 回滚：移除应用适配器两列，还原到迁移前结构（无损）。
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_adapter_check;
ALTER TABLE applications DROP COLUMN IF EXISTS adapter_config;
ALTER TABLE applications DROP COLUMN IF EXISTS adapter;
