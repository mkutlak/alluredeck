-- +goose Up
ALTER TABLE api_keys ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_api_keys_user_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS user_id;
