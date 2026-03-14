-- +goose Up
DROP INDEX IF EXISTS idx_api_keys_key_hash;

-- +goose Down
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
