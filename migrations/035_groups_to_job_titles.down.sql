-- 035 回滚：job_titles → groups（层级与无读取点字段一并恢复为空列）
ALTER TABLE application_subjects DROP CONSTRAINT IF EXISTS application_subjects_type_check;
UPDATE application_subjects SET subject_type = 'group' WHERE subject_type = 'job_title';
ALTER TABLE application_subjects
    ADD CONSTRAINT application_subjects_type_check
    CHECK (subject_type = ANY (ARRAY['user'::text, 'group'::text, 'department'::text]));

ALTER TABLE job_titles ADD COLUMN IF NOT EXISTS parent_id BIGINT;
ALTER TABLE job_titles ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT '';
ALTER TABLE job_titles ADD COLUMN IF NOT EXISTS properties JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE job_titles ADD COLUMN IF NOT EXISTS is_top_group BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE job_titles ADD CONSTRAINT groups_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES job_titles(id) ON DELETE SET NULL;

ALTER TABLE permission_job_titles RENAME CONSTRAINT permission_job_titles_pkey TO permission_groups_pkey;
ALTER TABLE permission_job_titles RENAME COLUMN job_title TO "group";
ALTER TABLE permission_job_titles RENAME TO permission_groups;
ALTER TABLE role_job_titles RENAME CONSTRAINT role_job_titles_pkey TO role_groups_pkey;
ALTER TABLE role_job_titles RENAME COLUMN job_title TO "group";
ALTER TABLE role_job_titles RENAME TO role_groups;

ALTER INDEX job_title_members_user_idx RENAME TO group_members_user_idx;
ALTER TABLE job_title_members RENAME CONSTRAINT job_title_members_pkey TO group_members_pkey;
ALTER TABLE job_title_members RENAME COLUMN job_title_id TO group_id;
ALTER TABLE job_title_members RENAME CONSTRAINT job_title_members_job_title_id_fkey TO group_members_group_id_fkey;
ALTER TABLE job_title_members RENAME TO group_members;

ALTER INDEX job_titles_institution_id_idx RENAME TO groups_institution_id_idx;
ALTER INDEX job_titles_pkey RENAME TO groups_pkey;
ALTER SEQUENCE job_titles_id_seq RENAME TO groups_id_seq;
ALTER TABLE job_titles RENAME CONSTRAINT job_titles_institution_id_fkey TO groups_institution_id_fkey;
ALTER TABLE job_titles RENAME TO groups;
