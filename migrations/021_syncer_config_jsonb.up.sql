-- 021：入方向（syncers）收敛为「公共列 + config + secrets + field_mapping」
--
-- 背景（docs/specs/261013-identity-connections-phase3.md §5.4）：
--   现状是一张"万能宽表"：每新增一种接入方式就要加列，且已出现"有列没人读"的死字段
--   （agent_id 全仓无读取点）。本步把类型专属字段迁入 JSONB，公共列只留调度/范围/运行态。
--
-- 约定：
--   - config：类型专属连接配置（非敏感），键为对外契约里的扁平字段名；
--   - secrets：类型专属凭据（沿用既有的 AES-GCM 密文，迁移只搬运不重加密）；
--   - field_mapping：字段映射，按分区组织（account → {本地字段: 源字段}）；
--   - 对外 API/DTO 契约不变（仍以扁平字段读写），变更只在存储层。
--
-- 回滚：021_syncer_config_jsonb.down.sql（把 JSONB 还原为列）

ALTER TABLE syncers ADD COLUMN config        JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE syncers ADD COLUMN secrets       JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE syncers ADD COLUMN field_mapping JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE syncers SET config = jsonb_strip_nulls(jsonb_build_object(
    'host',          to_jsonb(NULLIF(host, '')),
    'port',          to_jsonb(NULLIF(port, 0)::text),
    'user',          to_jsonb(NULLIF("user", '')),
    'database',      to_jsonb(NULLIF(database, '')),
    'table',         to_jsonb(NULLIF("table", '')),
    'database_type', to_jsonb(NULLIF(COALESCE(NULLIF(database_type, ''), sub_type), '')),
    'ssl_mode',      to_jsonb(NULLIF(ssl_mode, '')),
    'app_id',        to_jsonb(NULLIF(app_id, '')),
    'corp_id',       to_jsonb(NULLIF(corp_id, ''))
));

UPDATE syncers SET secrets = jsonb_strip_nulls(jsonb_build_object(
    'password',   to_jsonb(NULLIF(password, '')),
    'app_secret', to_jsonb(NULLIF(app_secret, ''))
));

UPDATE syncers SET field_mapping = jsonb_build_object('account', COALESCE(user_mapping, '{}'::jsonb));

-- 删除死字段与已迁走的列
ALTER TABLE syncers
    DROP COLUMN host,
    DROP COLUMN port,
    DROP COLUMN "user",
    DROP COLUMN password,
    DROP COLUMN database,
    DROP COLUMN "table",
    DROP COLUMN database_type,
    DROP COLUMN ssl_mode,
    DROP COLUMN app_id,
    DROP COLUMN app_secret,
    DROP COLUMN agent_id,      -- 全仓无读取点（企业微信不需要独立 AgentID）
    DROP COLUMN corp_id,
    DROP COLUMN user_mapping,  -- → field_mapping.account
    DROP COLUMN group_mapping; -- 无读取点（组织映射随 3.3 部门同步在 field_mapping.organization 下提供）
