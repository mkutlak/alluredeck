-- +goose Up
-- Remove orphan top-level projects created by InsertOrIgnore/Watcher bug
-- (post-migration 0031). Only rows with an existing child of the same slug
-- AND no attached data are deleted.
DELETE FROM projects p
WHERE p.parent_id IS NULL
  AND EXISTS (
    SELECT 1 FROM projects c
    WHERE c.slug = p.slug AND c.parent_id IS NOT NULL
  )
  AND NOT EXISTS (SELECT 1 FROM builds              WHERE project_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM test_results        WHERE project_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM branches            WHERE project_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM known_issues        WHERE project_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM defect_fingerprints WHERE project_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM webhooks            WHERE project_id = p.id);

-- +goose Down
-- This migration is not reversible. Restore from backup if needed.
SELECT 1;
