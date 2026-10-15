-- 029：短信模板变量声明（providers.template_params）。
--
-- 背景：阿里云 SendSms 的 TemplateParam 必须与模板内容里的 ${变量} **严格匹配**
-- （多传、少传、名字对不上都会被直接拒绝），通用 HTTP 网关同理。此前发送侧写死
-- `{"code": ...}`，于是任何带 ${minutes} 之类变量的模板都发不出去——"模板支持哪些变量"
-- 必须由模板自己声明，发送时按声明装配。
--
-- 取值：内置变量集见 internal/service/provider/template.go（code / minutes / phone /
-- institution / application），未声明的名字在落库前就被拒绝，不会出现"配了但取不到值"。
-- 空数组 = 按历史行为只发 code。
--
-- 回滚：029_provider_sms_template_params.down.sql

ALTER TABLE providers
    ADD COLUMN IF NOT EXISTS template_params TEXT[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN providers.template_params IS '短信模板变量声明（内置变量集合的子集）；空 = 默认只发 code';
