-- +goose Up
-- Drop the legacy exact-case unique constraint on users.email that was created
-- by the original 0019_users.sql DDL (`email TEXT NOT NULL UNIQUE`). It is
-- fully superseded by idx_users_email_global (0039), which is stricter:
-- case-insensitive and covers inactive rows for all non-env providers.
-- Keeping users_email_key alongside idx_users_email_global caused two problems:
--   1. On an exact-case duplicate the legacy constraint fires FIRST, so the
--      23505 error carries ConstraintName="users_email_key" instead of
--      "idx_users_email_global", breaking the OIDC email-collision mapping in
--      UpsertByOIDC (which only maps the latter to store.ErrEmailAlreadyLinked).
--   2. env-configured accounts have no users row, so removing this constraint
--      weakens nothing — the remaining partial index (WHERE provider != 'env')
--      already excludes them.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

-- +goose Down
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
