DROP TABLE IF EXISTS public.signin_method_migration_conflicts;
DROP TABLE IF EXISTS public.application_signin_methods;
DROP TABLE IF EXISTS public.institution_signin_method_mfa_providers;
DROP TABLE IF EXISTS public.institution_signin_methods;

DROP INDEX IF EXISTS public.provider_sms_templates_id_institution_key;
DROP INDEX IF EXISTS public.provider_email_templates_id_institution_key;
DROP INDEX IF EXISTS public.applications_id_institution_key;
