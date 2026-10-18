-- 042：表名去缩写——`app_accounts` → `application_accounts`（AGENTS.md §3.2「表名禁止缩写」）
--
-- 背景：本表是"本地用户 × 应用 → 下游账号"的台账（供给域）。`app_` 这类缩写在表名里一律不允许，
-- 一律使用完整单词（同类：`account_strategies` / `connectors`）。
-- 本迁移只做**重命名**（表 / 序列 / 主键 / 索引 / 外键约束名），不改列、不动数据；
-- 列层面的主键化（`application_id` / `user_id`）在后续批次单独进行。
ALTER TABLE app_accounts RENAME TO application_accounts;
ALTER SEQUENCE app_accounts_id_seq RENAME TO application_accounts_id_seq;
ALTER TABLE application_accounts RENAME CONSTRAINT app_accounts_pkey TO application_accounts_pkey;
ALTER TABLE application_accounts RENAME CONSTRAINT app_accounts_institution_id_fkey TO application_accounts_institution_id_fkey;
ALTER TABLE application_accounts RENAME CONSTRAINT app_accounts_inst_app_user_key TO application_accounts_inst_app_user_key;
ALTER INDEX app_accounts_institution_id_idx RENAME TO application_accounts_institution_id_idx;
