-- 009_webhooks.sql
SET search_path TO assets;

CREATE TABLE IF NOT EXISTS assets.webhook_endpoints (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id           UUID NOT NULL REFERENCES assets.organizations(id),
    url              VARCHAR(2048) NOT NULL,
    secret           BYTEA,
    events           TEXT[] NOT NULL DEFAULT '{*}',
    active           BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS assets.webhook_deliveries (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    endpoint_id   UUID NOT NULL REFERENCES assets.webhook_endpoints(id) ON DELETE CASCADE,
    event_type    VARCHAR(100) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','success','failed','retrying')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    status_code   INTEGER,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint
    ON assets.webhook_deliveries(endpoint_id, created_at DESC);
