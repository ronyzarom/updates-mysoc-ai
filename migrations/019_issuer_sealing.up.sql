-- Issuer sealing (1.16.2). Additive only: a 1.16.1 server keeps working on
-- this schema, and its inserts get seal_status 'unsealed' from the default.

-- Public keys the server accepts issuer seals from. issuer NULL means the key
-- is trusted for every issuer (today's shared release key).
CREATE TABLE IF NOT EXISTS trusted_keys (
    id          UUID PRIMARY KEY,
    key_id      TEXT NOT NULL UNIQUE,
    public_key  TEXT NOT NULL UNIQUE,
    issuer      TEXT,
    label       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at  TIMESTAMPTZ,
    CONSTRAINT trusted_keys_status_check CHECK (status IN ('active', 'retired'))
);

ALTER TABLE releases ADD COLUMN IF NOT EXISTS seal_status TEXT NOT NULL DEFAULT 'unsealed';
ALTER TABLE releases ADD COLUMN IF NOT EXISTS issuer TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN IF NOT EXISTS issuer_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN IF NOT EXISTS issuer_signature TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'releases_seal_status_check' AND conrelid = 'releases'::regclass) THEN
        ALTER TABLE releases ADD CONSTRAINT releases_seal_status_check
            CHECK (seal_status IN ('sealed', 'unsealed', 'invalid', 'unknown_key'));
    END IF;
END $$;
