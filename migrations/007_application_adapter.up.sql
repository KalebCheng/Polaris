-- 007: 应用适配器（P4，借鉴 MaxKey mxk_apps_adapters）。
-- 依据：docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md §3.5(a) 与 ADR D6。
--
-- 为应用挂一个"出口映射"：决定 userinfo / id_token 签发时的 claim 与属性映射。
--   applications.adapter        ∈ { default, custom }（默认 default，保持现状行为）
--   applications.adapter_config JSONB，结构 {"claimMap": {"<claim>": "<userAttr>"}}
--     仅当 adapter = 'custom' 时生效；default 时忽略（D6）。
--
-- 可逆：见 007_application_adapter.down.sql（删除两列，无损回滚到迁移前结构）。

ALTER TABLE applications ADD COLUMN adapter        TEXT  NOT NULL DEFAULT 'default';
ALTER TABLE applications ADD COLUMN adapter_config JSONB;

ALTER TABLE applications ADD CONSTRAINT applications_adapter_check
    CHECK (adapter IN ('default', 'custom'));
