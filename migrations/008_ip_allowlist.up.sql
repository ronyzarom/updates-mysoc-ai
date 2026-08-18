-- Migration 008: IP allowlist for the updater data-plane channel.
--
-- Allowlist-only enforcement: an instance (identified by instance_id in the
-- request body) may talk to the heartbeat / update-check / report / download
-- endpoints only from a source IP that matches an allowlist entry. There is no
-- trust-on-first-use; entries are provisioned by an administrator.
--
--   instance_id NULL  -> global entry, applies to every instance and to
--                        instance-less endpoints such as artifact download.
--   instance_id SET   -> entry scoped to that instance_id only.
--
-- cidr accepts a single IP (host) or a CIDR range, IPv4 or IPv6
-- (e.g. 203.0.113.7, 10.0.0.0/8, ::1, ::1/128).

CREATE TABLE IF NOT EXISTS ip_allowlist (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id VARCHAR(100),
    cidr        VARCHAR(64) NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Fast lookup of the entries that apply to a request: the per-instance entries
-- plus the global (NULL) entries.
CREATE INDEX IF NOT EXISTS idx_ip_allowlist_instance_id ON ip_allowlist(instance_id);

-- Prevent duplicate scope+cidr rows. COALESCE folds the global NULL scope into a
-- stable empty-string key so global duplicates are also rejected.
CREATE UNIQUE INDEX IF NOT EXISTS uq_ip_allowlist_scope_cidr
    ON ip_allowlist (COALESCE(instance_id, ''), cidr);
