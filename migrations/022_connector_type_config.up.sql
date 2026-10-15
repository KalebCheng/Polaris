-- 022：出方向（connectors）多类型化 + config/secrets 收敛 + 应用↔连接器 1:1
--
-- 背景（Spec §5.4）：
--   - 现状没有 type 列，代码里 `connectorProvisionerType()` 硬编码返回 "http"，注册表形同虚设；
--   - filters / schedule 两个列全仓无读取点（死配置），随本步删除；
--   - just_in_time 有读取点（JIT 建号），本步保留，其与账号策略 createWhen=jit 的归并随 3.4 处理。
--
-- 回滚：022_connector_type_config.down.sql

ALTER TABLE connectors ADD COLUMN type    TEXT  NOT NULL DEFAULT 'http';
ALTER TABLE connectors ADD COLUMN config  JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE connectors ADD COLUMN secrets JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE connectors SET config = jsonb_strip_nulls(jsonb_build_object(
    'provider_url', to_jsonb(NULLIF(provider_url, '')),
    'principal',    to_jsonb(NULLIF(principal, ''))
));

UPDATE connectors SET secrets = jsonb_strip_nulls(jsonb_build_object(
    'credentials', to_jsonb(NULLIF(credentials, ''))
));

-- 应用↔连接器 1:1：历史数据若同一应用绑定了多个连接器，保留 id 最小者，其余解绑（确定性收敛）
UPDATE connectors c SET application = ''
WHERE c.application <> '' AND EXISTS (
    SELECT 1 FROM connectors d
    WHERE d.institution_id = c.institution_id
      AND d.application = c.application
      AND d.id < c.id
);

CREATE UNIQUE INDEX connectors_app_unique
    ON connectors (institution_id, application)
    WHERE application <> '';

ALTER TABLE connectors
    DROP COLUMN provider_url,
    DROP COLUMN principal,
    DROP COLUMN credentials,
    DROP COLUMN filters,     -- 死配置：无读取点（供给范围由账号策略 + 应用表达）
    DROP COLUMN schedule;    -- 死配置：无读取点（出方向定时校验随 3.4 用 sync_config 承载）
