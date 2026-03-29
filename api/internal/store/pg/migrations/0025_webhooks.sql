-- +goose Up
CREATE TABLE webhooks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
    name        TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT 'generic'
        CHECK (target_type IN ('slack', 'discord', 'teams', 'generic')),
    url         TEXT NOT NULL,
    secret      TEXT,
    template    TEXT,
    events      TEXT[] NOT NULL DEFAULT '{report_completed}',
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhooks_project_active ON webhooks(project_id) WHERE is_active = true;

CREATE TABLE webhook_deliveries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id    UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    build_id      BIGINT REFERENCES builds(id) ON DELETE SET NULL,
    event         TEXT NOT NULL,
    payload       JSONB NOT NULL,
    status_code   INT,
    response_body TEXT,
    error         TEXT,
    attempt       INT NOT NULL DEFAULT 1,
    duration_ms   INT,
    delivered_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wh_deliveries_webhook ON webhook_deliveries(webhook_id, delivered_at DESC);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
