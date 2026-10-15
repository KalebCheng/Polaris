-- 014: 删除应用上「仅入库/回显、运行时从不读取」的三个外部跳转地址。
-- 依据：docs/specs/261013-application-module-phase2.md（「配置即生效」原则）与 AGENTS.md §1。
--
-- 判定（已在全仓核实读取点）：
--   forget_url       —— 平台没有找回密码流程，登录页也没有渲染"忘记密码"入口；
--   signin_url       —— 未下发、未读取；
--   affiliation_url  —— 未下发、未读取。
-- 保留并已接线的同类字段：signup_url（登录页「去注册」目标）、terms_of_use（注册页协议链接）。
--
-- 回滚：014_application_trim_dead_urls.down.sql

ALTER TABLE applications
    DROP COLUMN IF EXISTS forget_url,
    DROP COLUMN IF EXISTS signin_url,
    DROP COLUMN IF EXISTS affiliation_url;
