-- 041 回滚：令牌表恢复为业务键字符串
ALTER TABLE tokens ADD COLUMN application TEXT NOT NULL DEFAULT '';
ALTER TABLE tokens ADD COLUMN "user" TEXT NOT NULL DEFAULT '';
UPDATE tokens t SET application = a.code FROM applications a WHERE a.id = t.application_id;
UPDATE tokens t SET "user" = u.name FROM users u WHERE u.id = t.user_id;
DROP INDEX IF EXISTS tokens_app_user_idx;
ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_user_id_fkey;
ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_application_id_fkey;
ALTER TABLE tokens DROP COLUMN application_id;
ALTER TABLE tokens DROP COLUMN user_id;
