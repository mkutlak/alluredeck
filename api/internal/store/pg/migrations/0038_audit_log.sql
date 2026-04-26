-- +goose Up
-- Persistent audit log for security-sensitive events. Records authentication,
-- user lifecycle, and credential operations so incident responders can answer
-- "who did what, when, from where" without grepping log files. Schema is
-- intentionally narrow and append-only — there is no UPDATE path. Lookups are
-- expected to be by actor (timeline view) or by action (incident triage), so
-- both columns get descending indexes on occurred_at.
--
-- Columns:
--   actor_id    NULL for unauthenticated events (e.g. failed login pre-lookup);
--               otherwise the users.id of the acting principal.
--   actor_label cheap denormalisation of email or env-username so the row
--               stays readable after a user is deleted/renamed.
--   target_id   stored as TEXT because targets span numeric ids (users) and
--               opaque string ids (refresh-token family UUIDs, api-key UUIDs).
--   metadata    optional JSONB for action-specific details (e.g. previous
--               role on a role change). Always queryable, never required.
CREATE TABLE audit_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id    BIGINT,
    actor_label TEXT NOT NULL DEFAULT '',
    target_type TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    outcome     TEXT NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    request_id  TEXT NOT NULL DEFAULT '',
    metadata    JSONB,
    CONSTRAINT  audit_log_outcome_chk CHECK (outcome IN ('success', 'failure'))
);

-- Per-actor timeline (admin "user activity" view). Partial so the index does
-- not bloat with unauthenticated failure rows whose actor_id is NULL.
CREATE INDEX idx_audit_log_actor_occurred
    ON audit_log (actor_id, occurred_at DESC)
    WHERE actor_id IS NOT NULL;

-- Per-action timeline (incident triage: "show me every refresh.compromise in
-- the last 24h").
CREATE INDEX idx_audit_log_action_occurred
    ON audit_log (action, occurred_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_log_action_occurred;
DROP INDEX IF EXISTS idx_audit_log_actor_occurred;
DROP TABLE IF EXISTS audit_log;
