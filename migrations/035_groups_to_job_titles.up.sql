-- 035：用户组（groups）升级为「职务」（job_titles）
--
-- 决策（Spec 第六阶段 Q3 = A / Q3.1 / Q4）：
--   1) 职务 = 组织架构里的「人 × 职务」关系，是批量授权的载体（挂角色 / 挂权限 / 作应用访问主体）；
--   2) 命名对齐飞书 job_title，表名一律完整单词：groups → job_titles、group_members → job_title_members、
--      role_groups → role_job_titles、permission_groups → permission_job_titles；
--   3) application_subjects.subject_type 取值 'group' → 'job_title'；
--   4) **不做层级**（Q4）：飞书职务字典无层级、钉钉/企微职务是自由文本，层级无源数据可填；
--      且旧组层级在界面上根本无从设置（实测 parent_id 全为 NULL），属"半死能力"，按 §3.1-10 拆除。
--      连带删除同样无读取点的 type / properties / is_top_group。

ALTER TABLE groups RENAME TO job_titles;
ALTER TABLE job_titles RENAME CONSTRAINT groups_institution_id_fkey TO job_titles_institution_id_fkey;
ALTER SEQUENCE groups_id_seq RENAME TO job_titles_id_seq;
ALTER INDEX groups_pkey RENAME TO job_titles_pkey;
ALTER INDEX groups_institution_id_idx RENAME TO job_titles_institution_id_idx;

-- 关联表改名（主键约束名同步改）
ALTER TABLE group_members RENAME TO job_title_members;
ALTER TABLE job_title_members RENAME CONSTRAINT group_members_group_id_fkey TO job_title_members_job_title_id_fkey;
ALTER TABLE job_title_members RENAME COLUMN group_id TO job_title_id;
ALTER TABLE job_title_members RENAME CONSTRAINT group_members_pkey TO job_title_members_pkey;
ALTER INDEX group_members_user_idx RENAME TO job_title_members_user_idx;

ALTER TABLE role_groups RENAME TO role_job_titles;
ALTER TABLE role_job_titles RENAME COLUMN "group" TO job_title;
ALTER TABLE role_job_titles RENAME CONSTRAINT role_groups_pkey TO role_job_titles_pkey;

ALTER TABLE permission_groups RENAME TO permission_job_titles;
ALTER TABLE permission_job_titles RENAME COLUMN "group" TO job_title;
ALTER TABLE permission_job_titles RENAME CONSTRAINT permission_groups_pkey TO permission_job_titles_pkey;

-- 拆除无源数据、界面上也设置不了的层级与无读取点字段
ALTER TABLE job_titles DROP CONSTRAINT IF EXISTS groups_parent_id_fkey;
ALTER TABLE job_titles DROP COLUMN IF EXISTS parent_id;
ALTER TABLE job_titles DROP COLUMN IF EXISTS type;
ALTER TABLE job_titles DROP COLUMN IF EXISTS properties;
ALTER TABLE job_titles DROP COLUMN IF EXISTS is_top_group;

-- 应用访问主体类型：group → job_title
ALTER TABLE application_subjects DROP CONSTRAINT IF EXISTS application_subjects_type_check;
UPDATE application_subjects SET subject_type = 'job_title' WHERE subject_type = 'group';
ALTER TABLE application_subjects
    ADD CONSTRAINT application_subjects_type_check
    CHECK (subject_type = ANY (ARRAY['user'::text, 'job_title'::text, 'department'::text]));
