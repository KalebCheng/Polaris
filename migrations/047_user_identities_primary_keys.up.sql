-- 047：外部身份绑定改主键（Spec 第六阶段 Q5 第 5 批第 3 段）
--
-- `user_identities` 用 `users.name` / `providers.name` 做关联字符串，且唯一键是
-- `(provider, provider_user_id)` **不含机构**——两个组织接入同名 provider 会互相撞（真缺陷，见 Spec §2.2）。
-- 改为 `user_id` / `provider_id` + 外键，唯一键收敛为 `(institution_id, provider_id, provider_user_id)`。
ALTER TABLE user_identities ADD COLUMN user_id BIGINT;
ALTER TABLE user_identities ADD COLUMN provider_id BIGINT;

UPDATE user_identities ui SET user_id = u.id
  FROM users u
 WHERE u.institution_id = ui.institution_id AND u.name = ui."user";
UPDATE user_identities ui SET provider_id = p.id
  FROM providers p
 WHERE p.institution_id = ui.institution_id AND p.name = ui.provider;

-- 用户解析不到 → 绑定行无意义（用户已删除）；provider 解析不到则保留（provider_id 可空）
DELETE FROM user_identities WHERE user_id IS NULL;

ALTER TABLE user_identities DROP CONSTRAINT IF EXISTS user_identities_inst_provider_key;
ALTER TABLE user_identities DROP COLUMN "user";
ALTER TABLE user_identities DROP COLUMN provider;
ALTER TABLE user_identities ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_provider_id_fkey
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE SET NULL;
ALTER TABLE user_identities ADD CONSTRAINT user_identities_inst_provider_key
    UNIQUE (institution_id, provider_id, provider_user_id);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);
CREATE INDEX user_identities_inst_user_idx ON user_identities (institution_id, user_id);
