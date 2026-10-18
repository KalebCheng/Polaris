-- 046：角色/权限的成员关联改主键（Spec 第六阶段 Q5 第 5 批第 2 段）
--
-- `role_users."user"` / `permission_users."user"` 用账号名做关联：用户改名即断链、删用户留孤儿行。
-- 改为 `user_id` + 外键（ON DELETE CASCADE）；对外契约不变（读取 JOIN 回名称，写入在 SQL 内解析）。
ALTER TABLE role_users       ADD COLUMN user_id BIGINT;
ALTER TABLE permission_users ADD COLUMN user_id BIGINT;

UPDATE role_users ru SET user_id = u.id
  FROM users u, roles r
 WHERE r.id = ru.role_id AND u.institution_id = r.institution_id AND u.name = ru."user";
UPDATE permission_users pu SET user_id = u.id
  FROM users u, permissions p
 WHERE p.id = pu.permission_id AND u.institution_id = p.institution_id AND u.name = pu."user";

DELETE FROM role_users       WHERE user_id IS NULL;
DELETE FROM permission_users WHERE user_id IS NULL;

ALTER TABLE role_users       DROP CONSTRAINT role_users_pkey;
ALTER TABLE permission_users DROP CONSTRAINT permission_users_pkey;
ALTER TABLE role_users       DROP COLUMN "user";
ALTER TABLE permission_users DROP COLUMN "user";
ALTER TABLE role_users       ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE permission_users ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE role_users       ADD CONSTRAINT role_users_pkey PRIMARY KEY (role_id, user_id);
ALTER TABLE permission_users ADD CONSTRAINT permission_users_pkey PRIMARY KEY (permission_id, user_id);
ALTER TABLE role_users ADD CONSTRAINT role_users_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE permission_users ADD CONSTRAINT permission_users_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX role_users_user_idx ON role_users (user_id);
CREATE INDEX permission_users_user_idx ON permission_users (user_id);
