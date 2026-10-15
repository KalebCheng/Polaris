-- 006 回滚：删除供给相关的四张表（connectors / account_strategies / app_accounts / provision_history）。
-- 目标：完全还原到应用 006 之前的结构（新表整体删除，不涉及既有表，故无损）。
-- 顺序：先删依赖表，再删被依赖表（此处均无相互外键，仅按逻辑顺序删除）。

DROP TABLE IF EXISTS provision_history;
DROP TABLE IF EXISTS app_accounts;
DROP TABLE IF EXISTS account_strategies;
DROP TABLE IF EXISTS connectors;
