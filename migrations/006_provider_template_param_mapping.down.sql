ALTER TABLE public.provider_sms_templates
    DROP CONSTRAINT IF EXISTS provider_sms_templates_params_check;

ALTER TABLE public.provider_sms_templates
    ALTER COLUMN template_params DROP DEFAULT;

ALTER TABLE public.provider_sms_templates
    ADD COLUMN template_params_legacy text[] NOT NULL DEFAULT '{}'::text[];

UPDATE public.provider_sms_templates AS template
SET template_params_legacy = COALESCE(
    (
        SELECT array_agg(item.value ->> 'source' ORDER BY item.ordinality)
        FROM jsonb_array_elements(template.template_params)
            WITH ORDINALITY AS item(value, ordinality)
    ),
    '{}'::text[]
);

ALTER TABLE public.provider_sms_templates
    DROP COLUMN template_params;

ALTER TABLE public.provider_sms_templates
    RENAME COLUMN template_params_legacy TO template_params;
