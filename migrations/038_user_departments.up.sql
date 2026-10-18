-- 038：用户 ↔ 部门多值归属（Spec 第六阶段 Q2 = A）
--
-- 动机：一个人可能同时在多个部门挂职（钉钉 dept_id_list 是多值），而 users.department_id 只能存一个，
-- 同步时"只取第一个能解析的部门"把其余信息直接丢弃。
-- 约定：user_departments 是**多值关系的唯一真相**；users.department_id 保留为「主部门」（is_primary），
-- 让既有 DTO / 列表筛选 / 应用访问判定零改动兼容。
CREATE TABLE user_departments (
    id             BIGSERIAL   PRIMARY KEY,
    institution_id BIGINT      NOT NULL REFERENCES institutions(id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id  BIGINT      NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    is_primary     BOOLEAN     NOT NULL DEFAULT FALSE,
    syncer_id      BIGINT      REFERENCES syncers(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_departments_key UNIQUE (user_id, department_id)
);
-- 一人至多一个主部门
CREATE UNIQUE INDEX user_departments_primary_key ON user_departments (user_id) WHERE is_primary;
CREATE INDEX user_departments_dept_idx ON user_departments (institution_id, department_id);

-- 存量回填：把 users.department_id 提升为"主部门"归属
INSERT INTO user_departments (institution_id, user_id, department_id, is_primary)
SELECT institution_id, id, department_id, TRUE FROM users WHERE department_id IS NOT NULL
ON CONFLICT DO NOTHING;
