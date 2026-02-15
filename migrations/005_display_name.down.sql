-- Remove display_name column from instances
ALTER TABLE instances DROP COLUMN IF EXISTS display_name;
