-- 040：组织恒定作为部门树的根节点（Spec 第六阶段 Q4.5）
--
-- 现状问题（实测）：departments 表没有"根"的概念，"根"只是 seed 里的约定（name = 租户名、parent_id IS NULL，
-- 且只对内置组织生效）；新建组织根本没有根部门，于是钉钉同步来的**占位根部门**（dept_id=1、name="1"）
-- 就成了部门树的顶层 —— 实测机构 2 的 departments.id=119 就是它。
--
-- 本迁移把该约定正式化为**实体根**：每个组织恰好一行 is_root = TRUE 的部门（约束保证唯一），
-- 其余部门一律挂在根之下；界面上它渲染为"组织"本身，不允许改名/移动/删除。
ALTER TABLE departments ADD COLUMN is_root BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX departments_root_key ON departments (institution_id) WHERE is_root;

-- 1) 既有的约定行（name = 租户名 且无父）标记为根
UPDATE departments d SET is_root = TRUE
  FROM institutions i
 WHERE d.institution_id = i.id AND d.name = i.name AND d.parent_id IS NULL AND NOT d.is_root;

-- 2) 还没有根的组织：补一行（name = 组织标识，display_name = 组织展示名）
--    组织标识唯一 ⇒ 满足 departments_inst_name_key（039 补回）
INSERT INTO departments (institution_id, name, display_name, description, code_path, name_path, level, is_root)
SELECT i.id, i.name, i.display_name, '', i.name, i.display_name, 0, TRUE
  FROM institutions i
 WHERE NOT EXISTS (SELECT 1 FROM departments d WHERE d.institution_id = i.id AND d.is_root);

-- 3) 其余顶层部门挂到根之下
UPDATE departments d SET parent_id = r.id
  FROM departments r
 WHERE r.institution_id = d.institution_id AND r.is_root AND d.parent_id IS NULL AND NOT d.is_root;

-- 4) 按新层级重算派生列（code_path / name_path / level / has_child）
WITH RECURSIVE tree AS (
    SELECT id, parent_id, name, display_name, name AS cpath, display_name AS npath, 0 AS lvl
      FROM departments WHERE parent_id IS NULL
    UNION ALL
    SELECT d.id, d.parent_id, d.name, d.display_name,
           t.cpath || '/' || d.name, t.npath || '/' || d.display_name, t.lvl + 1
      FROM departments d JOIN tree t ON d.parent_id = t.id
)
UPDATE departments d SET code_path = t.cpath, name_path = t.npath, level = t.lvl FROM tree t WHERE t.id = d.id;

UPDATE departments p SET has_child = TRUE
 WHERE EXISTS (SELECT 1 FROM departments c WHERE c.parent_id = p.id);
