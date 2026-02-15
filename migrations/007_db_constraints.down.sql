-- Migration 007 rollback: Remove constraints

-- Remove CHECK constraint
ALTER TABLE instances DROP CONSTRAINT IF EXISTS valid_update_group;

-- Remove NOT NULL constraints (allow NULLs again)
ALTER TABLE instances ALTER COLUMN update_group DROP NOT NULL;
ALTER TABLE instances ALTER COLUMN update_group DROP DEFAULT;

ALTER TABLE instances ALTER COLUMN auto_update_enabled DROP NOT NULL;
ALTER TABLE instances ALTER COLUMN auto_update_enabled DROP DEFAULT;
