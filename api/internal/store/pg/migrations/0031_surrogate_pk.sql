-- +goose Up

-- 1. Add surrogate PK column and slug column to projects.
ALTER TABLE projects ADD COLUMN pk SERIAL;
ALTER TABLE projects ADD COLUMN slug TEXT;
UPDATE projects SET slug = id;
ALTER TABLE projects ALTER COLUMN slug SET NOT NULL;

-- 2. Backfill display_name from slug where empty (should already be set by 0030,
--    but guard against any gap).
UPDATE projects SET display_name = slug WHERE display_name = '' OR display_name IS NULL;

-- 3. Add temporary INT FK columns to all child tables.
ALTER TABLE builds           ADD COLUMN project_pk INT;
ALTER TABLE test_results     ADD COLUMN project_pk INT;
ALTER TABLE branches         ADD COLUMN project_pk INT;
ALTER TABLE known_issues     ADD COLUMN project_pk INT;
ALTER TABLE defect_fingerprints ADD COLUMN project_pk INT;
ALTER TABLE webhooks         ADD COLUMN project_pk INT;
ALTER TABLE projects         ADD COLUMN parent_pk  INT;

-- 4. Populate the new INT columns from the projects.pk mapping.
UPDATE builds           b  SET project_pk = p.pk FROM projects p WHERE b.project_id  = p.id;
UPDATE test_results     tr SET project_pk = p.pk FROM projects p WHERE tr.project_id = p.id;
UPDATE branches         br SET project_pk = p.pk FROM projects p WHERE br.project_id = p.id;
UPDATE known_issues     ki SET project_pk = p.pk FROM projects p WHERE ki.project_id = p.id;
UPDATE defect_fingerprints df SET project_pk = p.pk FROM projects p WHERE df.project_id = p.id;
UPDATE webhooks         w  SET project_pk = p.pk FROM projects p WHERE w.project_id  = p.id;
UPDATE projects         c  SET parent_pk  = p.pk FROM projects p WHERE c.parent_id   = p.id;

-- 5. Drop the no-nesting trigger and function (will be recreated with INT types).
DROP TRIGGER IF EXISTS trg_no_nested_parent ON projects;
DROP FUNCTION IF EXISTS check_no_nested_parent();

-- 6. Drop all FK constraints referencing projects(id).
--    Constraint names confirmed from migration history:
--      0001: builds_project_id_fkey, known_issues_project_id_fkey (auto-named inline)
--      0010: branches_project_id_fkey (auto-named inline)
--      0021: projects_parent_id_fkey (auto-named inline)
--      0022: all four above dropped and explicitly recreated with same names + ON UPDATE CASCADE
--      0024: defect_fingerprints_project_id_fkey (auto-named inline, not in 0022)
--      0025: webhooks_project_id_fkey (auto-named inline, not in 0022)
ALTER TABLE builds              DROP CONSTRAINT IF EXISTS builds_project_id_fkey;
ALTER TABLE test_results        DROP CONSTRAINT IF EXISTS test_results_project_id_fkey;
ALTER TABLE branches            DROP CONSTRAINT IF EXISTS branches_project_id_fkey;
ALTER TABLE known_issues        DROP CONSTRAINT IF EXISTS known_issues_project_id_fkey;
ALTER TABLE defect_fingerprints DROP CONSTRAINT IF EXISTS defect_fingerprints_project_id_fkey;
ALTER TABLE webhooks            DROP CONSTRAINT IF EXISTS webhooks_project_id_fkey;
ALTER TABLE projects            DROP CONSTRAINT IF EXISTS projects_parent_id_fkey;

-- 7. Drop UNIQUE constraints and indexes that reference the old TEXT project_id columns.
--    builds(project_id, build_order) UNIQUE — auto-named in 0001.
ALTER TABLE builds    DROP CONSTRAINT IF EXISTS builds_project_id_build_order_key;
--    branches(project_id, name) UNIQUE — auto-named in 0010.
ALTER TABLE branches  DROP CONSTRAINT IF EXISTS branches_project_id_name_key;
--    defect_fingerprints(project_id, fingerprint_hash) UNIQUE INDEX — created in 0024.
DROP INDEX IF EXISTS idx_defect_fingerprints_project_hash;
--    defect_fingerprints(project_id, resolution) and (project_id, category) plain indexes.
DROP INDEX IF EXISTS idx_defect_fingerprints_project_resolution;
DROP INDEX IF EXISTS idx_defect_fingerprints_project_category;
--    Other plain indexes on old project_id columns.
DROP INDEX IF EXISTS idx_builds_project;
DROP INDEX IF EXISTS idx_test_results_project;
DROP INDEX IF EXISTS idx_test_results_history;
DROP INDEX IF EXISTS idx_branches_project;
DROP INDEX IF EXISTS idx_known_issues_project;
DROP INDEX IF EXISTS idx_webhooks_project_active;
--    Parent index on projects.
DROP INDEX IF EXISTS idx_projects_parent_id;

-- 8. Drop the old TEXT primary key on projects.
ALTER TABLE projects DROP CONSTRAINT projects_pkey;

