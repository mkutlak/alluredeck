-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS parent_id TEXT REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_projects_parent_id ON projects(parent_id) WHERE parent_id IS NOT NULL;

-- Enforce max 1 level: a child's parent must not itself have a parent.
-- Using a trigger instead of CHECK with subquery for reliability.
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
DROP TRIGGER IF EXISTS trg_no_nested_parent ON projects;
DROP FUNCTION IF EXISTS check_no_nested_parent();
DROP INDEX IF EXISTS idx_projects_parent_id;
ALTER TABLE projects DROP COLUMN IF EXISTS parent_id;
