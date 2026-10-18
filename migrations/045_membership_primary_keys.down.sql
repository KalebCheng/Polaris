DROP INDEX IF EXISTS password_history_user_idx;
ALTER TABLE password_history DROP CONSTRAINT IF EXISTS password_history_user_id_fkey;
ALTER TABLE password_history ADD COLUMN "user" TEXT NOT NULL DEFAULT '';
UPDATE password_history h SET "user" = u.name FROM users u WHERE u.id = h.user_id;
ALTER TABLE password_history DROP COLUMN user_id;

DROP INDEX IF EXISTS job_title_members_user_idx;
ALTER TABLE job_title_members DROP CONSTRAINT IF EXISTS job_title_members_user_id_fkey;
ALTER TABLE job_title_members DROP CONSTRAINT job_title_members_pkey;
ALTER TABLE job_title_members ADD COLUMN "user" TEXT NOT NULL DEFAULT '';
UPDATE job_title_members m SET "user" = u.name FROM users u WHERE u.id = m.user_id;
ALTER TABLE job_title_members DROP COLUMN user_id;
ALTER TABLE job_title_members ADD CONSTRAINT job_title_members_pkey PRIMARY KEY (job_title_id, "user");
