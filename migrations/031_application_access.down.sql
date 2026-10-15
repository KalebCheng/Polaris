DROP TABLE IF EXISTS application_subjects;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_access_mode_check;
ALTER TABLE applications DROP COLUMN IF EXISTS access_mode;
