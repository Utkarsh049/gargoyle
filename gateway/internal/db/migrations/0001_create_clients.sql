-- Registered clients and routing configuration.
CREATE TABLE IF NOT EXISTS clients (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    api_key_hash  TEXT NOT NULL UNIQUE,
    target_url    TEXT NOT NULL,
    rate_limit    INTEGER NOT NULL DEFAULT 60,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_clients_api_key_hash ON clients (api_key_hash);
