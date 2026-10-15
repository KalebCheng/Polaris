-- 012: 默认（内置）组织的展示名由「内置组织」改为 Polaris。
-- 原值仅由种子数据写入新库产生，故此处按原值精确匹配纠正（幂等，且不误改已被管理员改过的名称）。
UPDATE institutions SET display_name = 'Polaris', updated_at = NOW()
 WHERE name = 'built-in' AND display_name = '内置组织';

-- 内置组织即其根部门：展示名与名称路径同源
UPDATE departments SET display_name = 'Polaris', updated_at = NOW()
 WHERE institution = 'built-in' AND name = 'built-in' AND display_name = '内置组织';

-- 子孙部门的 name_path 以「内置组织/」为前缀，需同步替换（code_path 基于标识，不受影响）
UPDATE departments
   SET name_path = regexp_replace(name_path, '^内置组织', 'Polaris'), updated_at = NOW()
 WHERE institution = 'built-in' AND name_path LIKE '内置组织%';
