-- 014 down: drop the fleet-scale indexes (leave the pg_trgm extension in place).

DROP INDEX IF EXISTS idx_instances_product_tier;
DROP INDEX IF EXISTS idx_instances_created_at;
DROP INDEX IF EXISTS idx_instances_last_heartbeat;
DROP INDEX IF EXISTS idx_instances_failed_update;
DROP INDEX IF EXISTS idx_instances_instance_id_trgm;
DROP INDEX IF EXISTS idx_instances_hostname_trgm;
DROP INDEX IF EXISTS idx_instances_customer_name_trgm;
