-- 用户导入记录（控制台「导入用户」的可回查历史）。
-- 不保存原始文件与密码：只记录每次导入的汇总与逐行状态/错误。

CREATE TABLE IF NOT EXISTS public.user_imports (
    id BIGSERIAL,
    institution_id bigint NOT NULL,
    file_name text NOT NULL,
    file_format text NOT NULL,
    status text NOT NULL,
    total_rows integer DEFAULT 0 NOT NULL,
    created_count integer DEFAULT 0 NOT NULL,
    skipped_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    auto_create_departments boolean DEFAULT false NOT NULL,
    operator text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_imports_status_check CHECK (status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'partial'::text, 'failed'::text])),
    CONSTRAINT user_imports_file_format_check CHECK (file_format = ANY (ARRAY['xlsx'::text, 'csv'::text])),
    CONSTRAINT user_imports_pkey PRIMARY KEY (id),
    CONSTRAINT user_imports_institution_id_fkey FOREIGN KEY (institution_id) REFERENCES public.institutions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS user_imports_inst_created_idx ON public.user_imports (institution_id, created_at DESC);

CREATE TABLE IF NOT EXISTS public.user_import_rows (
    id bigserial NOT NULL,
    import_id bigint NOT NULL,
    row_number integer NOT NULL,
    user_name text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    status text NOT NULL,
    created_user_id bigint,
    errors jsonb DEFAULT '[]'::jsonb NOT NULL,
    warnings jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_import_rows_status_check CHECK (status = ANY (ARRAY['created'::text, 'skipped'::text, 'failed'::text])),
    CONSTRAINT user_import_rows_pkey PRIMARY KEY (id),
    CONSTRAINT user_import_rows_import_id_fkey FOREIGN KEY (import_id) REFERENCES public.user_imports(id) ON DELETE CASCADE,
    CONSTRAINT user_import_rows_created_user_id_fkey FOREIGN KEY (created_user_id) REFERENCES public.users(id) ON DELETE SET NULL,
    CONSTRAINT user_import_rows_import_row_key UNIQUE (import_id, row_number)
);
