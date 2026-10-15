-- 回滚 028：只删索引。
-- 不恢复被停用的行——"回滚即启用两个同类提供商"会让运行时选择重新变得不确定。
DROP INDEX IF EXISTS providers_singleton_active_key;
