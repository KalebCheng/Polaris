-- 016: 删除组织上两个「仅入库/回显、运行时从不读取」的会话策略字段。
-- 依据：AGENTS.md §1「配置即生效」；同类处理的先例见 010（组织冗余字段）与 015（应用独占登录）。
--
-- 判定：institutions.max_sessions 与 institutions.enable_exclusive_signin 在前端没有编辑入口，
-- 全仓也没有任何运行时读取点（会话收敛只在**应用级**由 applications.max_sessions 实现，
-- 见 internal/service/application_policy.go 的 enforceSessionPolicy）。
--
-- 回滚：016_trim_institution_session_fields.down.sql

ALTER TABLE institutions
    DROP COLUMN IF EXISTS enable_exclusive_signin,
    DROP COLUMN IF EXISTS max_sessions;
