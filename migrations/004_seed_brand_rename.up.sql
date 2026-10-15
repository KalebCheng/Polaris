-- 品牌改名（Siming → Polaris）后同步内置应用凭据。
-- 说明：种子数据只在空库时写入，已初始化的环境需通过本迁移把内置应用的
-- client_id / client_secret 一并改名，避免与新种子定义不一致（冒烟测试与文档均按新值引用）。
UPDATE applications SET client_id = 'polaris-built-in-client', updated_at = NOW()
    WHERE client_id = 'siming-built-in-client';
UPDATE applications SET client_secret = 'polaris-built-in-secret', updated_at = NOW()
    WHERE client_secret = 'siming-built-in-secret';
