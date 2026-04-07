-- +goose Up
CREATE TABLE user_preferences (
    username    TEXT PRIMARY KEY,
    preferences JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS user_preferences;
