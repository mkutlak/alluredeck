-- +goose Up
-- F-5 of SECURITY_REVIEW.md: prevent two OIDC (or local) providers from
-- creating distinct accounts with the same email. The partial-unique index
-- below applies to every row whose provider is NOT 'env' — env-configured
-- accounts (admin/viewer) live in config and have no users row, so they
-- cannot collide. Note that 0037's idx_users_email_active enforces "one
-- active row per email"; this new index extends to inactive rows too, so a
-- deactivated user's email cannot be re-stolen by a new OIDC provider.
--
-- Pre-migration safety: this assumes no current duplicate non-env rows
-- exist. Run a one-time data fix if a deployment has them; the migration
-- will fail loudly if so, which is the correct behaviour.

CREATE UNIQUE INDEX idx_users_email_global
    ON users (LOWER(email))
    WHERE provider != 'env';

-- +goose Down
DROP INDEX IF EXISTS idx_users_email_global;
