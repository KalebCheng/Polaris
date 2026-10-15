-- 030 用户目录查询索引（第五阶段）
--
-- 动机：
--   1) 用户列表按「部门 / 账号状态 / 是否绑定 MFA」筛选与统计；
--   2) 用户详情「授权」标签要反查"此人的直接角色 / 直接权限 / 所属用户组"，
--      role_users 与 permission_users 的主键都是 (左侧 id, "user")，按用户反查只能全表扫。
-- 均为纯索引变更，不改列、不回填数据（迁移不可变，历史迁移不回改）。
CREATE INDEX IF NOT EXISTS users_institution_flags_idx ON users (institution_id, is_deleted, is_forbidden);
CREATE INDEX IF NOT EXISTS users_institution_department_idx ON users (institution_id, department_id);
CREATE INDEX IF NOT EXISTS role_users_user_idx ON role_users ("user");
CREATE INDEX IF NOT EXISTS permission_users_user_idx ON permission_users ("user");
