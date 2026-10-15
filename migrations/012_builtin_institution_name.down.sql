-- 回滚：展示名由 Polaris 改回「内置组织」（同样按当前值精确匹配）。
UPDATE institutions SET display_name = '内置组织', updated_at = NOW()
 WHERE name = 'built-in' AND display_name = 'Polaris';

UPDATE departments SET display_name = '内置组织', updated_at = NOW()
 WHERE institution = 'built-in' AND name = 'built-in' AND display_name = 'Polaris';

UPDATE departments
   SET name_path = regexp_replace(name_path, '^Polaris', '内置组织'), updated_at = NOW()
 WHERE institution = 'built-in' AND name_path LIKE 'Polaris%';
