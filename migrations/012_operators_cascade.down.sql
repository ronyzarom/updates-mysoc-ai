-- Rollback 012: drop cascade provenance and operator entities.

DROP INDEX IF EXISTS idx_instances_reported_via;
DROP INDEX IF EXISTS idx_instances_customer_id;
ALTER TABLE instances DROP COLUMN IF EXISTS reported_at;
ALTER TABLE instances DROP COLUMN IF EXISTS reported_via;
ALTER TABLE instances DROP COLUMN IF EXISTS customer_name;
ALTER TABLE instances DROP COLUMN IF EXISTS customer_id;

DROP INDEX IF EXISTS idx_licenses_operator_ref;
ALTER TABLE licenses DROP COLUMN IF EXISTS operator_ref;
ALTER TABLE licenses DROP COLUMN IF EXISTS product;

DROP TABLE IF EXISTS operators;
