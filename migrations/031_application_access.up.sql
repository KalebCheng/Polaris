-- 031 应用访问授权（第六阶段）
--
-- 动机：应用此前只声明"怎么登录"（signin_methods / ip_restriction），从不声明"谁能用"——
-- 任何本组织用户都能登录本组织任何一个应用。本迁移引入"应用准入"的数据面。
--
-- 语义（见 docs/specs/261015-application-access-phase6.md）：
--   access_mode = 'all'        → 全员可访问（默认；与引入该能力之前的行为完全一致）
--   access_mode = 'restricted' → 仅 application_subjects 名单内主体可访问
--   restricted + 空名单        → 暂时无人可访问（合法状态，允许保存，事后补授权）
-- 判定作用域：只在请求携带应用上下文（client_id / application）时生效，"登录平台"不做判定。

-- 1) 访问模式：默认 all，存量应用自动为 all（零回归）
ALTER TABLE applications ADD COLUMN IF NOT EXISTS access_mode TEXT NOT NULL DEFAULT 'all';
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_access_mode_check;
ALTER TABLE applications ADD CONSTRAINT applications_access_mode_check
    CHECK (access_mode IN ('all', 'restricted'));

-- 2) 授权名单：三类主体统一落一张表
--    subject_name 是**组织内唯一标识名**（users.name / groups.name / departments.name），
--    展示名在读取时 JOIN 回实体装配，因此主体改名/删除会体现为"已失效"（判定不命中，不误放行）。
CREATE TABLE IF NOT EXISTS application_subjects (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    application_id BIGINT      NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    subject_type   TEXT        NOT NULL,
    subject_name   TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT application_subjects_type_check CHECK (subject_type IN ('user', 'group', 'department')),
    CONSTRAINT application_subjects_app_type_name_key UNIQUE (application_id, subject_type, subject_name)
);

-- 判定主查询：按应用取名单
CREATE INDEX IF NOT EXISTS application_subjects_app_idx ON application_subjects (application_id, subject_type);
-- 反向查询：某主体被哪些应用授权（删除主体前的提示 / 组织级总览，界面本阶段不做）
CREATE INDEX IF NOT EXISTS application_subjects_subject_idx ON application_subjects (institution_id, subject_type, subject_name);
