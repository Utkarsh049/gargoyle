-- Registered clients: the mapping from an API key to a backend to forward
-- to, plus the config later phases hang off of (rate_limit in Phase 3,
-- plan_tier for future tiering). gen_random_uuid() is built into Postgres
-- core as of version 13, so no extension is required.
CREATE TABLE IF NOT EXISTS clients (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    api_key_hash  TEXT NOT NULL UNIQUE,
    target_url    TEXT NOT NULL,
    rate_limit    INTEGER NOT NULL DEFAULT 60,
    plan_tier     TEXT NOT NULL DEFAULT 'free',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every hot-path lookup is by api_key_hash; the UNIQUE constraint above
-- already creates a supporting index, but this makes the intent explicit
-- and survives if the constraint is ever relaxed.
CREATE INDEX IF NOT EXISTS idx_clients_api_key_hash ON clients (api_key_hash);
