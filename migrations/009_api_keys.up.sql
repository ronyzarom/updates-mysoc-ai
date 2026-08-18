-- Migration 009: managed API keys for admin/upload automation.
--
-- These are dashboard-managed credentials that authorize the admin/management
-- API (the same surface the static ADMIN_API_KEY env var covers), but each key
-- is individually named, scoped, revocable, and optionally expiring. Handing a
-- scoped key (e.g. scope='releases') to an external team lets them upload
-- releases without receiving the master admin key.
--
-- Only the SHA-256 hash of the full key is stored; the plaintext is shown once
-- at creation and never persisted. key_prefix keeps a short, non-sensitive hint
-- (e.g. msk_1a2b3c4d) for display in the UI.
--
--   scope='releases' -> release management only (upload/update/delete releases)
--   scope='admin'    -> full admin surface (same power as ADMIN_API_KEY)

CREATE TABLE IF NOT EXISTS api_keys (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name         VARCHAR(120) NOT NULL,
    key_hash     VARCHAR(64) UNIQUE NOT NULL,
    key_prefix   VARCHAR(24) NOT NULL,
    scope        VARCHAR(20) NOT NULL DEFAULT 'releases',
    created_by   VARCHAR(255),
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMP WITH TIME ZONE,
    last_used_at TIMESTAMP WITH TIME ZONE,
    revoked_at   TIMESTAMP WITH TIME ZONE,
    CONSTRAINT api_keys_scope_check CHECK (scope IN ('releases', 'admin'))
);

-- Authentication looks up an active key by its hash; the partial index keeps
-- that hot path scoped to keys that have not been revoked.
CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(key_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_created_at ON api_keys(created_at DESC);
