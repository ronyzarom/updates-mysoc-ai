-- 019 down: drop issuer sealing. Not needed to roll the binary back to 1.16.1
-- (it ignores these columns); run only to remove the schema entirely.
ALTER TABLE releases DROP CONSTRAINT IF EXISTS releases_seal_status_check;
ALTER TABLE releases DROP COLUMN IF EXISTS issuer_signature;
ALTER TABLE releases DROP COLUMN IF EXISTS issuer_key_id;
ALTER TABLE releases DROP COLUMN IF EXISTS issuer;
ALTER TABLE releases DROP COLUMN IF EXISTS seal_status;
DROP TABLE IF EXISTS trusted_keys;
DELETE FROM schema_migrations WHERE version = '019';
