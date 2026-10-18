-- 043：供给域 4 张表的列主键化（Spec 第六阶段 Q5 第 3 批）
--
-- 背景（AGENTS.md §3.8）：这四张表用 `applications.code` / `users.name` 这样的**业务键字符串**做关联，
-- 零引用完整性——账号策略可以配到不存在的应用、应用删除后留下孤儿行（实测事故见 Spec §2.2）。
-- 本迁移把关联列换成主键 + 外键；**对外 API 契约不变**（读取时 JOIN 回名称下发，写入由服务层解析）。
--
-- 语义保持：
--   connectors.application_id IS NULL        → 通用连接器（原 application = ''）
--   account_strategies.application_id IS NULL → 租户默认策略（原 application = ''）

ALTER TABLE connectors          ADD COLUMN application_id BIGINT;
ALTER TABLE account_strategies  ADD COLUMN application_id BIGINT;
ALTER TABLE application_accounts ADD COLUMN application_id BIGINT;
ALTER TABLE application_accounts ADD COLUMN user_id BIGINT;
ALTER TABLE provision_history   ADD COLUMN application_id BIGINT;
ALTER TABLE provision_history   ADD COLUMN user_id BIGINT;

UPDATE connectors c SET application_id = a.id
  FROM applications a WHERE a.institution_id = c.institution_id AND a.code = c.application AND c.application <> '';
UPDATE account_strategies s SET application_id = a.id
  FROM applications a WHERE a.institution_id = s.institution_id AND a.code = s.application AND s.application <> '';
UPDATE application_accounts x SET application_id = a.id
  FROM applications a WHERE a.institution_id = x.institution_id AND a.code = x.application;
UPDATE application_accounts x SET user_id = u.id
  FROM users u WHERE u.institution_id = x.institution_id AND u.name = x."user";
UPDATE provision_history h SET application_id = a.id
  FROM applications a WHERE a.institution_id = h.institution_id AND a.code = h.application AND h.application <> '';
UPDATE provision_history h SET user_id = u.id
  FROM users u WHERE u.institution_id = h.institution_id AND u.name = h."user" AND h."user" <> '';

ALTER TABLE connectors           DROP COLUMN application;
ALTER TABLE account_strategies   DROP COLUMN application;
ALTER TABLE application_accounts DROP COLUMN application;
ALTER TABLE application_accounts DROP COLUMN "user";
ALTER TABLE provision_history    DROP COLUMN application;
ALTER TABLE provision_history    DROP COLUMN "user";

ALTER TABLE connectors ADD CONSTRAINT connectors_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;
ALTER TABLE account_strategies ADD CONSTRAINT account_strategies_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;
ALTER TABLE application_accounts ADD CONSTRAINT application_accounts_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;
ALTER TABLE application_accounts ADD CONSTRAINT application_accounts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE provision_history ADD CONSTRAINT provision_history_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE SET NULL;
ALTER TABLE provision_history ADD CONSTRAINT provision_history_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX connectors_application_idx ON connectors (institution_id, application_id);
CREATE INDEX account_strategies_application_idx ON account_strategies (institution_id, application_id);

-- 幂等键由 (institution, application_code, user_name) 变为 (institution, application_id, user_id)
ALTER TABLE application_accounts DROP CONSTRAINT IF EXISTS application_accounts_inst_app_user_key;
ALTER TABLE application_accounts ADD CONSTRAINT application_accounts_inst_app_user_key
    UNIQUE (institution_id, application_id, user_id);
CREATE INDEX application_accounts_user_idx ON application_accounts (institution_id, user_id);
CREATE INDEX provision_history_user_idx ON provision_history (institution_id, user_id);
