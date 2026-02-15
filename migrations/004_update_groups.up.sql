-- Add update_group column to instances
ALTER TABLE instances ADD COLUMN IF NOT EXISTS update_group VARCHAR(50) DEFAULT 'stable';

-- Add target_groups column to releases (which groups can receive this release)
ALTER TABLE releases ADD COLUMN IF NOT EXISTS target_groups TEXT[] DEFAULT '{alpha,beta,stable,production}';

-- Add index for querying by update_group
CREATE INDEX IF NOT EXISTS idx_instances_update_group ON instances(update_group);
