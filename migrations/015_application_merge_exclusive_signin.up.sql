-- 015: 合并「独占登录」到「最大同时登录会话数」。
-- 依据：docs/specs/261013-application-module-phase2.md（「配置即生效」与"不留相悖配置"）。
--
-- 判定：enforceSessionPolicy 中 enable_exclusive_signin 分支的语义就是"只保留最新会话"，
-- 与 max_sessions = 1 完全等价——两个字段表达同一件事（与已删除的 enable_sign_up /
-- disable_self_signup 属同一类问题）。
--
-- 回填：历史上开启独占的应用 → max_sessions = 1（其余保持原值；0 仍表示"不限制"）。
-- 回滚：015_application_merge_exclusive_signin.down.sql

UPDATE applications
   SET max_sessions = 1,
       updated_at   = NOW()
 WHERE enable_exclusive_signin;

ALTER TABLE applications DROP COLUMN IF EXISTS enable_exclusive_signin;
