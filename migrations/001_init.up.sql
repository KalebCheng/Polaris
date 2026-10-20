--
-- PostgreSQL database dump
--


-- Dumped from database version 16.15
-- Dumped by pg_dump version 16.15

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--



SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Sequences
--

CREATE SEQUENCE public.account_strategies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.application_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.application_subjects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.applications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.asyncd_job_run_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.asyncd_schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.asyncd_tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.certs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.connectors_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.institutions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.job_levels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.job_titles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.organizations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.password_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.provider_email_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.provider_sms_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.providers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.provision_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncer_department_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncer_job_level_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncer_job_title_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncer_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncer_user_links_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.syncers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.user_departments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.user_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.user_mfa_credentials_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Tables
--

CREATE TABLE public.institutions (
    id bigint DEFAULT nextval('public.institutions_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    domain text DEFAULT ''::text NOT NULL,
    website_url text DEFAULT ''::text NOT NULL,
    logo text DEFAULT ''::text NOT NULL,
    favicon text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    captcha text DEFAULT ''::text NOT NULL,
    password_type text DEFAULT 'bcrypt'::text NOT NULL,
    password_options text[] DEFAULT '{}'::text[] NOT NULL,
    password_expire_days integer DEFAULT 0 NOT NULL,
    password_history_count integer DEFAULT 0 NOT NULL,
    token_retention_days integer DEFAULT 30 NOT NULL,
    record_retention_days integer DEFAULT 30 NOT NULL,
    default_application text DEFAULT ''::text NOT NULL,
    default_password_enc text DEFAULT ''::text NOT NULL,
    use_email_as_username boolean DEFAULT false NOT NULL,
    disable_signin boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    password_salt text DEFAULT ''::text NOT NULL,
    CONSTRAINT institutions_name_key UNIQUE (name),
    CONSTRAINT institutions_pkey PRIMARY KEY (id)
);

CREATE TABLE public.certs (
    id bigint DEFAULT nextval('public.certs_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'JWT'::text NOT NULL,
    type text DEFAULT 'x509'::text NOT NULL,
    crypto_algorithm text DEFAULT 'RSA'::text NOT NULL,
    bit_size integer DEFAULT 2048 NOT NULL,
    expire_in_years integer DEFAULT 10 NOT NULL,
    certificate text DEFAULT ''::text NOT NULL,
    private_key text DEFAULT ''::text NOT NULL,
    expire_time timestamp with time zone,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    CONSTRAINT certs_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT certs_pkey PRIMARY KEY (id),
    CONSTRAINT certs_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.job_titles (
    id bigint DEFAULT nextval('public.job_titles_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    manager text DEFAULT ''::text NOT NULL,
    contact_email text DEFAULT ''::text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    CONSTRAINT job_titles_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT job_titles_pkey PRIMARY KEY (id),
    CONSTRAINT job_titles_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.applications (
    id bigint DEFAULT nextval('public.applications_id_seq'::regclass) NOT NULL,
    code text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    category text DEFAULT 'default'::text NOT NULL,
    type text DEFAULT 'oidc'::text NOT NULL,
    logo text DEFAULT ''::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    homepage_url text DEFAULT ''::text NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    cert_id bigint,
    default_job_title_id bigint,
    max_sessions integer DEFAULT 0 NOT NULL,
    grant_types text[] DEFAULT '{authorization_code,password,refresh_token,client_credentials}'::text[] NOT NULL,
    redirect_uris text[] DEFAULT '{}'::text[] NOT NULL,
    providers jsonb DEFAULT '[]'::jsonb NOT NULL,
    scopes jsonb DEFAULT '[]'::jsonb NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret_hash text DEFAULT ''::text NOT NULL,
    token_format text DEFAULT 'custom'::text NOT NULL,
    token_signing_method text DEFAULT 'RS256'::text NOT NULL,
    expire_in_hours double precision DEFAULT 1 NOT NULL,
    refresh_expire_in_hours double precision DEFAULT 24 NOT NULL,
    cookie_expire_in_hours bigint DEFAULT 720 NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    logo_dark text DEFAULT ''::text NOT NULL,
    favicon text DEFAULT ''::text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    org_choice_mode text DEFAULT ''::text NOT NULL,
    ip_restriction text DEFAULT ''::text NOT NULL,
    ip_whitelist text DEFAULT ''::text NOT NULL,
    signup_url text DEFAULT ''::text NOT NULL,
    terms_of_use text DEFAULT ''::text NOT NULL,
    forced_redirect_origin text DEFAULT ''::text NOT NULL,
    backchannel_logout_uri text DEFAULT ''::text NOT NULL,
    disable_refresh_rotation boolean DEFAULT false NOT NULL,
    signin_methods jsonb DEFAULT '[]'::jsonb NOT NULL,
    signup_items jsonb DEFAULT '[]'::jsonb NOT NULL,
    theme_data jsonb,
    adapter text DEFAULT 'default'::text NOT NULL,
    adapter_config jsonb,
    self_signup_enabled boolean DEFAULT false NOT NULL,
    signup_auto_signin boolean DEFAULT true NOT NULL,
    email_code_ttl_seconds integer DEFAULT 300 NOT NULL,
    sms_code_ttl_seconds integer DEFAULT 300 NOT NULL,
    magic_link_ttl_seconds integer DEFAULT 600 NOT NULL,
    institution_id bigint NOT NULL,
    signin_error_limit integer DEFAULT 0 NOT NULL,
    signin_retry_wait_minutes integer DEFAULT 0 NOT NULL,
    access_mode text DEFAULT 'all'::text NOT NULL,
    token_attributes jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT applications_access_mode_check CHECK ((access_mode = ANY (ARRAY['all'::text, 'restricted'::text]))),
    CONSTRAINT applications_adapter_check CHECK ((adapter = ANY (ARRAY['default'::text, 'custom'::text]))),
    CONSTRAINT applications_signin_error_limit_check CHECK ((signin_error_limit >= 0)),
    CONSTRAINT applications_signin_retry_wait_check CHECK ((signin_retry_wait_minutes >= 0)),
    CONSTRAINT applications_inst_code_key UNIQUE (institution_id, code),
    CONSTRAINT applications_pkey PRIMARY KEY (id),
    CONSTRAINT applications_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT applications_cert_id_fkey FOREIGN KEY (cert_id) REFERENCES public.certs(id) ON DELETE SET NULL,
    CONSTRAINT applications_default_job_title_id_fkey FOREIGN KEY (default_job_title_id) REFERENCES public.job_titles(id) ON DELETE SET NULL
);

CREATE TABLE public.account_strategies (
    id bigint DEFAULT nextval('public.account_strategies_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    mapping jsonb DEFAULT '{}'::jsonb NOT NULL,
    policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    application_id bigint,
    CONSTRAINT account_strategies_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT account_strategies_pkey PRIMARY KEY (id),
    CONSTRAINT account_strategies_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE CASCADE,
    CONSTRAINT account_strategies_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.job_levels (
    id bigint DEFAULT nextval('public.job_levels_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    sort_order integer DEFAULT 100 NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_levels_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT job_levels_pkey PRIMARY KEY (id),
    CONSTRAINT job_levels_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.users (
    id bigint DEFAULT nextval('public.users_id_seq'::regclass) NOT NULL,
    uid text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    type text DEFAULT 'normal'::text NOT NULL,
    password_hash text DEFAULT ''::text NOT NULL,
    password_salt text DEFAULT ''::text NOT NULL,
    password_type text DEFAULT 'bcrypt'::text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    first_name text DEFAULT ''::text NOT NULL,
    last_name text DEFAULT ''::text NOT NULL,
    avatar text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    email_verified boolean DEFAULT false NOT NULL,
    phone text DEFAULT ''::text NOT NULL,
    country_code text DEFAULT ''::text NOT NULL,
    location text DEFAULT ''::text NOT NULL,
    address text DEFAULT ''::text NOT NULL,
    affiliation text DEFAULT ''::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    homepage text DEFAULT ''::text NOT NULL,
    bio text DEFAULT ''::text NOT NULL,
    language text DEFAULT ''::text NOT NULL,
    gender text DEFAULT ''::text NOT NULL,
    birthday text DEFAULT ''::text NOT NULL,
    education text DEFAULT ''::text NOT NULL,
    score integer DEFAULT 0 NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    is_forbidden boolean DEFAULT false NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    is_online boolean DEFAULT false NOT NULL,
    signup_application text DEFAULT ''::text NOT NULL,
    register_type text DEFAULT ''::text NOT NULL,
    register_source text DEFAULT ''::text NOT NULL,
    created_ip text DEFAULT ''::text NOT NULL,
    last_signin_time timestamp with time zone,
    last_signin_ip text DEFAULT ''::text NOT NULL,
    password_updated_at timestamp with time zone,
    mfa_enabled boolean DEFAULT false NOT NULL,
    mfa_type text DEFAULT ''::text NOT NULL,
    mfa_secret text DEFAULT ''::text NOT NULL,
    mfa_recovery_codes text[] DEFAULT '{}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    phone_verified boolean DEFAULT false NOT NULL,
    institution_id bigint NOT NULL,
    job_level_id bigint,
    CONSTRAINT users_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT users_pkey PRIMARY KEY (id),
    CONSTRAINT users_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT users_job_level_id_fkey FOREIGN KEY (job_level_id) REFERENCES public.job_levels(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX users_id_institution_key ON public.users USING btree (id, institution_id);

CREATE TABLE public.application_accounts (
    id bigint DEFAULT nextval('public.application_accounts_id_seq'::regclass) NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    related_username text DEFAULT ''::text NOT NULL,
    create_type text DEFAULT 'automatic'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    has_password boolean DEFAULT false NOT NULL,
    application_id bigint,
    user_id bigint,
    CONSTRAINT application_accounts_inst_app_user_key UNIQUE (institution_id, application_id, user_id),
    CONSTRAINT application_accounts_pkey PRIMARY KEY (id),
    CONSTRAINT application_accounts_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE CASCADE,
    CONSTRAINT application_accounts_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT application_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.departments (
    id bigint DEFAULT nextval('public.organizations_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    parent_id bigint,
    has_child boolean DEFAULT false NOT NULL,
    sort_index integer DEFAULT 0 NOT NULL,
    institution_id bigint NOT NULL,
    is_root boolean DEFAULT false NOT NULL,
    parent_id_chain bigint[] DEFAULT '{}'::bigint[] NOT NULL,
    CONSTRAINT departments_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT departments_pkey PRIMARY KEY (id),
    CONSTRAINT departments_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT departments_parent_fkey FOREIGN KEY (parent_id) REFERENCES public.departments(id) ON DELETE SET NULL
);

CREATE TABLE public.application_subjects (
    id bigint DEFAULT nextval('public.application_subjects_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    application_id bigint NOT NULL,
    subject_type text NOT NULL,
    user_id bigint,
    job_title_id bigint,
    department_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT application_subjects_type_check CHECK ((subject_type = ANY (ARRAY['user'::text, 'job_title'::text, 'department'::text]))),
    CONSTRAINT application_subjects_subject_id_check CHECK (
        (subject_type = 'user'::text AND user_id IS NOT NULL AND job_title_id IS NULL AND department_id IS NULL)
        OR (subject_type = 'job_title'::text AND user_id IS NULL AND job_title_id IS NOT NULL AND department_id IS NULL)
        OR (subject_type = 'department'::text AND user_id IS NULL AND job_title_id IS NULL AND department_id IS NOT NULL)
    ),
    CONSTRAINT application_subjects_pkey PRIMARY KEY (id),
    CONSTRAINT application_subjects_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE CASCADE,
    CONSTRAINT application_subjects_department_id_fkey FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE CASCADE,
    CONSTRAINT application_subjects_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT application_subjects_job_title_id_fkey FOREIGN KEY (job_title_id) REFERENCES public.job_titles(id) ON DELETE CASCADE,
    CONSTRAINT application_subjects_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.asyncd_jobs (
    id bigint DEFAULT nextval('public.asyncd_schedules_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    task_type text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    schedule_kind text NOT NULL,
    interval_seconds integer DEFAULT 0 NOT NULL,
    daily_times text[] DEFAULT '{}'::text[] NOT NULL,
    window_minutes integer DEFAULT 30 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    next_run_at timestamp with time zone DEFAULT now() NOT NULL,
    last_run_at timestamp with time zone,
    last_status text DEFAULT 'idle'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    kind text DEFAULT 'scheduled'::text NOT NULL,
    cron_expr text DEFAULT ''::text NOT NULL,
    unique_key text DEFAULT ''::text NOT NULL,
    retired_at timestamp with time zone,
    CONSTRAINT asyncd_jobs_async_no_rule CHECK (((kind <> 'async'::text) OR (schedule_kind = ''::text))),
    CONSTRAINT asyncd_jobs_cron_check CHECK (((schedule_kind <> 'cron'::text) OR (cron_expr <> ''::text))),
    CONSTRAINT asyncd_jobs_daily_times_check CHECK (((schedule_kind <> 'daily'::text) OR ((cardinality(daily_times) >= 1) AND (cardinality(daily_times) <= 24)))),
    CONSTRAINT asyncd_jobs_interval_check CHECK (((schedule_kind <> 'interval'::text) OR (interval_seconds > 0))),
    CONSTRAINT asyncd_jobs_kind_check CHECK ((kind = ANY (ARRAY['async'::text, 'scheduled'::text]))),
    CONSTRAINT asyncd_jobs_schedule_kind_check CHECK ((schedule_kind = ANY (ARRAY[''::text, 'interval'::text, 'daily'::text, 'cron'::text]))),
    CONSTRAINT asyncd_jobs_scheduled_has_rule CHECK (((kind <> 'scheduled'::text) OR (schedule_kind <> ''::text))),
    CONSTRAINT asyncd_jobs_name_key UNIQUE (name),
    CONSTRAINT asyncd_jobs_pkey PRIMARY KEY (id)
);

CREATE TABLE public.asyncd_job_runs (
    id bigint DEFAULT nextval('public.asyncd_tasks_id_seq'::regclass) NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    payload_version integer DEFAULT 1 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 8 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    deadline timestamp with time zone,
    unique_key text DEFAULT ''::text NOT NULL,
    worker_id text DEFAULT ''::text NOT NULL,
    lease_expires_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    institution_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    job_id bigint NOT NULL,
    CONSTRAINT asyncd_job_runs_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'dead'::text]))),
    CONSTRAINT asyncd_job_runs_pkey PRIMARY KEY (id),
    CONSTRAINT asyncd_job_runs_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.asyncd_jobs(id) ON DELETE CASCADE,
    CONSTRAINT asyncd_tasks_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.asyncd_job_run_attempts (
    id bigint DEFAULT nextval('public.asyncd_job_run_attempts_id_seq'::regclass) NOT NULL,
    job_run_id bigint NOT NULL,
    attempt_no integer NOT NULL,
    worker_id text NOT NULL,
    status text DEFAULT 'running'::text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    duration_ms integer DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    CONSTRAINT asyncd_job_run_attempts_status_check CHECK ((status = ANY (ARRAY['running'::text, 'succeeded'::text, 'failed'::text, 'lease_lost'::text]))),
    CONSTRAINT asyncd_job_run_attempts_no_key UNIQUE (job_run_id, attempt_no),
    CONSTRAINT asyncd_job_run_attempts_pkey PRIMARY KEY (id),
    CONSTRAINT asyncd_job_run_attempts_job_run_id_fkey FOREIGN KEY (job_run_id) REFERENCES public.asyncd_job_runs(id) ON DELETE CASCADE
);

CREATE TABLE public.asyncd_workers (
    id text NOT NULL,
    instance_id text NOT NULL,
    host text NOT NULL,
    pid integer NOT NULL,
    slot_index integer NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    last_heartbeat_at timestamp with time zone DEFAULT now() NOT NULL,
    stopped_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT asyncd_workers_pkey PRIMARY KEY (id)
);

CREATE TABLE public.connectors (
    id bigint DEFAULT nextval('public.connectors_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    last_run_time timestamp with time zone,
    last_status text DEFAULT 'idle'::text NOT NULL,
    last_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    type text DEFAULT 'http'::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    secrets jsonb DEFAULT '{}'::jsonb NOT NULL,
    application_id bigint,
    sync_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    schedule_job_id bigint,
    CONSTRAINT connectors_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT connectors_pkey PRIMARY KEY (id),
    CONSTRAINT connectors_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE CASCADE,
    CONSTRAINT connectors_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT connectors_schedule_job_id_fkey FOREIGN KEY (schedule_job_id) REFERENCES public.asyncd_jobs(id) ON DELETE SET NULL
);

CREATE TABLE public.job_title_members (
    job_title_id bigint NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT job_title_members_pkey PRIMARY KEY (job_title_id, user_id),
    CONSTRAINT job_title_members_job_title_id_fkey FOREIGN KEY (job_title_id) REFERENCES public.job_titles(id) ON DELETE CASCADE,
    CONSTRAINT job_title_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.password_history (
    id bigint DEFAULT nextval('public.password_history_id_seq'::regclass) NOT NULL,
    password_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    user_id bigint NOT NULL,
    CONSTRAINT password_history_pkey PRIMARY KEY (id),
    CONSTRAINT password_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.permissions (
    id bigint DEFAULT nextval('public.permissions_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resources text[] DEFAULT '{}'::text[] NOT NULL,
    actions text[] DEFAULT '{}'::text[] NOT NULL,
    effect text DEFAULT 'Allow'::text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    expire_time timestamp with time zone,
    submitter text DEFAULT ''::text NOT NULL,
    approver text DEFAULT ''::text NOT NULL,
    approve_time timestamp with time zone,
    state text DEFAULT 'Approved'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    CONSTRAINT permissions_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT permissions_pkey PRIMARY KEY (id),
    CONSTRAINT permissions_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.permission_job_titles (
    permission_id bigint NOT NULL,
    job_title_id bigint NOT NULL,
    CONSTRAINT permission_job_titles_pkey PRIMARY KEY (permission_id, job_title_id),
    CONSTRAINT permission_groups_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE,
    CONSTRAINT permission_job_titles_job_title_id_fkey FOREIGN KEY (job_title_id) REFERENCES public.job_titles(id) ON DELETE CASCADE
);

CREATE TABLE public.roles (
    id bigint DEFAULT nextval('public.roles_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    CONSTRAINT roles_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT roles_pkey PRIMARY KEY (id),
    CONSTRAINT roles_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE TABLE public.permission_roles (
    permission_id bigint NOT NULL,
    role_id bigint NOT NULL,
    CONSTRAINT permission_roles_pkey PRIMARY KEY (permission_id, role_id),
    CONSTRAINT permission_roles_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE,
    CONSTRAINT permission_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE
);

CREATE TABLE public.permission_users (
    permission_id bigint NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT permission_users_pkey PRIMARY KEY (permission_id, user_id),
    CONSTRAINT permission_users_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE,
    CONSTRAINT permission_users_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.providers (
    id bigint DEFAULT nextval('public.providers_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    category text DEFAULT 'OAuth'::text NOT NULL,
    type text DEFAULT 'oidc'::text NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    CONSTRAINT providers_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT providers_pkey PRIMARY KEY (id),
    CONSTRAINT providers_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX providers_id_institution_key ON public.providers USING btree (id, institution_id);

CREATE TABLE public.provider_captcha (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret text DEFAULT ''::text NOT NULL,
    CONSTRAINT provider_captcha_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_captcha_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provider_email (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    host text DEFAULT ''::text NOT NULL,
    port integer DEFAULT 0 NOT NULL,
    ssl_mode text DEFAULT ''::text NOT NULL,
    from_address text DEFAULT ''::text NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret text DEFAULT ''::text NOT NULL,
    receiver text DEFAULT ''::text NOT NULL,
    CONSTRAINT provider_email_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_email_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provider_email_templates (
    id bigint DEFAULT nextval('public.provider_email_templates_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    provider_id bigint NOT NULL,
    name text NOT NULL,
    scene text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_email_templates_scene_check CHECK ((scene = ANY (ARRAY['login_code'::text, 'magic_link'::text]))),
    CONSTRAINT provider_email_templates_name_key UNIQUE (provider_id, name),
    CONSTRAINT provider_email_templates_pkey PRIMARY KEY (id),
    CONSTRAINT provider_email_templates_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT provider_email_templates_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provider_sms_templates (
    id bigint DEFAULT nextval('public.provider_sms_templates_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    provider_id bigint NOT NULL,
    name text NOT NULL,
    scene text NOT NULL,
    template_code text DEFAULT ''::text NOT NULL,
    template_params text[] DEFAULT '{}'::text[] NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_sms_templates_scene_check CHECK ((scene = ANY (ARRAY['login_code'::text, 'mfa_code'::text]))),
    CONSTRAINT provider_sms_templates_name_key UNIQUE (provider_id, name),
    CONSTRAINT provider_sms_templates_pkey PRIMARY KEY (id),
    CONSTRAINT provider_sms_templates_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT provider_sms_templates_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provider_mfa (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    mfa_issuer text DEFAULT ''::text NOT NULL,
    mfa_digits integer DEFAULT 6 NOT NULL,
    mfa_period integer DEFAULT 30 NOT NULL,
    mfa_algorithm text DEFAULT 'SHA1'::text NOT NULL,
    mfa_sms_provider_id bigint,
    mfa_sms_template_id bigint,
    mfa_code_ttl_seconds integer DEFAULT 300 NOT NULL,
    mfa_remember_hours integer DEFAULT 0 NOT NULL,
    CONSTRAINT provider_mfa_algorithm_check CHECK ((mfa_algorithm = ANY (ARRAY['SHA1'::text, 'SHA256'::text, 'SHA512'::text]))),
    CONSTRAINT provider_mfa_code_ttl_check CHECK (((mfa_code_ttl_seconds >= 30) AND (mfa_code_ttl_seconds <= 1800))),
    CONSTRAINT provider_mfa_digits_check CHECK (((mfa_digits >= 6) AND (mfa_digits <= 8))),
    CONSTRAINT provider_mfa_period_check CHECK (((mfa_period >= 15) AND (mfa_period <= 120))),
    CONSTRAINT provider_mfa_remember_check CHECK (((mfa_remember_hours >= 0) AND (mfa_remember_hours <= 8760))),
    CONSTRAINT provider_mfa_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_mfa_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT provider_mfa_sms_provider_fk FOREIGN KEY (mfa_sms_provider_id) REFERENCES public.providers(id) ON DELETE RESTRICT,
    CONSTRAINT provider_mfa_sms_template_fk FOREIGN KEY (mfa_sms_template_id) REFERENCES public.provider_sms_templates(id) ON DELETE RESTRICT
);

CREATE TABLE public.syncers (
    id bigint DEFAULT nextval('public.syncers_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    type text NOT NULL,
    sub_type text DEFAULT ''::text NOT NULL,
    is_enabled boolean DEFAULT false NOT NULL,
    sync_users boolean DEFAULT true NOT NULL,
    sync_groups boolean DEFAULT false NOT NULL,
    soft_delete boolean DEFAULT false NOT NULL,
    last_run_time timestamp with time zone,
    last_status text DEFAULT 'idle'::text NOT NULL,
    last_message text DEFAULT ''::text NOT NULL,
    last_counts jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    secrets jsonb DEFAULT '{}'::jsonb NOT NULL,
    field_mapping jsonb DEFAULT '{}'::jsonb NOT NULL,
    sync_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    target_department_id bigint,
    schedule_job_id bigint,
    CONSTRAINT syncers_inst_name_key UNIQUE (institution_id, name),
    CONSTRAINT syncers_pkey PRIMARY KEY (id),
    CONSTRAINT syncers_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT syncers_target_department_id_fkey FOREIGN KEY (target_department_id) REFERENCES public.departments(id) ON DELETE SET NULL,
    CONSTRAINT syncers_schedule_job_id_fkey FOREIGN KEY (schedule_job_id) REFERENCES public.asyncd_jobs(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX syncers_id_institution_key ON public.syncers USING btree (id, institution_id);

CREATE TABLE public.provider_oauth (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret text DEFAULT ''::text NOT NULL,
    custom_auth_url text DEFAULT ''::text NOT NULL,
    custom_token_url text DEFAULT ''::text NOT NULL,
    custom_user_info_url text DEFAULT ''::text NOT NULL,
    issuer_url text DEFAULT ''::text NOT NULL,
    scopes text[] DEFAULT '{}'::text[] NOT NULL,
    enable_pkce boolean DEFAULT false NOT NULL,
    auto_create boolean DEFAULT true NOT NULL,
    link_by_email boolean DEFAULT true NOT NULL,
    allowed_domains text[] DEFAULT '{}'::text[] NOT NULL,
    redirect_uri text DEFAULT ''::text NOT NULL,
    match_type text DEFAULT ''::text NOT NULL,
    match_local_field text DEFAULT ''::text NOT NULL,
    match_provider_field text DEFAULT ''::text NOT NULL,
    match_syncer_id bigint,
    match_syncer_field text DEFAULT ''::text NOT NULL,
    -- JIT（不存在时自动建号）的字段映射：某本地字段取自哪个提供商字段；
    -- 空串表示使用默认字段。运行时由 authService 的 provision 读取（配置即生效）。
    provision_username_field text DEFAULT ''::text NOT NULL,
    provision_name_field text DEFAULT ''::text NOT NULL,
    provision_email_field text DEFAULT ''::text NOT NULL,
    provision_phone_field text DEFAULT ''::text NOT NULL,
    provision_avatar_field text DEFAULT ''::text NOT NULL,
    CONSTRAINT provider_oauth_match_shape_check CHECK (
        (match_type = ''::text
            AND match_local_field = ''::text
            AND match_provider_field = ''::text
            AND match_syncer_id IS NULL
            AND match_syncer_field = ''::text)
        OR (match_type = 'system'::text
            AND match_local_field = ANY (ARRAY['username'::text, 'email'::text, 'phone'::text])
            AND match_provider_field <> ''::text
            AND match_syncer_id IS NULL
            AND match_syncer_field = ''::text)
        OR (match_type = 'identity_link'::text
            AND match_local_field = ''::text
            AND match_provider_field <> ''::text
            AND match_syncer_id IS NOT NULL
            AND match_syncer_field <> ''::text)
    ),
    CONSTRAINT provider_oauth_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_oauth_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT provider_oauth_match_syncer_fk FOREIGN KEY (match_syncer_id, institution_id) REFERENCES public.syncers(id, institution_id) ON DELETE RESTRICT
);

CREATE TABLE public.provider_sms (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    region_id text DEFAULT ''::text NOT NULL,
    sign_name text DEFAULT ''::text NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret text DEFAULT ''::text NOT NULL,
    endpoint text DEFAULT ''::text NOT NULL,
    receiver text DEFAULT ''::text NOT NULL,
    CONSTRAINT provider_sms_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_sms_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provider_storage (
    provider_id bigint NOT NULL,
    institution_id bigint NOT NULL,
    endpoint text DEFAULT ''::text NOT NULL,
    domain text DEFAULT ''::text NOT NULL,
    bucket text DEFAULT ''::text NOT NULL,
    path_prefix text DEFAULT ''::text NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret text DEFAULT ''::text NOT NULL,
    CONSTRAINT provider_storage_pkey PRIMARY KEY (provider_id),
    CONSTRAINT provider_storage_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE
);

CREATE TABLE public.provision_history (
    id bigint DEFAULT nextval('public.provision_history_id_seq'::regclass) NOT NULL,
    connector text DEFAULT ''::text NOT NULL,
    action text NOT NULL,
    status text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    application_id bigint,
    user_id bigint,
    CONSTRAINT provision_history_pkey PRIMARY KEY (id),
    CONSTRAINT provision_history_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE SET NULL,
    CONSTRAINT provision_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE TABLE public.records (
    id bigint DEFAULT nextval('public.records_id_seq'::regclass) NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    client_ip text DEFAULT ''::text NOT NULL,
    "user" text DEFAULT ''::text NOT NULL,
    method text DEFAULT ''::text NOT NULL,
    request_uri text DEFAULT ''::text NOT NULL,
    action text DEFAULT ''::text NOT NULL,
    language text DEFAULT ''::text NOT NULL,
    object text DEFAULT ''::text NOT NULL,
    response text DEFAULT ''::text NOT NULL,
    status_code integer DEFAULT 0 NOT NULL,
    detail text DEFAULT ''::text NOT NULL,
    is_triggered boolean DEFAULT false NOT NULL,
    trace_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    user_id bigint,
    CONSTRAINT records_pkey PRIMARY KEY (id),
    CONSTRAINT records_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE TABLE public.role_job_titles (
    role_id bigint NOT NULL,
    job_title_id bigint NOT NULL,
    CONSTRAINT role_job_titles_pkey PRIMARY KEY (role_id, job_title_id),
    CONSTRAINT role_groups_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE,
    CONSTRAINT role_job_titles_job_title_id_fkey FOREIGN KEY (job_title_id) REFERENCES public.job_titles(id) ON DELETE CASCADE
);

CREATE TABLE public.role_permissions (
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL,
    CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id),
    CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE,
    CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE
);

CREATE TABLE public.role_roles (
    role_id bigint NOT NULL,
    child_role_id bigint NOT NULL,
    CONSTRAINT role_roles_pkey PRIMARY KEY (role_id, child_role_id),
    CONSTRAINT role_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE,
    CONSTRAINT role_roles_child_role_id_fkey FOREIGN KEY (child_role_id) REFERENCES public.roles(id) ON DELETE CASCADE
);

CREATE TABLE public.role_users (
    role_id bigint NOT NULL,
    user_id bigint NOT NULL,
    CONSTRAINT role_users_pkey PRIMARY KEY (role_id, user_id),
    CONSTRAINT role_users_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE,
    CONSTRAINT role_users_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.storage_settings (
    id smallint DEFAULT 1 NOT NULL,
    type text DEFAULT ''::text NOT NULL,
    endpoint text DEFAULT ''::text NOT NULL,
    region text DEFAULT ''::text NOT NULL,
    bucket text DEFAULT ''::text NOT NULL,
    access_key text DEFAULT ''::text NOT NULL,
    secret_key_enc text DEFAULT ''::text NOT NULL,
    path_prefix text DEFAULT ''::text NOT NULL,
    domain text DEFAULT ''::text NOT NULL,
    root_path text DEFAULT ''::text NOT NULL,
    public_base text DEFAULT ''::text NOT NULL,
    path_style boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_settings_id_check CHECK ((id = 1)),
    CONSTRAINT storage_settings_pkey PRIMARY KEY (id)
);

CREATE TABLE public.syncer_department_links (
    id bigint DEFAULT nextval('public.syncer_department_links_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    syncer_id bigint NOT NULL,
    department_id bigint NOT NULL,
    external_id text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT syncer_department_links_department_key UNIQUE (syncer_id, department_id),
    CONSTRAINT syncer_department_links_key UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_department_links_pkey PRIMARY KEY (id),
    CONSTRAINT syncer_department_links_department_id_fkey FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE CASCADE,
    CONSTRAINT syncer_department_links_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT syncer_department_links_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE CASCADE
);

CREATE TABLE public.syncer_job_level_links (
    id bigint DEFAULT nextval('public.syncer_job_level_links_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    syncer_id bigint NOT NULL,
    job_level_id bigint NOT NULL,
    external_id text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT syncer_job_level_links_job_level_key UNIQUE (syncer_id, job_level_id),
    CONSTRAINT syncer_job_level_links_key UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_job_level_links_pkey PRIMARY KEY (id),
    CONSTRAINT syncer_job_level_links_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT syncer_job_level_links_job_level_id_fkey FOREIGN KEY (job_level_id) REFERENCES public.job_levels(id) ON DELETE CASCADE,
    CONSTRAINT syncer_job_level_links_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE CASCADE
);

CREATE TABLE public.syncer_job_title_links (
    id bigint DEFAULT nextval('public.syncer_job_title_links_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    syncer_id bigint NOT NULL,
    job_title_id bigint NOT NULL,
    external_id text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT syncer_job_title_links_job_title_key UNIQUE (syncer_id, job_title_id),
    CONSTRAINT syncer_job_title_links_key UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_job_title_links_pkey PRIMARY KEY (id),
    CONSTRAINT syncer_job_title_links_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT syncer_job_title_links_job_title_id_fkey FOREIGN KEY (job_title_id) REFERENCES public.job_titles(id) ON DELETE CASCADE,
    CONSTRAINT syncer_job_title_links_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE CASCADE
);

CREATE TABLE public.syncer_runs (
    id bigint DEFAULT nextval('public.syncer_runs_id_seq'::regclass) NOT NULL,
    syncer_id bigint NOT NULL,
    status text DEFAULT ''::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    counts jsonb,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    trigger text DEFAULT 'manual'::text NOT NULL,
    duration_ms integer DEFAULT 0 NOT NULL,
    errors jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT syncer_runs_pkey PRIMARY KEY (id),
    CONSTRAINT syncer_runs_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE CASCADE
);

CREATE TABLE public.syncer_user_links (
    id bigint DEFAULT nextval('public.syncer_user_links_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    syncer_id bigint NOT NULL,
    user_id bigint NOT NULL,
    external_id text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT syncer_user_links_key UNIQUE (syncer_id, external_id),
    CONSTRAINT syncer_user_links_pkey PRIMARY KEY (id),
    CONSTRAINT syncer_user_links_user_key UNIQUE (syncer_id, user_id),
    CONSTRAINT syncer_user_links_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT syncer_user_links_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE CASCADE,
    CONSTRAINT syncer_user_links_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.tokens (
    id bigint DEFAULT nextval('public.tokens_id_seq'::regclass) NOT NULL,
    name text NOT NULL,
    code_hash text DEFAULT ''::text NOT NULL,
    code_challenge text DEFAULT ''::text NOT NULL,
    code_challenge_method text DEFAULT ''::text NOT NULL,
    code_is_used boolean DEFAULT false NOT NULL,
    code_expire_in bigint DEFAULT 0 NOT NULL,
    code_scope text DEFAULT ''::text NOT NULL,
    code_redirect_uri text DEFAULT ''::text NOT NULL,
    access_token_hash text DEFAULT ''::text NOT NULL,
    refresh_token_hash text DEFAULT ''::text NOT NULL,
    id_token_hash text DEFAULT ''::text NOT NULL,
    expires_in integer DEFAULT 0 NOT NULL,
    scope text DEFAULT ''::text NOT NULL,
    token_type text DEFAULT 'Bearer'::text NOT NULL,
    grant_type text DEFAULT ''::text NOT NULL,
    resource text DEFAULT ''::text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    expire_time timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    application_id bigint,
    user_id bigint,
    code_nonce text DEFAULT ''::text NOT NULL,
    CONSTRAINT tokens_pkey PRIMARY KEY (id),
    CONSTRAINT tokens_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.applications(id) ON DELETE SET NULL,
    CONSTRAINT tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE TABLE public.user_departments (
    id bigint DEFAULT nextval('public.user_departments_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    user_id bigint NOT NULL,
    department_id bigint NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    syncer_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_departments_key UNIQUE (user_id, department_id),
    CONSTRAINT user_departments_pkey PRIMARY KEY (id),
    CONSTRAINT user_departments_department_id_fkey FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE CASCADE,
    CONSTRAINT user_departments_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT user_departments_syncer_id_fkey FOREIGN KEY (syncer_id) REFERENCES public.syncers(id) ON DELETE SET NULL,
    CONSTRAINT user_departments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.user_identities (
    id bigint DEFAULT nextval('public.user_identities_id_seq'::regclass) NOT NULL,
    provider_user_id text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    -- 提供商侧原始身份字段（键名与 providerMatchFields 一致，如 unionid/openid/sub/email/mobile/…）。
    -- 供"同源身份连接"反向认领使用：同步侧按连接源字段值在 attributes 里查同一人。
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    institution_id bigint,
    user_id bigint NOT NULL,
    provider_id bigint,
    CONSTRAINT user_identities_inst_provider_key UNIQUE (institution_id, provider_id, provider_user_id),
    CONSTRAINT user_identities_pkey PRIMARY KEY (id),
    CONSTRAINT user_identities_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.providers(id) ON DELETE SET NULL,
    CONSTRAINT user_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.user_mfa_credentials (
    id bigint DEFAULT nextval('public.user_mfa_credentials_id_seq'::regclass) NOT NULL,
    institution_id bigint NOT NULL,
    user_id bigint NOT NULL,
    provider_id bigint NOT NULL,
    type text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    secret_encrypted text DEFAULT ''::text NOT NULL,
    recovery_codes_hashed text[] DEFAULT '{}'::text[] NOT NULL,
    phone text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_mfa_credentials_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'active'::text, 'disabled'::text]))),
    CONSTRAINT user_mfa_credentials_type_check CHECK ((type = ANY (ARRAY['totp'::text, 'sms'::text]))),
    CONSTRAINT user_mfa_credentials_key UNIQUE (user_id, provider_id),
    CONSTRAINT user_mfa_credentials_pkey PRIMARY KEY (id),
    CONSTRAINT user_mfa_credentials_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE,
    CONSTRAINT user_mfa_credentials_provider_fk FOREIGN KEY (provider_id, institution_id) REFERENCES public.providers(id, institution_id) ON DELETE CASCADE,
    CONSTRAINT user_mfa_credentials_user_fk FOREIGN KEY (user_id, institution_id) REFERENCES public.users(id, institution_id) ON DELETE CASCADE
);

--
-- Sequence ownership
--

ALTER SEQUENCE public.account_strategies_id_seq OWNED BY public.account_strategies.id;

ALTER SEQUENCE public.application_accounts_id_seq OWNED BY public.application_accounts.id;

ALTER SEQUENCE public.application_subjects_id_seq OWNED BY public.application_subjects.id;

ALTER SEQUENCE public.applications_id_seq OWNED BY public.applications.id;

ALTER SEQUENCE public.asyncd_job_run_attempts_id_seq OWNED BY public.asyncd_job_run_attempts.id;

ALTER SEQUENCE public.asyncd_schedules_id_seq OWNED BY public.asyncd_jobs.id;

ALTER SEQUENCE public.asyncd_tasks_id_seq OWNED BY public.asyncd_job_runs.id;

ALTER SEQUENCE public.certs_id_seq OWNED BY public.certs.id;

ALTER SEQUENCE public.connectors_id_seq OWNED BY public.connectors.id;

ALTER SEQUENCE public.institutions_id_seq OWNED BY public.institutions.id;

ALTER SEQUENCE public.job_levels_id_seq OWNED BY public.job_levels.id;

ALTER SEQUENCE public.job_titles_id_seq OWNED BY public.job_titles.id;

ALTER SEQUENCE public.organizations_id_seq OWNED BY public.departments.id;

ALTER SEQUENCE public.password_history_id_seq OWNED BY public.password_history.id;

ALTER SEQUENCE public.permissions_id_seq OWNED BY public.permissions.id;

ALTER SEQUENCE public.provider_email_templates_id_seq OWNED BY public.provider_email_templates.id;

ALTER SEQUENCE public.provider_sms_templates_id_seq OWNED BY public.provider_sms_templates.id;

ALTER SEQUENCE public.providers_id_seq OWNED BY public.providers.id;

ALTER SEQUENCE public.provision_history_id_seq OWNED BY public.provision_history.id;

ALTER SEQUENCE public.records_id_seq OWNED BY public.records.id;

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;

ALTER SEQUENCE public.syncer_department_links_id_seq OWNED BY public.syncer_department_links.id;

ALTER SEQUENCE public.syncer_job_level_links_id_seq OWNED BY public.syncer_job_level_links.id;

ALTER SEQUENCE public.syncer_job_title_links_id_seq OWNED BY public.syncer_job_title_links.id;

ALTER SEQUENCE public.syncer_runs_id_seq OWNED BY public.syncer_runs.id;

ALTER SEQUENCE public.syncer_user_links_id_seq OWNED BY public.syncer_user_links.id;

ALTER SEQUENCE public.syncers_id_seq OWNED BY public.syncers.id;

ALTER SEQUENCE public.tokens_id_seq OWNED BY public.tokens.id;

ALTER SEQUENCE public.user_departments_id_seq OWNED BY public.user_departments.id;

ALTER SEQUENCE public.user_identities_id_seq OWNED BY public.user_identities.id;

ALTER SEQUENCE public.user_mfa_credentials_id_seq OWNED BY public.user_mfa_credentials.id;

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;

--
-- Indexes
--

CREATE INDEX permission_job_titles_job_title_idx ON public.permission_job_titles USING btree (job_title_id);

CREATE INDEX permission_roles_role_idx ON public.permission_roles USING btree (role_id);

CREATE INDEX role_job_titles_job_title_idx ON public.role_job_titles USING btree (job_title_id);

CREATE INDEX role_permissions_permission_idx ON public.role_permissions USING btree (permission_id);

CREATE INDEX role_roles_child_role_idx ON public.role_roles USING btree (child_role_id);

CREATE INDEX account_strategies_application_idx ON public.account_strategies USING btree (institution_id, application_id);

CREATE INDEX account_strategies_institution_id_idx ON public.account_strategies USING btree (institution_id);

CREATE INDEX application_accounts_institution_id_idx ON public.application_accounts USING btree (institution_id);

CREATE INDEX application_accounts_user_idx ON public.application_accounts USING btree (institution_id, user_id);

CREATE INDEX application_subjects_app_idx ON public.application_subjects USING btree (application_id, subject_type);

CREATE UNIQUE INDEX application_subjects_department_idx ON public.application_subjects USING btree (application_id, department_id) WHERE (department_id IS NOT NULL);

CREATE INDEX application_subjects_department_lookup_idx ON public.application_subjects USING btree (department_id) WHERE (department_id IS NOT NULL);

CREATE UNIQUE INDEX application_subjects_job_title_idx ON public.application_subjects USING btree (application_id, job_title_id) WHERE (job_title_id IS NOT NULL);

CREATE INDEX application_subjects_job_title_lookup_idx ON public.application_subjects USING btree (job_title_id) WHERE (job_title_id IS NOT NULL);

CREATE UNIQUE INDEX application_subjects_user_idx ON public.application_subjects USING btree (application_id, user_id) WHERE (user_id IS NOT NULL);

CREATE INDEX application_subjects_user_lookup_idx ON public.application_subjects USING btree (user_id) WHERE (user_id IS NOT NULL);

CREATE UNIQUE INDEX applications_client_id_key ON public.applications USING btree (client_id) WHERE (client_id <> ''::text);

CREATE INDEX applications_institution_id_idx ON public.applications USING btree (institution_id);

CREATE INDEX asyncd_job_run_attempts_job_run_idx ON public.asyncd_job_run_attempts USING btree (job_run_id, attempt_no);

CREATE INDEX asyncd_job_run_attempts_open_idx ON public.asyncd_job_run_attempts USING btree (started_at) WHERE (status = 'running'::text);

CREATE INDEX asyncd_job_runs_finished_idx ON public.asyncd_job_runs USING btree (finished_at) WHERE (finished_at IS NOT NULL);

CREATE INDEX asyncd_job_runs_job_id_idx ON public.asyncd_job_runs USING btree (job_id, status, id DESC);

CREATE INDEX asyncd_job_runs_lease_idx ON public.asyncd_job_runs USING btree (lease_expires_at) WHERE (status = 'running'::text);

CREATE INDEX asyncd_job_runs_ready_idx ON public.asyncd_job_runs USING btree (priority DESC, next_attempt_at) WHERE (status = 'queued'::text);

CREATE UNIQUE INDEX asyncd_job_runs_unique_key_idx ON public.asyncd_job_runs USING btree (unique_key) WHERE ((unique_key <> ''::text) AND (status = ANY (ARRAY['queued'::text, 'running'::text])));

CREATE INDEX asyncd_job_runs_worker_idx ON public.asyncd_job_runs USING btree (worker_id, status) WHERE (worker_id <> ''::text);

CREATE INDEX asyncd_jobs_due_idx ON public.asyncd_jobs USING btree (next_run_at) WHERE (enabled AND (kind = 'scheduled'::text) AND (retired_at IS NULL));

CREATE INDEX asyncd_workers_heartbeat_idx ON public.asyncd_workers USING btree (last_heartbeat_at DESC);

CREATE INDEX certs_institution_id_idx ON public.certs USING btree (institution_id);

CREATE INDEX connectors_application_idx ON public.connectors USING btree (institution_id, application_id);

CREATE INDEX connectors_institution_id_idx ON public.connectors USING btree (institution_id);

CREATE INDEX departments_institution_id_idx ON public.departments USING btree (institution_id);

CREATE INDEX departments_parent_idx ON public.departments USING btree (parent_id);

CREATE UNIQUE INDEX departments_root_key ON public.departments USING btree (institution_id) WHERE is_root;

CREATE UNIQUE INDEX institutions_domain_key ON public.institutions USING btree (lower(domain)) WHERE (domain <> ''::text);

CREATE INDEX job_levels_inst_idx ON public.job_levels USING btree (institution_id, sort_order);

CREATE INDEX job_title_members_user_idx ON public.job_title_members USING btree (user_id);

CREATE INDEX job_titles_institution_id_idx ON public.job_titles USING btree (institution_id);

CREATE INDEX password_history_institution_id_idx ON public.password_history USING btree (institution_id);

CREATE INDEX password_history_user_idx ON public.password_history USING btree (user_id, created_at DESC);

CREATE INDEX permission_users_user_idx ON public.permission_users USING btree (user_id);

CREATE INDEX permissions_institution_id_idx ON public.permissions USING btree (institution_id);

CREATE UNIQUE INDEX provider_email_templates_default_key ON public.provider_email_templates USING btree (provider_id, scene) WHERE is_default;

CREATE INDEX provider_email_templates_scene_idx ON public.provider_email_templates USING btree (institution_id, provider_id, scene);

CREATE UNIQUE INDEX provider_sms_templates_default_key ON public.provider_sms_templates USING btree (provider_id, scene) WHERE is_default;

CREATE INDEX provider_sms_templates_scene_idx ON public.provider_sms_templates USING btree (institution_id, provider_id, scene);

CREATE INDEX providers_institution_category_idx ON public.providers USING btree (institution_id, category);

CREATE INDEX providers_institution_id_idx ON public.providers USING btree (institution_id);

CREATE UNIQUE INDEX providers_singleton_active_key ON public.providers USING btree (institution_id, category) WHERE ((state = 'active'::text) AND (category = ANY (ARRAY['captcha'::text, 'storage'::text])));

CREATE INDEX provision_history_created_idx ON public.provision_history USING btree (created_at DESC);

CREATE INDEX provision_history_institution_id_idx ON public.provision_history USING btree (institution_id);

CREATE INDEX provision_history_user_idx ON public.provision_history USING btree (institution_id, user_id);

CREATE INDEX records_created_idx ON public.records USING btree (created_at DESC);

CREATE INDEX records_institution_id_idx ON public.records USING btree (institution_id);

CREATE INDEX records_user_id_idx ON public.records USING btree (institution_id, user_id);

CREATE INDEX role_users_user_idx ON public.role_users USING btree (user_id);

CREATE INDEX roles_institution_id_idx ON public.roles USING btree (institution_id);

CREATE INDEX syncer_department_links_dept_idx ON public.syncer_department_links USING btree (institution_id, department_id);

CREATE INDEX syncer_job_level_links_level_idx ON public.syncer_job_level_links USING btree (institution_id, job_level_id);

CREATE INDEX syncer_job_title_links_job_idx ON public.syncer_job_title_links USING btree (institution_id, job_title_id);

CREATE INDEX syncer_runs_institution_id_idx ON public.syncer_runs USING btree (institution_id);

CREATE INDEX syncer_runs_syncer_idx ON public.syncer_runs USING btree (syncer_id, started_at DESC);

CREATE INDEX syncer_runs_syncer_started_idx ON public.syncer_runs USING btree (syncer_id, started_at DESC);

CREATE INDEX syncer_user_links_user_idx ON public.syncer_user_links USING btree (institution_id, user_id);

CREATE INDEX syncers_institution_id_idx ON public.syncers USING btree (institution_id);

CREATE UNIQUE INDEX syncers_schedule_job_key ON public.syncers USING btree (schedule_job_id) WHERE (schedule_job_id IS NOT NULL);

CREATE UNIQUE INDEX connectors_schedule_job_key ON public.connectors USING btree (schedule_job_id) WHERE (schedule_job_id IS NOT NULL);

CREATE INDEX provider_oauth_match_syncer_idx ON public.provider_oauth USING btree (match_syncer_id) WHERE (match_syncer_id IS NOT NULL);

CREATE UNIQUE INDEX tokens_access_hash_key ON public.tokens USING btree (access_token_hash) WHERE (access_token_hash <> ''::text);

CREATE INDEX tokens_app_user_idx ON public.tokens USING btree (institution_id, application_id, user_id);

CREATE UNIQUE INDEX tokens_code_hash_key ON public.tokens USING btree (code_hash) WHERE (code_hash <> ''::text);

CREATE INDEX tokens_institution_id_idx ON public.tokens USING btree (institution_id);

CREATE UNIQUE INDEX tokens_refresh_hash_key ON public.tokens USING btree (refresh_token_hash) WHERE (refresh_token_hash <> ''::text);

CREATE INDEX user_departments_dept_idx ON public.user_departments USING btree (institution_id, department_id);

CREATE UNIQUE INDEX user_departments_primary_key ON public.user_departments USING btree (user_id) WHERE is_primary;

CREATE INDEX user_identities_inst_user_idx ON public.user_identities USING btree (institution_id, user_id);

CREATE INDEX user_identities_institution_id_idx ON public.user_identities USING btree (institution_id);

CREATE INDEX user_identities_user_idx ON public.user_identities USING btree (user_id);

CREATE INDEX user_identities_provider_idx ON public.user_identities USING btree (institution_id, provider_id);

CREATE INDEX user_identities_attributes_idx ON public.user_identities USING gin (attributes);

CREATE INDEX user_mfa_credentials_provider_idx ON public.user_mfa_credentials USING btree (provider_id);

CREATE INDEX user_mfa_credentials_user_idx ON public.user_mfa_credentials USING btree (institution_id, user_id, status);

CREATE INDEX users_email_idx ON public.users USING btree (email);

CREATE UNIQUE INDEX users_inst_email_key ON public.users USING btree (institution_id, lower(email)) WHERE (email <> ''::text);

CREATE UNIQUE INDEX users_inst_phone_key ON public.users USING btree (institution_id, phone) WHERE (phone <> ''::text);

CREATE INDEX users_institution_flags_idx ON public.users USING btree (institution_id, is_deleted, is_forbidden);

CREATE INDEX users_institution_id_idx ON public.users USING btree (institution_id);

CREATE INDEX users_job_level_idx ON public.users USING btree (institution_id, job_level_id);

CREATE UNIQUE INDEX users_uid_key ON public.users USING btree (uid) WHERE (uid <> ''::text);

--
-- PostgreSQL database dump complete
--
