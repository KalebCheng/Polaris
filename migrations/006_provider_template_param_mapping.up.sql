-- 短信模板参数从"平台变量名数组"升级为"厂商变量名 → 平台占位符"的显式映射。
-- 存量数据按 key = source = 原变量名转换，发送行为保持等价。

ALTER TABLE public.provider_sms_templates
    ALTER COLUMN template_params DROP DEFAULT;

ALTER TABLE public.provider_sms_templates
    ALTER COLUMN template_params TYPE jsonb
    USING to_jsonb(template_params);

UPDATE public.provider_sms_templates AS template
SET template_params = COALESCE(
    (
        SELECT jsonb_agg(
            jsonb_build_object('key', item.value, 'source', item.value)
            ORDER BY item.ordinality
        )
        FROM jsonb_array_elements_text(template.template_params)
            WITH ORDINALITY AS item(value, ordinality)
    ),
    '[]'::jsonb
);

ALTER TABLE public.provider_sms_templates
    ALTER COLUMN template_params SET DEFAULT '[]'::jsonb;

ALTER TABLE public.provider_sms_templates
    ADD CONSTRAINT provider_sms_templates_params_check
    CHECK (jsonb_typeof(template_params) = 'array');
