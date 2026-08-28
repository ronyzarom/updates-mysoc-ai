-- 015: customer_summaries — the compact per-customer aggregate that powers the
-- exceptions-first dashboard at 20k customers without scanning the full
-- instances table (Fleet Scalability 1.12).
--
-- Delta-reporting relays push one FleetSummary per customer whose aggregates
-- changed; the server upserts here keyed by (customer_id, reporter_id) so each
-- reporting relay owns its slice. status_reported_at is the summary freshness
-- the relay stamped, distinct from updated_at (when the server wrote the row).
CREATE TABLE IF NOT EXISTS customer_summaries (
    customer_id        TEXT        NOT NULL,
    reporter_id        TEXT        NOT NULL,
    customer_name      TEXT        NOT NULL DEFAULT '',
    total              INTEGER     NOT NULL DEFAULT 0,
    online             INTEGER     NOT NULL DEFAULT 0,
    offline            INTEGER     NOT NULL DEFAULT 0,
    degraded           INTEGER     NOT NULL DEFAULT 0,
    decommissioned     INTEGER     NOT NULL DEFAULT 0,
    failed_updates     INTEGER     NOT NULL DEFAULT 0,
    versions           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status_reported_at TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, reporter_id)
);

-- Exceptions-first ordering (most failures, then most offline) and the biggest
-- customers surface first without a full sort at query time.
CREATE INDEX IF NOT EXISTS idx_customer_summaries_exceptions
    ON customer_summaries(failed_updates DESC, offline DESC, total DESC);

-- Name search for the directory.
CREATE INDEX IF NOT EXISTS idx_customer_summaries_name
    ON customer_summaries(customer_name);
