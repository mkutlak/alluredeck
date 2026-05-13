-- +goose Up
-- Migration: MCP audit actions, proposal tables, and api_keys.allow_mcp_writes.
--
-- 1. audit_log_action_check — 0038 never created this CHECK; DROP is a no-op.
--    We create it here with all current action values (from store/types.go) plus
--    the five new MCP proposal actions.
-- 2. audit_log_target_type_check — also never created by 0038 (no-op DROP).
--    Included for forward symmetry if a future migration adds this constraint.
-- 3. defect_proposals, known_issue_proposals, flaky_proposals — MCP proposal
--    workflow tables. All mutations are proposal-only; a human approves via UI.
-- 4. api_keys.allow_mcp_writes — feature flag per API key; default FALSE so
--    existing keys are unaffected.
--
-- Note: goose wraps each migration in a transaction automatically; no explicit
-- BEGIN/COMMIT needed here.

-- -------------------------------------------------------------------------
-- Step 1: audit_log action CHECK constraint
-- -------------------------------------------------------------------------
-- DROP is a no-op: 0038_audit_log.sql never added audit_log_action_check.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_action_check;

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_action_check CHECK (action IN (
        -- auth actions (from store/types.go AuditAction* constants)
        'auth.login.success',
        'auth.login.failure',
        'auth.logout',
        'auth.refresh.success',
        'auth.refresh.compromise',
        'auth.session.revoke_all',
        -- user lifecycle
        'users.create',
        'users.update.role',
        'users.update.active',
        'users.delete',
        'users.password_change',
        'users.password_reset',
        -- API key lifecycle
        'api_keys.create',
        'api_keys.delete',
        'api_keys.cascade_delete',
        -- MCP proposal actions (new in this migration)
        'mcp.propose_defect_classify',
        'mcp.propose_known_issue',
        'mcp.propose_flaky',
        'mcp.proposal_approve',
        'mcp.proposal_reject'
    ));

-- -------------------------------------------------------------------------
-- Step 2: audit_log target_type CHECK constraint (no-op drop + comment)
-- -------------------------------------------------------------------------
-- no-op — 0038 does not define this CHECK; included for forward symmetry
-- if a later migration adds audit_log_target_type_check.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_target_type_check;

-- -------------------------------------------------------------------------
-- Step 3: defect_proposals
-- -------------------------------------------------------------------------
-- Stores MCP-proposed reclassifications of a defect fingerprint.
-- A proposal is approved/rejected by a human; approval writes through to
-- defect_fingerprints.category / .resolution.
CREATE TABLE defect_proposals (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id           INT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint_hash     TEXT NOT NULL,
    proposed_category    TEXT NOT NULL,
    proposed_resolution  TEXT,
    rationale            TEXT,
    proposer_user_id     BIGINT NOT NULL REFERENCES users(id),
    proposer_api_key_id  BIGINT REFERENCES api_keys(id),
    status               TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by_user_id  BIGINT REFERENCES users(id),
    reviewed_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_defect_proposals_project_status
    ON defect_proposals (project_id, status);
CREATE INDEX idx_defect_proposals_status_created
    ON defect_proposals (status, created_at);

-- -------------------------------------------------------------------------
-- Step 4: known_issue_proposals
-- -------------------------------------------------------------------------
-- Stores MCP-proposed new known-issue rules (regex-based, not fingerprint).
-- proposed_category here is the issue category (e.g. 'infrastructure'),
-- not a defect category — the column name is shared from the base shape.
-- error_message_sample replaces fingerprint_hash (regex matches by message,
-- not by hash).
-- dry_run_match_count is populated at proposal time by scanning recent
-- test_results; updated on re-dry-run before approval.
CREATE TABLE known_issue_proposals (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id           INT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    error_message_sample TEXT,
    proposed_category    TEXT NOT NULL,  -- issue category (e.g. 'infrastructure')
    proposed_resolution  TEXT,
    rationale            TEXT,
    regex_pattern        TEXT NOT NULL,
    applies_to_status    TEXT[] NOT NULL DEFAULT '{}',
    dry_run_match_count  INT NOT NULL DEFAULT 0,
    proposer_user_id     BIGINT NOT NULL REFERENCES users(id),
    proposer_api_key_id  BIGINT REFERENCES api_keys(id),
    status               TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by_user_id  BIGINT REFERENCES users(id),
    reviewed_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_known_issue_proposals_project_status
    ON known_issue_proposals (project_id, status);
CREATE INDEX idx_known_issue_proposals_status_created
    ON known_issue_proposals (status, created_at);

-- -------------------------------------------------------------------------
-- Step 5: flaky_proposals
-- -------------------------------------------------------------------------
-- Stores MCP-proposed flaky-test flags, keyed by test identity
-- (test_full_name + history_id). No fingerprint_hash / proposed_category /
-- proposed_resolution — those concepts don't apply to flakiness proposals.
-- The unique partial index prevents duplicate pending proposals for the same
-- test; once a proposal is approved or rejected a new one can be submitted.
CREATE TABLE flaky_proposals (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id           INT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_full_name       TEXT NOT NULL,
    history_id           TEXT NOT NULL,
    rationale            TEXT,
    proposer_user_id     BIGINT NOT NULL REFERENCES users(id),
    proposer_api_key_id  BIGINT REFERENCES api_keys(id),
    status               TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by_user_id  BIGINT REFERENCES users(id),
    reviewed_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite index for listing pending proposals per project.
CREATE INDEX idx_flaky_proposals_project_status
    ON flaky_proposals (project_id, status);
CREATE INDEX idx_flaky_proposals_status_created
    ON flaky_proposals (status, created_at);

-- Prevent duplicate pending proposals for the same test.
CREATE UNIQUE INDEX idx_flaky_proposals_pending_unique
    ON flaky_proposals (project_id, test_full_name, history_id)
    WHERE status = 'pending';

-- -------------------------------------------------------------------------
-- Step 6: api_keys.allow_mcp_writes
-- -------------------------------------------------------------------------
-- Feature flag: FALSE by default so all existing API keys are read-only
-- from the MCP layer until an admin explicitly enables writes.
ALTER TABLE api_keys
    ADD COLUMN allow_mcp_writes BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN IF EXISTS allow_mcp_writes;
DROP TABLE IF EXISTS flaky_proposals;
DROP TABLE IF EXISTS known_issue_proposals;
DROP TABLE IF EXISTS defect_proposals;
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_action_check;
-- Note: audit_log_target_type_check was never created; DROP is a no-op.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_target_type_check;
