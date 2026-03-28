-- +goose Up
CREATE TABLE defect_fingerprints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint_hash TEXT NOT NULL,
    normalized_message TEXT,
    sample_trace    TEXT,
    category        TEXT NOT NULL DEFAULT 'to_investigate',
    resolution      TEXT NOT NULL DEFAULT 'open',
    known_issue_id  BIGINT REFERENCES known_issues(id) ON DELETE SET NULL,
    first_seen_build_id BIGINT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    last_seen_build_id  BIGINT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    occurrence_count    INT NOT NULL DEFAULT 0,
    consecutive_clean_builds INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_defect_fingerprints_project_hash
    ON defect_fingerprints(project_id, fingerprint_hash);
CREATE INDEX idx_defect_fingerprints_project_resolution
    ON defect_fingerprints(project_id, resolution);
CREATE INDEX idx_defect_fingerprints_project_category
    ON defect_fingerprints(project_id, category);

CREATE TABLE defect_occurrences (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    defect_fingerprint_id UUID NOT NULL REFERENCES defect_fingerprints(id) ON DELETE CASCADE,
    build_id              BIGINT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    test_result_count     INT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_defect_occurrences_fingerprint_build
    ON defect_occurrences(defect_fingerprint_id, build_id);
CREATE INDEX idx_defect_occurrences_build
    ON defect_occurrences(build_id);

ALTER TABLE test_results
    ADD COLUMN defect_fingerprint_id UUID REFERENCES defect_fingerprints(id) ON DELETE SET NULL;
CREATE INDEX idx_test_results_defect_fingerprint
    ON test_results(defect_fingerprint_id) WHERE defect_fingerprint_id IS NOT NULL;

-- +goose Down
ALTER TABLE test_results DROP COLUMN IF EXISTS defect_fingerprint_id;
DROP TABLE IF EXISTS defect_occurrences;
DROP TABLE IF EXISTS defect_fingerprints;
