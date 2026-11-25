-- Department names are unique only within the same parent. Different parents
-- may legitimately contain departments with the same name (for example,
-- "市场部" under two regional branches).
ALTER TABLE public.departments DROP CONSTRAINT IF EXISTS departments_inst_name_key;

CREATE UNIQUE INDEX departments_parent_name_key
    ON public.departments (institution_id, parent_id, name)
    WHERE parent_id IS NOT NULL;

-- The organization root has parent_id = NULL. Keep it unique by name as well
-- so legacy data without a root row cannot introduce duplicate top-level rows
-- through this gap.
CREATE UNIQUE INDEX departments_root_name_key
    ON public.departments (institution_id, name)
    WHERE parent_id IS NULL;
