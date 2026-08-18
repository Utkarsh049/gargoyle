-- Per-client decision logs: detailed records for blocked or flagged requests
-- (rate limits in Phase 3/5, heuristic and ML abuse detections in Phases 6/8).
-- High-cardinality detail lives here in Postgres rather than Prometheus to
-- prevent time-series metric blowup (see PROJECT.md §4 and §9).
CREATE TABLE IF NOT EXISTS request_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id   UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip          TEXT NOT NULL,
    path        TEXT NOT NULL,
    outcome     TEXT NOT NULL,
    abuse_score DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    reason      TEXT NOT NULL DEFAULT ''
);

-- Queries for client logs filter by client_id ordered by timestamp descending.
CREATE INDEX IF NOT EXISTS idx_request_logs_client_id_timestamp ON request_logs (client_id, timestamp DESC);
