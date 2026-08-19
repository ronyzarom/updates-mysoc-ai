-- 012: Operator entities and cascaded fleet reporting (1.8.0)
--
-- Operators become first-class entities. Each operator holds one platform
-- license key; siemcore/swf credentials are parent-issued inside the cascade
-- and never touch this database. Instances gain cascade provenance columns.

CREATE TABLE IF NOT EXISTS operators (
    id         VARCHAR(100) PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- product: which tier this key authorizes (mysoc for platform keys).
-- operator_ref: owning operator. Both NULL on pre-1.8.0 (legacy) licenses.
ALTER TABLE licenses ADD COLUMN IF NOT EXISTS product VARCHAR(20);
ALTER TABLE licenses ADD COLUMN IF NOT EXISTS operator_ref VARCHAR(100) REFERENCES operators(id);
CREATE INDEX IF NOT EXISTS idx_licenses_operator_ref ON licenses(operator_ref);

-- customer_id: end-customer this node belongs to, as reported up the cascade.
-- reported_via: instance_id of the relay that reported this node (NULL = the
-- node heartbeats to this server directly).
-- reported_at: when the covering rollup was received.
ALTER TABLE instances ADD COLUMN IF NOT EXISTS customer_id VARCHAR(100);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS customer_name VARCHAR(255);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS reported_via VARCHAR(100);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS reported_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX IF NOT EXISTS idx_instances_customer_id ON instances(customer_id);
CREATE INDEX IF NOT EXISTS idx_instances_reported_via ON instances(reported_via);

-- Backfill: every pre-existing platform (mysoc-cloud) license becomes an
-- operator; its license row is attached as the operator's platform key.
INSERT INTO operators (id, name, is_active)
SELECT DISTINCT COALESCE(NULLIF(operator_id, ''), customer_id), customer_name, is_active
FROM licenses
WHERE license_type = 'mysoc-cloud'
ON CONFLICT (id) DO NOTHING;

UPDATE licenses
SET product = 'mysoc',
    operator_ref = COALESCE(NULLIF(operator_id, ''), customer_id)
WHERE license_type = 'mysoc-cloud' AND product IS NULL;

-- Legacy customer licenses that already point at a known operator get attached
-- (product stays NULL: they remain legacy multi-tier keys until rotated).
UPDATE licenses
SET operator_ref = operator_id
WHERE operator_ref IS NULL
  AND operator_id IS NOT NULL
  AND operator_id <> ''
  AND EXISTS (SELECT 1 FROM operators o WHERE o.id = licenses.operator_id);
