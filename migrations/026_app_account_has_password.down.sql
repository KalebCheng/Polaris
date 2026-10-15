ALTER TABLE app_accounts ADD COLUMN related_password TEXT NOT NULL DEFAULT '';
ALTER TABLE app_accounts DROP COLUMN has_password;
