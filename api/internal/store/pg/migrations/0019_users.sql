-- +goose Up
CREATE TABLE users (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL DEFAULT '',
    provider     TEXT NOT NULL DEFAULT 'local',
    provider_sub TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT 'viewer',
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    last_login   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_users_provider_sub ON users(provider, provider_sub) WHERE provider_sub != '';

-- +goose Down
DROP INDEX IF EXISTS idx_users_provider_sub;
DROP TABLE IF EXISTS users;
