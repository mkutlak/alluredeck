-- +goose Up
CREATE TABLE refresh_token_families (
    family_id     UUID PRIMARY KEY,
    user_id       TEXT NOT NULL,
    role          TEXT NOT NULL,
    provider      TEXT NOT NULL DEFAULT 'local',
    current_jti   TEXT NOT NULL,
    previous_jti  TEXT,
    grace_until   TIMESTAMP WITH TIME ZONE,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_refresh_token_families_user_id ON refresh_token_families(user_id);
CREATE INDEX idx_refresh_token_families_expires ON refresh_token_families(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_refresh_token_families_expires;
DROP INDEX IF EXISTS idx_refresh_token_families_user_id;
DROP TABLE IF EXISTS refresh_token_families;
