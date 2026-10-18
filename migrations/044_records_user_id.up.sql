-- 044：审计记录补 `user_id`（Spec 第六阶段 Q5 第 4 批；审计表按"名称快照 + 可空主键"处理）
--
-- 审计是 **append-only 的历史**：事件发生时的操作者名称必须原样保留（对象改名/删除后仍可读），
-- 因此 `records."user"` 保留为快照，另加可空 `user_id` 供按人精确检索（`ON DELETE SET NULL`，
-- 删除用户不会连带删除审计行）。
ALTER TABLE records ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
UPDATE records r SET user_id = u.id
  FROM users u
 WHERE u.institution_id = r.institution_id AND u.name = r."user" AND r."user" <> '';
CREATE INDEX records_user_id_idx ON records (institution_id, user_id);
