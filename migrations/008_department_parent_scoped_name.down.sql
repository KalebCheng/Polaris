DROP INDEX IF EXISTS public.departments_root_name_key;
DROP INDEX IF EXISTS public.departments_parent_name_key;

ALTER TABLE public.departments
    ADD CONSTRAINT departments_inst_name_key UNIQUE (institution_id, name);
