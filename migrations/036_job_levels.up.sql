-- 036：职级字典（job_levels）+ 用户单值引用（users.job_level_id）
--
-- 决策（Spec 第六阶段 Q4 / Q4.1 / Q4.3）：职级与职务同构——本地可维护、也可从身份源（飞书）同步；
-- 但**不参与授权**（Q4.1 = A）：仅作为用户属性用于展示 / 筛选 / 统计。
-- sort_order 对齐飞书 order 语义（数值越小越靠前）。
CREATE TABLE job_levels (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    sort_order     INTEGER     NOT NULL DEFAULT 100,
    is_enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT job_levels_inst_name_key UNIQUE (institution_id, name)
);
CREATE INDEX job_levels_inst_idx ON job_levels (institution_id, sort_order);

ALTER TABLE users ADD COLUMN job_level_id BIGINT REFERENCES job_levels(id) ON DELETE SET NULL;
CREATE INDEX users_job_level_idx ON users (institution_id, job_level_id);
