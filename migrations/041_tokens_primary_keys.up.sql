-- 041：令牌表改为「主键关联」（Spec 第六阶段 Q5 = 全量主键化，第 1 批）
--
-- 背景（AGENTS.md §3.8）：`tokens.application` / `tokens.user` 存的是 `applications.code` / `users.name`
-- 这样的**业务键字符串**，没有任何引用完整性——应用改名/删除、用户改名都会让历史令牌"指向空气"。
-- 本迁移把它们换成主键 + 外键；**对外 API 契约不变**（读取时 JOIN 回名称下发，写入时服务层解析）。
ALTER TABLE tokens ADD COLUMN application_id BIGINT;
ALTER TABLE tokens ADD COLUMN user_id BIGINT;

-- 回填（按组织内的 code / name 解析；解析不到的留空，不阻断迁移）
UPDATE tokens t SET application_id = a.id
  FROM applications a
 WHERE a.institution_id = t.institution_id AND a.code = t.application AND t.application <> '';
UPDATE tokens t SET user_id = u.id
  FROM users u
 WHERE u.institution_id = t.institution_id AND u.name = t."user" AND t."user" <> '';

ALTER TABLE tokens DROP COLUMN application;
ALTER TABLE tokens DROP COLUMN "user";

ALTER TABLE tokens ADD CONSTRAINT tokens_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE SET NULL;
ALTER TABLE tokens ADD CONSTRAINT tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX tokens_app_user_idx ON tokens (institution_id, application_id, user_id);
