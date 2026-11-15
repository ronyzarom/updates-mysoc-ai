-- Refuse rollback rather than silently stripping an active credential's
-- authorization: if any key still uses the new scope or a tier binding, the
-- down migration would leave those keys either unparsable (scope check) or
-- silently widened (dropping product_tier), so fail loudly instead.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM api_keys WHERE scope = 'update-group' OR product_tier IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot roll back 020: api_keys still use scope=update-group or a product_tier binding';
    END IF;
END $$;

ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_scope_check;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_scope_check
    CHECK (scope IN ('releases', 'admin'));

ALTER TABLE api_keys DROP COLUMN IF EXISTS product_tier;
