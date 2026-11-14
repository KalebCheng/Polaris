-- Organization-level sign-in methods. Application rows keep only selection links.
-- Legacy application JSON columns remain until 008 for controlled rollback.

CREATE TABLE public.institution_signin_methods (
    id bigserial NOT NULL,
    institution_id bigint NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    identifiers text[] DEFAULT '{}'::text[] NOT NULL,
    provider_id bigint,
    email_template_id bigint,
    sms_template_id bigint,
    ttl_seconds integer DEFAULT 0 NOT NULL,
    mfa_enabled boolean DEFAULT false NOT NULL,
    mfa_required boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT institution_signin_methods_name_check
        CHECK (name = ANY (ARRAY['Password'::text, 'EmailCode'::text, 'SMSCode'::text, 'MagicLink'::text, 'Provider'::text])),
    CONSTRAINT institution_signin_methods_state_check
        CHECK (state = ANY (ARRAY['active'::text, 'disabled'::text])),
    CONSTRAINT institution_signin_methods_mfa_check
        CHECK ((NOT mfa_required) OR mfa_enabled),
    CONSTRAINT institution_signin_methods_shape_check CHECK (
        (name = 'Password'::text
            AND provider_id IS NULL
            AND email_template_id IS NULL
            AND sms_template_id IS NULL
            AND cardinality(identifiers) >= 1
            AND ttl_seconds = 0)
        OR (name = 'Provider'::text
            AND provider_id IS NOT NULL
            AND email_template_id IS NULL
            AND sms_template_id IS NULL
            AND cardinality(identifiers) = 0
            AND ttl_seconds = 0)
        OR (name = ANY (ARRAY['EmailCode'::text, 'MagicLink'::text])
            AND provider_id IS NOT NULL
            AND email_template_id IS NOT NULL
            AND sms_template_id IS NULL
            AND cardinality(identifiers) = 0
            AND ttl_seconds > 0)
        OR (name = 'SMSCode'::text
            AND provider_id IS NOT NULL
            AND email_template_id IS NULL
            AND sms_template_id IS NOT NULL
            AND cardinality(identifiers) = 0
            AND ttl_seconds > 0)
    ),
    CONSTRAINT institution_signin_methods_pkey PRIMARY KEY (id),
    CONSTRAINT institution_signin_methods_institution_id_fkey
        FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX institution_signin_methods_id_institution_key
    ON public.institution_signin_methods (id, institution_id);
CREATE UNIQUE INDEX institution_signin_methods_password_key
    ON public.institution_signin_methods (institution_id)
    WHERE name = 'Password'::text;
CREATE UNIQUE INDEX institution_signin_methods_channel_key
    ON public.institution_signin_methods (institution_id, name)
    WHERE name = ANY (ARRAY['EmailCode'::text, 'SMSCode'::text, 'MagicLink'::text]);
CREATE UNIQUE INDEX institution_signin_methods_provider_key
    ON public.institution_signin_methods (institution_id, provider_id)
    WHERE name = 'Provider'::text;

CREATE TABLE public.institution_signin_method_mfa_providers (
    institution_id bigint NOT NULL,
    method_id bigint NOT NULL,
    provider_id bigint NOT NULL,
    CONSTRAINT institution_signin_method_mfa_providers_pkey PRIMARY KEY (method_id, provider_id),
    CONSTRAINT institution_signin_method_mfa_method_fk
        FOREIGN KEY (method_id, institution_id)
        REFERENCES public.institution_signin_methods(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT institution_signin_method_mfa_provider_fk
        FOREIGN KEY (provider_id, institution_id)
        REFERENCES public.providers(id, institution_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX applications_id_institution_key
    ON public.applications (id, institution_id);

CREATE TABLE public.application_signin_methods (
    institution_id bigint NOT NULL,
    application_id bigint NOT NULL,
    method_id bigint NOT NULL,
    CONSTRAINT application_signin_methods_pkey PRIMARY KEY (application_id, method_id),
    CONSTRAINT application_signin_methods_application_fk
        FOREIGN KEY (application_id, institution_id)
        REFERENCES public.applications(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT application_signin_methods_method_fk
        FOREIGN KEY (method_id, institution_id)
        REFERENCES public.institution_signin_methods(id, institution_id) ON DELETE RESTRICT
);

CREATE INDEX application_signin_methods_method_idx
    ON public.application_signin_methods (method_id);

CREATE UNIQUE INDEX provider_email_templates_id_institution_key
    ON public.provider_email_templates (id, institution_id);
CREATE UNIQUE INDEX provider_sms_templates_id_institution_key
    ON public.provider_sms_templates (id, institution_id);

ALTER TABLE public.institution_signin_methods
    ADD CONSTRAINT institution_signin_methods_provider_fk
        FOREIGN KEY (provider_id, institution_id)
        REFERENCES public.providers(id, institution_id) ON DELETE RESTRICT,
    ADD CONSTRAINT institution_signin_methods_email_template_fk
        FOREIGN KEY (email_template_id, institution_id)
        REFERENCES public.provider_email_templates(id, institution_id) ON DELETE RESTRICT,
    ADD CONSTRAINT institution_signin_methods_sms_template_fk
        FOREIGN KEY (sms_template_id, institution_id)
        REFERENCES public.provider_sms_templates(id, institution_id) ON DELETE RESTRICT;

CREATE TABLE public.signin_method_migration_conflicts (
    id bigserial NOT NULL,
    institution_id bigint NOT NULL,
    application_id bigint,
    method_name text NOT NULL,
    detail jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT signin_method_migration_conflicts_pkey PRIMARY KEY (id)
);

CREATE INDEX signin_method_migration_conflicts_institution_idx
    ON public.signin_method_migration_conflicts (institution_id);
