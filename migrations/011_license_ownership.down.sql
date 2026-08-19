-- Rollback migration 011.
DROP INDEX IF EXISTS idx_licenses_operator_id;
ALTER TABLE licenses DROP COLUMN IF EXISTS reseller_name;
ALTER TABLE licenses DROP COLUMN IF EXISTS reseller_id;
ALTER TABLE licenses DROP COLUMN IF EXISTS operator_id;