-- 9. Drop the old TEXT id and project_id columns (and parent_id).
ALTER TABLE builds              DROP COLUMN project_id;
ALTER TABLE test_results        DROP COLUMN project_id;
ALTER TABLE branches            DROP COLUMN project_id;
ALTER TABLE known_issues        DROP COLUMN project_id;
ALTER TABLE defect_fingerprints DROP COLUMN project_id;
ALTER TABLE webhooks            DROP COLUMN project_id;
ALTER TABLE projects            DROP COLUMN parent_id;
ALTER TABLE projects            DROP COLUMN id;

-- 10. Rename new columns into place.
ALTER TABLE projects            RENAME COLUMN pk        TO id;
ALTER TABLE projects            RENAME COLUMN parent_pk TO parent_id;
ALTER TABLE builds              RENAME COLUMN project_pk TO project_id;
ALTER TABLE test_results        RENAME COLUMN project_pk TO project_id;
ALTER TABLE branches            RENAME COLUMN project_pk TO project_id;
ALTER TABLE known_issues        RENAME COLUMN project_pk TO project_id;
ALTER TABLE defect_fingerprints RENAME COLUMN project_pk TO project_id;
ALTER TABLE webhooks            RENAME COLUMN project_pk TO project_id;

-- 11. Set NOT NULL on the renamed child columns.
ALTER TABLE builds              ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE test_results        ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE branches            ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE known_issues        ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE defect_fingerprints ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE webhooks            ALTER COLUMN project_id SET NOT NULL;

-- 12. Promote projects.id (the renamed SERIAL) to primary key.
--     SERIAL already owns a sequence; making it the PK is safe.
ALTER TABLE projects ADD PRIMARY KEY (id);

-- 13. Restore UNIQUE constraints on child tables.
ALTER TABLE builds   ADD CONSTRAINT builds_project_id_build_order_key  UNIQUE (project_id, build_order);
ALTER TABLE branches ADD CONSTRAINT branches_project_id_name_key        UNIQUE (project_id, name);

-- 14. Restore FK constraints with the same semantics as before.
ALTER TABLE builds ADD CONSTRAINT builds_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE test_results ADD CONSTRAINT test_results_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE branches ADD CONSTRAINT branches_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE known_issues ADD CONSTRAINT known_issues_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;
-- defect_fingerprints had ON DELETE CASCADE only (not in 0022 ON UPDATE CASCADE batch).
ALTER TABLE defect_fingerprints ADD CONSTRAINT defect_fingerprints_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
-- webhooks had ON DELETE CASCADE ON UPDATE CASCADE (created that way in 0025).
ALTER TABLE webhooks ADD CONSTRAINT webhooks_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;
-- parent self-reference: ON DELETE CASCADE, no ON UPDATE CASCADE (parent_id is now INT,
-- updates to projects.id via SERIAL are not user-driven).
ALTER TABLE projects ADD CONSTRAINT projects_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES projects(id) ON DELETE CASCADE;

-- 15. Restore plain indexes on child project_id columns.
CREATE INDEX idx_builds_project               ON builds(project_id, build_order DESC);
CREATE INDEX idx_test_results_project         ON test_results(project_id);
CREATE INDEX idx_test_results_history         ON test_results(project_id, history_id);
CREATE INDEX idx_branches_project             ON branches(project_id);
CREATE INDEX idx_known_issues_project         ON known_issues(project_id);
CREATE INDEX idx_webhooks_project_active      ON webhooks(project_id) WHERE is_active = true;
CREATE INDEX idx_projects_parent_id           ON projects(parent_id) WHERE parent_id IS NOT NULL;

-- Restore defect_fingerprints indexes (unique + plain).
CREATE UNIQUE INDEX idx_defect_fingerprints_project_hash
    ON defect_fingerprints(project_id, fingerprint_hash);
CREATE INDEX idx_defect_fingerprints_project_resolution
    ON defect_fingerprints(project_id, resolution);
CREATE INDEX idx_defect_fingerprints_project_category
    ON defect_fingerprints(project_id, category);

-- 16. Add slug uniqueness indexes.
--     Standalone projects: globally unique slug.
CREATE UNIQUE INDEX idx_projects_slug_standalone ON projects(slug) WHERE parent_id IS NULL;
--     Child projects: slug unique per parent.
CREATE UNIQUE INDEX idx_projects_slug_per_parent ON projects(parent_id, slug) WHERE parent_id IS NOT NULL;

-- 17. Re-create no-nesting trigger with INT column types.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_no_nested_parent() RETURNS TRIGGER AS $$
BEGIN
  IF NEW.parent_id IS NOT NULL THEN
    IF EXISTS (SELECT 1 FROM projects WHERE id = NEW.parent_id AND parent_id IS NOT NULL) THEN
      RAISE EXCEPTION 'Cannot nest projects: parent "%" is already a child', NEW.parent_id;
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_no_nested_parent
  BEFORE INSERT OR UPDATE OF parent_id ON projects
  FOR EACH ROW EXECUTE FUNCTION check_no_nested_parent();

-- +goose Down
-- This migration is not reversible. Restore from backup if needed.
SELECT 1/0;
