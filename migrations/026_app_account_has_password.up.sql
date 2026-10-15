-- 026：应用账号不再存储可解密的下游口令
--
-- 背景（Spec FR-11 / Q6）：`app_accounts.related_password` 保存的是可解密（AES-GCM）的
-- 下游明文口令。一次库泄露即等于所有下游账号被接管；而本地系统并不需要它来做任何事
-- （建号由下游消费、改密由下游返回）。
--
-- 处置：删除密文列，改为 `has_password` 布尔——只表达"建号时设置过初始口令"这一事实，
-- 供界面展示"已设置"，密钥本体不落库（审计里也不写，否则等于换个地方泄漏）。
--
-- 回滚：026_app_account_has_password.down.sql（恢复密文列，但历史口令无法找回）

ALTER TABLE app_accounts ADD COLUMN has_password BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE app_accounts SET has_password = (related_password <> '');
ALTER TABLE app_accounts DROP COLUMN related_password;
