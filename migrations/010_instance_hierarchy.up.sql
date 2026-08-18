-- Migration 010: product-tier hierarchy for instances.
--
-- A customer owns one license that spans a three-tier tree:
--   mysoc (root) -> siemcore (many) -> swf (many).
-- Agents self-report their tier and their parent's instance_id on heartbeat; the
-- server binds the shared license and stores the parent link so the fleet can be
-- rendered as a per-customer tree.
--
--   product_tier        one of: mysoc | siemcore | swf (nullable until an agent
--                       reports it; legacy rows stay NULL).
--   parent_instance_id  the instance_id of this node's parent (a siemcore for an
--                       swf, a mysoc for a siemcore). NULL/empty for a mysoc root
--                       or an orphan whose parent has not enrolled yet.

ALTER TABLE instances ADD COLUMN IF NOT EXISTS product_tier VARCHAR(20);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS parent_instance_id VARCHAR(100);

-- Fast child lookups when assembling the tree.
CREATE INDEX IF NOT EXISTS idx_instances_parent_instance_id ON instances(parent_instance_id);
-- Group a customer's fleet by license and tier.
CREATE INDEX IF NOT EXISTS idx_instances_license_tier ON instances(license_id, product_tier);
