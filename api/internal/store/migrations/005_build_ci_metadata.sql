ALTER TABLE builds ADD COLUMN ci_provider   TEXT;
ALTER TABLE builds ADD COLUMN ci_build_url  TEXT;
ALTER TABLE builds ADD COLUMN ci_branch     TEXT;
ALTER TABLE builds ADD COLUMN ci_commit_sha TEXT;
