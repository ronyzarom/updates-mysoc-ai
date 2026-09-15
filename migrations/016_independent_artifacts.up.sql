-- Preserve existing rows and the legacy identity; independent kinds get distinct identities.
ALTER TABLE releases DROP CONSTRAINT IF EXISTS releases_product_name_version_key;
CREATE UNIQUE INDEX IF NOT EXISTS releases_product_version_kind_key
ON releases(product_name, version, (COALESCE(manifest->>'artifact_kind', '')));
