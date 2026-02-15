-- Add auto_update_enabled column to instances
ALTER TABLE instances ADD COLUMN IF NOT EXISTS auto_update_enabled BOOLEAN DEFAULT true;

-- Add index for querying by auto_update status
CREATE INDEX IF NOT EXISTS idx_instances_auto_update ON instances(auto_update_enabled);
