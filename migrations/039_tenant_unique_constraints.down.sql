-- 039 回滚：移除补回的唯一约束（数据改名不可逆，去重删除的重复账号不恢复）
ALTER TABLE user_identities DROP CONSTRAINT IF EXISTS user_identities_inst_provider_key;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_provider_key UNIQUE (provider, provider_user_id);
ALTER TABLE app_accounts DROP CONSTRAINT IF EXISTS app_accounts_inst_app_user_key;
ALTER TABLE account_strategies DROP CONSTRAINT IF EXISTS account_strategies_inst_name_key;
ALTER TABLE connectors   DROP CONSTRAINT IF EXISTS connectors_inst_name_key;
ALTER TABLE departments  DROP CONSTRAINT IF EXISTS departments_inst_name_key;
ALTER TABLE syncers      DROP CONSTRAINT IF EXISTS syncers_inst_name_key;
ALTER TABLE certs        DROP CONSTRAINT IF EXISTS certs_inst_name_key;
ALTER TABLE job_titles   DROP CONSTRAINT IF EXISTS job_titles_inst_name_key;
ALTER TABLE permissions  DROP CONSTRAINT IF EXISTS permissions_inst_name_key;
ALTER TABLE roles        DROP CONSTRAINT IF EXISTS roles_inst_name_key;
ALTER TABLE providers    DROP CONSTRAINT IF EXISTS providers_inst_name_key;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_inst_code_key;
DROP INDEX IF EXISTS users_inst_email_key;
DROP INDEX IF EXISTS users_inst_phone_key;
ALTER TABLE users        DROP CONSTRAINT IF EXISTS users_inst_name_key;
