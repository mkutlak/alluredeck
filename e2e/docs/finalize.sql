-- Applied by `mise run docs:screenshots` after seed-demo.ts, straight to the demo database.

-- Builds can only be created "now" through the API. Space each project's builds 6h apart,
-- newest at now, so the dashboards read like a real week of CI.
UPDATE builds b
SET created_at = now() - (m.newest - b.build_order) * interval '6 hours'
FROM (SELECT project_id, MAX(build_order) AS newest FROM builds GROUP BY project_id) m
WHERE b.project_id = m.project_id;
