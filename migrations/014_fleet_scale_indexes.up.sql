-- 014: indexes backing the fleet-scale paged/filtered instance views (1.12.0)
--
-- The paged dashboard queries (internal/server/licensing/instances.go
-- ListPagedFiltered / FleetStatsSummary) filter by product_tier, customer_id,
-- and operator; sort by created_at / last_heartbeat; count failed updates; and
-- search instance_id / hostname / customer_name. These indexes keep those
-- queries O(page) rather than O(fleet) at 100k-500k rows.

-- Filter/group by tier on its own (idx_instances_license_tier is composite and
-- does not serve a bare product_tier predicate well).
CREATE INDEX IF NOT EXISTS idx_instances_product_tier ON instances(product_tier);

-- Default sort key.
CREATE INDEX IF NOT EXISTS idx_instances_created_at ON instances(created_at DESC);

-- Freshness sort / offline-derivation inputs.
CREATE INDEX IF NOT EXISTS idx_instances_last_heartbeat ON instances(last_heartbeat DESC);

-- Failed-update exceptions surface first; a partial index keeps it tiny.
CREATE INDEX IF NOT EXISTS idx_instances_failed_update
    ON instances(updated_at DESC)
    WHERE last_update_success IS FALSE;

-- Text search across instance_id / hostname / customer_name uses trigram GIN
-- indexes. pg_trgm may be unavailable or require privileges we lack on managed
-- Postgres, so attempt it best-effort: without the extension the queries still
-- work (sequential ILIKE), they are just not index-accelerated.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
    CREATE INDEX IF NOT EXISTS idx_instances_instance_id_trgm
        ON instances USING gin (instance_id gin_trgm_ops);
    CREATE INDEX IF NOT EXISTS idx_instances_hostname_trgm
        ON instances USING gin (hostname gin_trgm_ops);
    CREATE INDEX IF NOT EXISTS idx_instances_customer_name_trgm
        ON instances USING gin (customer_name gin_trgm_ops);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm unavailable (%); text search falls back to sequential ILIKE', SQLERRM;
END
$$;
