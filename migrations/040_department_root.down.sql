-- 040 回滚：删除根部门标记与自动补出的根行（其子部门回到顶层）
UPDATE departments SET parent_id = NULL WHERE parent_id IN (SELECT id FROM departments WHERE is_root);
DROP INDEX IF EXISTS departments_root_key;
ALTER TABLE departments DROP COLUMN IF EXISTS is_root;
