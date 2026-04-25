-- +goose Up
-- Add password_hash column for local (non-OIDC) authentication.
-- bcrypt output is stored as a TEXT string. Existing rows (OIDC users) receive
-- the empty-string default which never matches a bcrypt comparison, so they
-- remain non-authenticatable via the /login password flow.
ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';

-- Partial unique index enforcing one active account per email (case-insensitive).
-- Deactivated accounts may share the email with a newly created replacement.
CREATE UNIQUE INDEX idx_users_email_active ON users (LOWER(email)) WHERE is_active = true;

-- +goose Down
DROP INDEX IF EXISTS idx_users_email_active;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
