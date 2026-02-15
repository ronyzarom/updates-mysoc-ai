-- Remove auto_update_enabled column from instances
DROP INDEX IF EXISTS idx_instances_auto_update;
ALTER TABLE instances DROP COLUMN IF EXISTS auto_update_enabled;
