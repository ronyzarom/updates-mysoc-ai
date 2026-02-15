-- Migration 006 rollback: Remove instance tracking columns

-- Remove index
DROP INDEX IF EXISTS idx_instances_last_ip_address;

-- Remove update attempt tracking columns
ALTER TABLE instances DROP COLUMN IF EXISTS last_update_at;
ALTER TABLE instances DROP COLUMN IF EXISTS last_update_error;
ALTER TABLE instances DROP COLUMN IF EXISTS last_update_success;
ALTER TABLE instances DROP COLUMN IF EXISTS last_update_target_version;
ALTER TABLE instances DROP COLUMN IF EXISTS last_update_from_version;

-- Remove IP address tracking
ALTER TABLE instances DROP COLUMN IF EXISTS last_ip_seen_at;
ALTER TABLE instances DROP COLUMN IF EXISTS last_ip_address;
