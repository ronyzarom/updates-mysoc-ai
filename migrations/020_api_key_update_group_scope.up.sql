-- Migration 020: scoped self-service ring control.
--
-- Adds a new least-privilege scope 'update-group' that authorizes ONLY
-- PUT /instances/{id}/update-group (moving a node between rings), plus an
-- optional product_tier binding so a key can be constrained to instances of a
-- single tier. Together these let an operator (e.g. SiemCore) hold ONE key that
-- can set the ring of only their own nodes, with no other admin power and no
-- reach into another tier's fleet.
--
--   scope='update-group' + product_tier='siemcore'
--     -> may set update_group ONLY for instances whose product_tier is siemcore
--
-- An unbound key (product_tier NULL) with scope='update-group' may set the ring
-- for any instance; 'admin' still covers everything; 'releases' does NOT cover
-- update-group (orthogonal least-privilege surfaces).

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS product_tier VARCHAR(40);

ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_scope_check;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_scope_check
    CHECK (scope IN ('releases', 'admin', 'update-group'));
