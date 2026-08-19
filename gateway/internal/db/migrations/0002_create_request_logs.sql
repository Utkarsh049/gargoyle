-- Per-client decision audit logs for blocked or flagged requests.
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

CREATE INDEX IF NOT EXISTS idx_request_logs_client_id_timestamp ON request_logs (client_id, timestamp DESC);
