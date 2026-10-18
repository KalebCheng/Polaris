-- 043 回滚：恢复业务键字符串列
DROP INDEX IF EXISTS provision_history_user_idx;
DROP INDEX IF EXISTS application_accounts_user_idx;
ALTER TABLE application_accounts DROP CONSTRAINT IF EXISTS application_accounts_inst_app_user_key;

ALTER TABLE connectors           ADD COLUMN application TEXT NOT NULL DEFAULT '';
ALTER TABLE account_strategies   ADD COLUMN application TEXT NOT NULL DEFAULT '';
ALTER TABLE application_accounts ADD COLUMN application TEXT NOT NULL DEFAULT '';
ALTER TABLE application_accounts ADD COLUMN "user" TEXT NOT NULL DEFAULT '';
ALTER TABLE provision_history    ADD COLUMN application TEXT NOT NULL DEFAULT '';
ALTER TABLE provision_history    ADD COLUMN "user" TEXT NOT NULL DEFAULT '';

UPDATE connectors c SET application = COALESCE(a.code, '') FROM applications a WHERE a.id = c.application_id;
UPDATE account_strategies s SET application = COALESCE(a.code, '') FROM applications a WHERE a.id = s.application_id;
UPDATE application_accounts x SET application = COALESCE(a.code, '') FROM applications a WHERE a.id = x.application_id;
UPDATE application_accounts x SET "user" = COALESCE(u.name, '') FROM users u WHERE u.id = x.user_id;
UPDATE provision_history h SET application = COALESCE(a.code, '') FROM applications a WHERE a.id = h.application_id;
UPDATE provision_history h SET "user" = COALESCE(u.name, '') FROM users u WHERE u.id = h.user_id;

DROP INDEX IF EXISTS account_strategies_application_idx;
DROP INDEX IF EXISTS connectors_application_idx;
ALTER TABLE provision_history    DROP CONSTRAINT IF EXISTS provision_history_user_id_fkey;
ALTER TABLE provision_history    DROP CONSTRAINT IF EXISTS provision_history_application_id_fkey;
ALTER TABLE application_accounts DROP CONSTRAINT IF EXISTS application_accounts_user_id_fkey;
ALTER TABLE application_accounts DROP CONSTRAINT IF EXISTS application_accounts_application_id_fkey;
ALTER TABLE account_strategies   DROP CONSTRAINT IF EXISTS account_strategies_application_id_fkey;
ALTER TABLE connectors           DROP CONSTRAINT IF EXISTS connectors_application_id_fkey;

ALTER TABLE provision_history    DROP COLUMN user_id;
ALTER TABLE provision_history    DROP COLUMN application_id;
ALTER TABLE application_accounts DROP COLUMN user_id;
ALTER TABLE application_accounts DROP COLUMN application_id;
ALTER TABLE account_strategies   DROP COLUMN application_id;
ALTER TABLE connectors           DROP COLUMN application_id;

ALTER TABLE application_accounts ADD CONSTRAINT application_accounts_inst_app_user_key
    UNIQUE (institution_id, application, "user");
