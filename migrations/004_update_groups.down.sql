-- Remove update_group from instances
DROP INDEX IF EXISTS idx_instances_update_group;
ALTER TABLE instances DROP COLUMN IF EXISTS update_group;
ALTER TABLE releases DROP COLUMN IF EXISTS target_groups;
