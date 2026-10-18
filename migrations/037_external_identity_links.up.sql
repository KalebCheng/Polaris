-- 037：外部身份映射子表（Spec 第六阶段 Q1 = A / Q2）
--
-- 目的：把"外部系统里的对象"锚定到"本地对象"，键为 **连接 + 源端唯一标识**（不再依赖本地账号名）。
-- 如此：源端改名不产生新对象、同步天然幂等、一个人挂多部门不再等于多个人。
-- 设计：每类连接方向一张表（真外键，避免多态列违反 §3.8）；raw 存源端报文，供详情页排障展示。
CREATE TABLE syncer_user_links (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    syncer_id      BIGINT      NOT NULL REFERENCES syncers(id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external_id    TEXT        NOT NULL,
    raw            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT syncer_user_links_key      UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_user_links_user_key UNIQUE (syncer_id, user_id)
);
CREATE INDEX syncer_user_links_user_idx ON syncer_user_links (institution_id, user_id);

CREATE TABLE syncer_department_links (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    syncer_id      BIGINT      NOT NULL REFERENCES syncers(id) ON DELETE CASCADE,
    department_id  BIGINT      NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    external_id    TEXT        NOT NULL,
    raw            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT syncer_department_links_key           UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_department_links_department_key UNIQUE (syncer_id, department_id)
);
CREATE INDEX syncer_department_links_dept_idx ON syncer_department_links (institution_id, department_id);

CREATE TABLE syncer_job_title_links (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    syncer_id      BIGINT      NOT NULL REFERENCES syncers(id) ON DELETE CASCADE,
    job_title_id   BIGINT      NOT NULL REFERENCES job_titles(id) ON DELETE CASCADE,
    external_id    TEXT        NOT NULL,
    raw            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT syncer_job_title_links_key            UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_job_title_links_job_title_key   UNIQUE (syncer_id, job_title_id)
);
CREATE INDEX syncer_job_title_links_job_idx ON syncer_job_title_links (institution_id, job_title_id);

CREATE TABLE syncer_job_level_links (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    syncer_id      BIGINT      NOT NULL REFERENCES syncers(id) ON DELETE CASCADE,
    job_level_id   BIGINT      NOT NULL REFERENCES job_levels(id) ON DELETE CASCADE,
    external_id    TEXT        NOT NULL,
    raw            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT syncer_job_level_links_key            UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_job_level_links_job_level_key   UNIQUE (syncer_id, job_level_id)
);
CREATE INDEX syncer_job_level_links_level_idx ON syncer_job_level_links (institution_id, job_level_id);
