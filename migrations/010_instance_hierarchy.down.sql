-- Rollback migration 010.
DROP INDEX IF EXISTS idx_instances_license_tier;
DROP INDEX IF EXISTS idx_instances_parent_instance_id;
ALTER TABLE instances DROP COLUMN IF EXISTS parent_instance_id;
ALTER TABLE instances DROP COLUMN IF EXISTS product_tier;
