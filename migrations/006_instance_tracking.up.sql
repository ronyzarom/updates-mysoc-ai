-- Migration 006: Add instance tracking columns (IP address and update attempt tracking)

-- Add IP address tracking
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_ip_address TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_ip_seen_at TIMESTAMPTZ;

-- Add update attempt tracking columns
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_update_from_version TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_update_target_version TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_update_success BOOLEAN;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_update_error TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS last_update_at TIMESTAMPTZ;

-- Add index for IP address (useful for security auditing)
CREATE INDEX IF NOT EXISTS idx_instances_last_ip_address ON instances(last_ip_address);
