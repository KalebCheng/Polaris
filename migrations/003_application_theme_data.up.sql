-- 应用级品牌主题（此前遗漏；002 已发布不再改动，按"迁移不可变"原则新增本迁移）。
ALTER TABLE applications ADD COLUMN theme_data JSONB;
