-- +goose Up
ALTER TABLE builds DROP CONSTRAINT builds_project_id_fkey;
ALTER TABLE builds ADD CONSTRAINT builds_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE known_issues DROP CONSTRAINT known_issues_project_id_fkey;
ALTER TABLE known_issues ADD CONSTRAINT known_issues_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE test_results DROP CONSTRAINT test_results_project_id_fkey;
ALTER TABLE test_results ADD CONSTRAINT test_results_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE branches DROP CONSTRAINT branches_project_id_fkey;
ALTER TABLE branches ADD CONSTRAINT branches_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE projects DROP CONSTRAINT projects_parent_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_parent_id_fkey
  FOREIGN KEY (parent_id) REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE;

-- +goose Down
ALTER TABLE projects DROP CONSTRAINT projects_parent_id_fkey;
ALTER TABLE projects ADD CONSTRAINT projects_parent_id_fkey
  FOREIGN KEY (parent_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE branches DROP CONSTRAINT branches_project_id_fkey;
ALTER TABLE branches ADD CONSTRAINT branches_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE test_results DROP CONSTRAINT test_results_project_id_fkey;
ALTER TABLE test_results ADD CONSTRAINT test_results_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE known_issues DROP CONSTRAINT known_issues_project_id_fkey;
ALTER TABLE known_issues ADD CONSTRAINT known_issues_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE builds DROP CONSTRAINT builds_project_id_fkey;
ALTER TABLE builds ADD CONSTRAINT builds_project_id_fkey
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
