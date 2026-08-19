-- Migration 011: license ownership (operator / reseller / customer).
--
-- Sales reality: MySoc sells the mysoc platform to a SOC operator. The
-- operator sells siemcore+swf to end customers, either directly or through a
-- reseller. One mysoc installation (the operator's) serves many customers, so
-- ownership is license metadata, not an instance tier:
--
--   operator_id    business id of the SOC operator this license belongs to.
--                  On the operator's own platform license it equals the
--                  license's customer_id. NULL on legacy rows ("Unassigned").
--   reseller_id    optional channel id when the customer was sold through a
--                  reseller; NULL for direct sales. Resellers are metadata
--                  only - they deploy no software and have no instances.
--   reseller_name  human-friendly reseller label for the dashboard.

ALTER TABLE licenses ADD COLUMN IF NOT EXISTS operator_id VARCHAR(100);
ALTER TABLE licenses ADD COLUMN IF NOT EXISTS reseller_id VARCHAR(100);
ALTER TABLE licenses ADD COLUMN IF NOT EXISTS reseller_name VARCHAR(255);

-- Group a fleet's licenses by operator when assembling the instance tree.
CREATE INDEX IF NOT EXISTS idx_licenses_operator_id ON licenses(operator_id);
