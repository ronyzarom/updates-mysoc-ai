-- Keep deleted identities for stale-report suppression, outside fleet views.
ALTER TABLE instances ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
