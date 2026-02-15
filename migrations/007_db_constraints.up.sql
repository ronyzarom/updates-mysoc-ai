-- Migration 007: Add NOT NULL constraints and CHECK constraint for update_group

-- Backfill existing NULLs before adding constraints
UPDATE instances SET auto_update_enabled = true WHERE auto_update_enabled IS NULL;
UPDATE instances SET update_group = 'stable' WHERE update_group IS NULL OR update_group = '';

-- Add NOT NULL constraints with defaults
ALTER TABLE instances ALTER COLUMN auto_update_enabled SET DEFAULT true;
ALTER TABLE instances ALTER COLUMN auto_update_enabled SET NOT NULL;

ALTER TABLE instances ALTER COLUMN update_group SET DEFAULT 'stable';
ALTER TABLE instances ALTER COLUMN update_group SET NOT NULL;

-- Add CHECK constraint for valid update groups
ALTER TABLE instances ADD CONSTRAINT valid_update_group 
    CHECK (update_group IN ('alpha', 'beta', 'stable', 'production'));
