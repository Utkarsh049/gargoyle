-- =============================================================================
-- Gargoyle API Gateway - PostgreSQL Database Initialization & Seed Script
-- =============================================================================

CREATE TABLE IF NOT EXISTS clients (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    api_key_hash  TEXT NOT NULL UNIQUE,
    target_url    TEXT NOT NULL,
    rate_limit    INTEGER NOT NULL DEFAULT 60,
    plan_tier     TEXT NOT NULL DEFAULT 'free',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_clients_api_key_hash ON clients (api_key_hash);

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
CREATE INDEX IF NOT EXISTS idx_request_logs_timestamp ON request_logs (timestamp DESC);

-- Seed Initial Demo Client:
-- Plaintext Key: gk_live_mRLoRus8nmTOakGwOaEm5d99f7WwbD9t
-- Target: http://dummybackend:9000 (Internal Docker Network)
INSERT INTO clients (name, api_key_hash, target_url, rate_limit, plan_tier)
VALUES (
    'Demo Ingress Tenant',
    '48e3f5b11acf7dd2aee0fc31ebcae300e318175e315bb215210b18691c6a25cc',
    'http://dummybackend:9000',
    1000,
    'enterprise'
)
ON CONFLICT (api_key_hash) DO NOTHING;
