-- 045：成员/历史关联改主键（Spec 第六阶段 Q5 第 5 批，第 1 段）
--
-- 覆盖：`job_title_members`（职务任职人员）与 `password_history`（密码历史）。
-- 二者都用 `users.name` 做关联字符串：用户改名即断链、删用户留孤儿行（无外键，级联清理无从谈起）。
-- 本迁移换成 `user_id` + 外键；**对外契约不变**（读取 JOIN 回名称，写入在 SQL 内解析名称→主键）。
ALTER TABLE job_title_members ADD COLUMN user_id BIGINT;
ALTER TABLE password_history  ADD COLUMN user_id BIGINT;

UPDATE job_title_members m SET user_id = u.id
  FROM users u, job_titles t
 WHERE t.id = m.job_title_id AND u.institution_id = t.institution_id AND u.name = m."user";
UPDATE password_history h SET user_id = u.id
  FROM users u
 WHERE u.institution_id = h.institution_id AND u.name = h."user";

DELETE FROM job_title_members WHERE user_id IS NULL;      -- 解析不到的历史脏行（用户已删除）
DELETE FROM password_history  WHERE user_id IS NULL;

ALTER TABLE job_title_members DROP CONSTRAINT job_title_members_pkey;
ALTER TABLE job_title_members DROP COLUMN "user";
ALTER TABLE job_title_members ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE job_title_members ADD CONSTRAINT job_title_members_pkey PRIMARY KEY (job_title_id, user_id);
ALTER TABLE job_title_members ADD CONSTRAINT job_title_members_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX job_title_members_user_idx ON job_title_members (user_id);

ALTER TABLE password_history DROP COLUMN "user";
ALTER TABLE password_history ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE password_history ADD CONSTRAINT password_history_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX password_history_user_idx ON password_history (user_id, created_at DESC);
