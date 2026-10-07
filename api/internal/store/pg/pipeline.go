package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PipelineStore provides cross-project pipeline run queries.
type PipelineStore struct {
	pool *pgxpool.Pool
}

var _ store.PipelineStorer = (*PipelineStore)(nil)

// NewPipelineStore creates a PipelineStore backed by the given PGStore's connection pool.
func NewPipelineStore(s *PGStore) *PipelineStore {
	return &PipelineStore{pool: s.pool}
}

const pipelineRunsQuery = `
WITH child_builds AS (
    SELECT COALESCE(b.ci_pipeline_id, '') AS ci_pipeline_id,
           COALESCE(b.ci_pipeline_url, '') AS ci_pipeline_url,
           COALESCE(b.ci_commit_sha, '') AS ci_commit_sha,
           COALESCE(b.ci_branch, '') AS ci_branch,
           COALESCE(b.ci_build_url, '') AS ci_build_url,
           b.created_at,
           b.project_id,
           p.slug AS project_slug,
           COALESCE(p.display_name, '') AS project_display_name,
           b.build_order,
           b.id AS build_id,
           b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_total,
           b.duration_ms,
           COALESCE(b.ci_pipeline_id, b.ci_commit_sha) AS group_key
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    WHERE p.parent_id = $1
      AND (b.ci_pipeline_id IS NOT NULL OR b.ci_commit_sha IS NOT NULL)
      AND ($2::text = '' OR b.ci_branch = $2)
),
distinct_groups AS (
    SELECT group_key,
           MAX(created_at) AS max_ts
    FROM child_builds
    GROUP BY group_key
    ORDER BY max_ts DESC
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM distinct_groups
),
paginated_groups AS (
    SELECT group_key FROM distinct_groups
    ORDER BY max_ts DESC
    LIMIT $3 OFFSET $4
)
SELECT cb.ci_pipeline_id, cb.ci_pipeline_url,
       cb.ci_commit_sha, cb.ci_branch, cb.ci_build_url, cb.created_at,
       cb.project_id, cb.project_slug, cb.project_display_name, cb.build_order, cb.build_id,
       cb.stat_passed, cb.stat_failed, cb.stat_broken, cb.stat_skipped, cb.stat_total,
       cb.duration_ms,
       tc.cnt
FROM child_builds cb
JOIN paginated_groups pg ON cb.group_key = pg.group_key
CROSS JOIN total_count tc
ORDER BY cb.created_at DESC, cb.project_id ASC`

// ListPipelineRuns returns builds from child projects of the given parent,
// paginated by distinct commit SHA. Returns flat rows that the caller groups.
func (s *PipelineStore) ListPipelineRuns(ctx context.Context, parentID int64, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
	offset := (page - 1) * perPage

	rows, err := s.pool.Query(ctx, pipelineRunsQuery, parentID, branch, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("pipeline runs query: %w", err)
	}
	defer rows.Close()

	var (
		result []store.PipelineRunRow
		total  int
	)
	for rows.Next() {
		var r store.PipelineRunRow
		if err := rows.Scan(
			&r.PipelineID, &r.PipelineURL,
			&r.CommitSHA, &r.Branch, &r.CIBuildURL, &r.CreatedAt,
			&r.ProjectID, &r.Slug, &r.DisplayName, &r.BuildNumber, &r.BuildID,
			&r.StatPassed, &r.StatFailed, &r.StatBroken, &r.StatSkipped, &r.StatTotal,
			&r.DurationMs,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan pipeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("pipeline rows iteration: %w", err)
	}

	return result, total, nil
}

// pipelineRunBuilds selects every build of every child project with its group
// (parent), CI metadata and stats, and the group_key that makes one run: the
// parent project ID prefixed to the pipeline ID (or commit SHA when there is no
// pipeline ID), so the same commit SHA appearing under two different parent
// groups remains two separate runs. Callers append the filters that scope it
// and wrap it as the child_builds CTE.
const pipelineRunBuilds = `
    SELECT COALESCE(b.ci_pipeline_id, '') AS ci_pipeline_id,
           COALESCE(b.ci_pipeline_url, '') AS ci_pipeline_url,
           COALESCE(b.ci_commit_sha, '') AS ci_commit_sha,
           COALESCE(b.ci_branch, '') AS ci_branch,
           COALESCE(b.ci_build_url, '') AS ci_build_url,
           b.created_at,
           b.project_id,
           p.slug AS project_slug,
           COALESCE(p.display_name, '') AS project_display_name,
           b.build_order,
           b.id AS build_id,
           gp.id AS group_project_id,
           gp.slug AS group_slug,
           b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_total,
           b.duration_ms,
           p.parent_id::text || ':' || COALESCE(b.ci_pipeline_id, b.ci_commit_sha) AS group_key
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    JOIN projects gp ON gp.id = p.parent_id
    WHERE p.parent_id IS NOT NULL
      AND (b.ci_pipeline_id IS NOT NULL OR b.ci_commit_sha IS NOT NULL)`

// pipelineRunCols is the child_builds projection scanned into a
// store.PipelineRunRow by pipelineRunDests; keep the two in the same order.
const pipelineRunCols = `cb.ci_pipeline_id, cb.ci_pipeline_url,
       cb.ci_commit_sha, cb.ci_branch, cb.ci_build_url, cb.created_at,
       cb.project_id, cb.project_slug, cb.project_display_name, cb.build_order, cb.build_id,
       cb.group_project_id, cb.group_slug,
       cb.stat_passed, cb.stat_failed, cb.stat_broken, cb.stat_skipped, cb.stat_total,
       cb.duration_ms`

// pipelineRunDests lists the scan targets of pipelineRunCols.
func pipelineRunDests(r *store.PipelineRunRow) []any {
	return []any{
		&r.PipelineID, &r.PipelineURL,
		&r.CommitSHA, &r.Branch, &r.CIBuildURL, &r.CreatedAt,
		&r.ProjectID, &r.Slug, &r.DisplayName, &r.BuildNumber, &r.BuildID,
		&r.GroupProjectID, &r.GroupSlug,
		&r.StatPassed, &r.StatFailed, &r.StatBroken, &r.StatSkipped, &r.StatTotal,
		&r.DurationMs,
	}
}

// allPipelineRunsQuery is the cross-parent generalization of pipelineRunsQuery.
const allPipelineRunsQuery = `
WITH child_builds AS (` + pipelineRunBuilds + `
      AND ($1::text = '' OR b.ci_branch = $1)
      AND (cardinality($2::bigint[]) = 0 OR p.parent_id = ANY($2))
),
distinct_groups AS (
    SELECT group_key,
           MAX(created_at) AS max_ts
    FROM child_builds
    GROUP BY group_key
    ORDER BY max_ts DESC
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM distinct_groups
),
paginated_groups AS (
    SELECT group_key FROM distinct_groups
    ORDER BY max_ts DESC
    LIMIT $3 OFFSET $4
)
SELECT ` + pipelineRunCols + `,
       tc.cnt
FROM child_builds cb
JOIN paginated_groups pg ON cb.group_key = pg.group_key
CROSS JOIN total_count tc
ORDER BY cb.created_at DESC, cb.project_id ASC`

// runKeyMatches is the filter that picks the builds of the run whose key is $2:
// the pipeline id, or the commit SHA when the build has no pipeline id (the
// COALESCE(ci_pipeline_id, ci_commit_sha) that group_key is built from).
// Spelled as a disjunction rather than that COALESCE so the planner can use
// idx_builds_pipeline_id and idx_builds_commit_sha. Equivalent because an empty
// pipeline id is stored as NULL (UpdateBuildCIMetadata).
const runKeyMatches = `(b.ci_pipeline_id = $2 OR (b.ci_pipeline_id IS NULL AND b.ci_commit_sha = $2))`

// pipelineRunQuery is allPipelineRunsQuery for one run: the same rows, scoped to
// the group and run key instead of paginated. The key is the one runFailuresQuery
// uses, so the run page and its failures always cover the same builds.
const pipelineRunQuery = `
WITH child_builds AS (` + pipelineRunBuilds + `
      AND p.parent_id = $1
      AND ` + runKeyMatches + `
)
SELECT ` + pipelineRunCols + `
FROM child_builds cb
ORDER BY cb.created_at DESC, cb.project_id ASC`

// runFailuresBody is the failures query both scopes share, appended to a
// run_builds CTE (build_id, run_key, build_order, project_id, project_slug,
// project_display_name) that says which builds make up the run and what its
// key is. Keeping one body is what keeps the REST run failures and the MCP
// diagnose_pipeline tags identical. status_message is capped the same way
// ListFailedByBuild caps it: callers only render a short preview.
//
// origin compares a failing test with its suite's baseline, matched by
// history_id. run_baselines gives each run build (a shard build gets its own)
// its baseline builds in two steps:
//   - the anchor is the latest build older than it (by build_order, never by
//     database id) on its own project's default branch, skipping builds of the
//     run itself (same run key: pipeline id, else commit SHA). Sibling shards
//     and a retried job's first attempt belong to the run being judged, so
//     comparing the run with them would compare it with itself. The first
//     branch a project ever uploads is its default, so the anchor can be on the
//     very branch the run is on; then it is the previous run there.
//   - if the anchor has a ci_pipeline_id, the baseline is every build of the
//     project on the default branch with that pipeline id, i.e. all the shards
//     of the baseline pipeline, so a test that ran in another shard is found.
//     An anchor with no pipeline id is the baseline on its own.
//
// Across the baseline builds, a test that failed or broke in any of them is
// inherited (a retried shard that failed first still counts); otherwise one
// that passed in any is new; everything else is unknown. Unknown covers: no
// default branch, no anchor, the test absent from every baseline build, no
// history_id to match on, and the test skipped or of unknown status everywhere
// it appears (it did not run, so it neither passed nor failed).
//
// run_baselines is MATERIALIZED so the lookup runs once per run build, not once
// per failing test. It walks from the default branch row into
// idx_builds_project_branch (project_id, branch_id, build_order DESC), so a
// stale default branch costs one index probe however many other builds lie
// between, and finds the shards through idx_builds_project_pipeline; the outer
// ORDER BY ... LIMIT 1 only matters if a project ever has two default branches.
// The non-empty history_id condition on bt repeats the predicate of the
// partial unique index on test_results(build_id, history_id), which the
// planner needs spelled out to use it.
//
// environment: the test's stored defect fingerprint is categorised
// infrastructure; no fingerprint is false.
const runFailuresBody = `,
run_baselines AS MATERIALIZED (
    SELECT rb.build_id, bl.build_ids AS baseline_build_ids
    FROM run_builds rb
    LEFT JOIN LATERAL (
        SELECT CASE WHEN d.pipeline_id IS NULL THEN ARRAY[d.id]
                    ELSE ARRAY(
                        SELECT sb.id
                        FROM builds sb
                        WHERE sb.project_id = d.project_id
                          AND sb.branch_id = d.branch_id
                          AND sb.ci_pipeline_id = d.pipeline_id
                    )
               END AS build_ids
        FROM branches br
        CROSS JOIN LATERAL (
            SELECT bb.id, bb.project_id, bb.branch_id, bb.build_order,
                   bb.ci_pipeline_id AS pipeline_id
            FROM builds bb
            WHERE bb.project_id = br.project_id
              AND bb.branch_id = br.id
              AND bb.build_order < rb.build_order
              AND COALESCE(bb.ci_pipeline_id, bb.ci_commit_sha) IS DISTINCT FROM rb.run_key
            ORDER BY bb.build_order DESC
            LIMIT 1
        ) d
        WHERE br.project_id = rb.project_id
          AND br.is_default
        ORDER BY d.build_order DESC
        LIMIT 1
    ) bl ON TRUE
)
SELECT rb.project_id, rb.project_slug, rb.project_display_name,
       rb.build_id, rb.build_order, tr.id,
       tr.test_name, tr.full_name, tr.status, tr.duration_ms,
       tr.history_id, tr.flaky, tr.retries, tr.new_failed,
       CASE WHEN base.failed THEN '` + store.FailureOriginInherited + `'
            WHEN base.passed THEN '` + store.FailureOriginNew + `'
            ELSE '` + store.FailureOriginUnknown + `'
       END AS origin,
       COALESCE(df.category = '` + store.DefectCategoryInfrastructure + `', FALSE) AS environment,
       COALESCE(LEFT(tr.status_message, 500), '')
FROM run_builds rb
JOIN run_baselines rbl ON rbl.build_id = rb.build_id
JOIN test_results tr
  ON tr.build_id = rb.build_id AND tr.project_id = rb.project_id
LEFT JOIN defect_fingerprints df ON df.id = tr.defect_fingerprint_id
LEFT JOIN LATERAL (
    SELECT bool_or(bt.status IN ('failed', 'broken')) AS failed,
           bool_or(bt.status = 'passed') AS passed
    FROM test_results bt
    WHERE bt.build_id = ANY(rbl.baseline_build_ids)
      AND bt.history_id = tr.history_id
      AND bt.history_id <> ''
) base ON TRUE
WHERE tr.status IN ('failed','broken')
ORDER BY rb.project_id ASC, rb.build_order ASC, tr.duration_ms DESC, tr.id ASC
LIMIT $3`

// runFailuresQuery collects the failing tests of an entire run in one round
// trip. run_builds resolves every build the group's children uploaded under the
// run key — a sharded suite contributes one build per shard — and the body then
// pulls their failed/broken results.
const runFailuresQuery = `
WITH run_builds AS (
    SELECT b.id AS build_id,
           COALESCE(b.ci_pipeline_id, b.ci_commit_sha) AS run_key,
           b.build_order,
           b.project_id,
           p.slug AS project_slug,
           COALESCE(p.display_name, '') AS project_display_name
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    WHERE p.parent_id = $1
      AND ` + runKeyMatches + `
)` + runFailuresBody

// pipelineFailuresQuery is runFailuresQuery for the builds of one project that
// share a ci_pipeline_id: only the run_builds scope differs.
const pipelineFailuresQuery = `
WITH run_builds AS (
    SELECT b.id AS build_id,
           b.ci_pipeline_id AS run_key,
           b.build_order,
           b.project_id,
           p.slug AS project_slug,
           COALESCE(p.display_name, '') AS project_display_name
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    WHERE b.project_id = $1
      AND b.ci_pipeline_id = $2
)` + runFailuresBody

// ListRunFailures returns the failing tests of every build in one pipeline run.
// runKey is a ci_pipeline_id, or a ci_commit_sha when the pipeline ID is absent
// — the same key groupPipelineRuns uses to assemble a run.
//
// Each row is tagged with Origin and Environment; see store.RunFailureRow.
//
// Rows are NOT deduplicated here. A build can hold two rows for one test when
// two ingestion paths assign different history_ids to it, and the only field
// they share is full_name; collapsing on that in SQL would also merge
// parameterized tests that legitimately share a full name. Callers that need a
// per-test view merge on their own terms.
func (s *PipelineStore) ListRunFailures(ctx context.Context, groupProjectID int64, runKey string, limit int) ([]store.RunFailureRow, error) {
	return s.queryRunFailures(ctx, runFailuresQuery, "run failures", groupProjectID, runKey, limit)
}

// ListFailuresByPipelineID returns the failing tests of the builds one project
// uploaded under pipelineID, tagged exactly as ListRunFailures tags them: both
// run the same query body.
func (s *PipelineStore) ListFailuresByPipelineID(ctx context.Context, projectID int64, pipelineID string, limit int) ([]store.RunFailureRow, error) {
	return s.queryRunFailures(ctx, pipelineFailuresQuery, "pipeline failures", projectID, pipelineID, limit)
}

func (s *PipelineStore) queryRunFailures(ctx context.Context, query, what string, scopeID int64, key string, limit int) ([]store.RunFailureRow, error) {
	rows, err := s.pool.Query(ctx, query, scopeID, key, limit)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", what, err)
	}
	defer rows.Close()

	result := []store.RunFailureRow{}
	for rows.Next() {
		var r store.RunFailureRow
		if err := rows.Scan(
			&r.ProjectID, &r.Slug, &r.DisplayName,
			&r.BuildID, &r.BuildNumber, &r.TestResultID,
			&r.TestName, &r.FullName, &r.Status, &r.DurationMs,
			&r.HistoryID, &r.Flaky, &r.Retries, &r.NewFailed,
			&r.Origin, &r.Environment,
			&r.StatusMessage,
		); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", what, err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", what, err)
	}

	return result, nil
}

// GetPipelineRun returns every build of one pipeline run: the builds the
// children of groupProjectID uploaded under runKey, a ci_pipeline_id or, when
// the pipeline ID is absent, a ci_commit_sha. A sharded suite contributes one
// row per shard. Rows have the shape ListAllPipelineRuns returns, so the same
// grouping builds the run the feed shows. An unknown run yields no rows.
func (s *PipelineStore) GetPipelineRun(ctx context.Context, groupProjectID int64, runKey string) ([]store.PipelineRunRow, error) {
	rows, err := s.pool.Query(ctx, pipelineRunQuery, groupProjectID, runKey)
	if err != nil {
		return nil, fmt.Errorf("pipeline run query: %w", err)
	}
	defer rows.Close()

	var result []store.PipelineRunRow
	for rows.Next() {
		var r store.PipelineRunRow
		if err := rows.Scan(pipelineRunDests(&r)...); err != nil {
			return nil, fmt.Errorf("scan pipeline run row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pipeline run rows iteration: %w", err)
	}

	return result, nil
}

// ListAllPipelineRuns returns builds across every parent/child project group,
// paginated by distinct (parent, commit-SHA-or-pipeline-ID) group. Returns
// flat rows that the caller groups. groupIDs must be non-nil (an empty slice
// disables the group filter); nil would break the cardinality() check pgx
// sends for a bigint[] parameter.
func (s *PipelineStore) ListAllPipelineRuns(ctx context.Context, branch string, groupIDs []int64, page, perPage int) ([]store.PipelineRunRow, int, error) {
	offset := (page - 1) * perPage
	if groupIDs == nil {
		groupIDs = []int64{}
	}

	rows, err := s.pool.Query(ctx, allPipelineRunsQuery, branch, groupIDs, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("all pipeline runs query: %w", err)
	}
	defer rows.Close()

	var (
		result []store.PipelineRunRow
		total  int
	)
	for rows.Next() {
		var r store.PipelineRunRow
		if err := rows.Scan(append(pipelineRunDests(&r), &total)...); err != nil {
			return nil, 0, fmt.Errorf("scan all pipeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("all pipeline rows iteration: %w", err)
	}

	return result, total, nil
}

// defaultPipelineBuildsLimit bounds ListBuildsByPipelineID when the caller
// passes no limit of its own. It matches the cap the MCP layer applies, which
// is in turn sized to the largest shard count a single CI pipeline realistically
// produces.
const defaultPipelineBuildsLimit = 50

// ListBuildsByPipelineID returns the builds within a single project that share
// the given ci_pipeline_id, ordered by build_order ascending. Unlike
// ListPipelineRuns/ListAllPipelineRuns above (which group builds across a
// parent's child projects via p.parent_id), this scopes directly to one
// project — the shape Playwright CI shards produce: several builds, one
// project, one ci_pipeline_id, one build per shard. Reuses buildSelectCols
// and scanBuildRowsAll from build.go (same package) rather than duplicating
// the builds-row shape here. Served by idx_builds_project_pipeline (migration
// 0051), which covers both filter columns.
//
// It returns up to limit+1 rows, not limit. A caller that asked for limit and
// received limit+1 knows the pipeline holds more builds than it displayed and
// can say so; capping at exactly limit would make a truncated list
// indistinguishable from a complete one. Callers are expected to drop the extra
// row before presenting the result.
//
// A non-positive limit means "unspecified" and is served with
// defaultPipelineBuildsLimit: PostgreSQL rejects a negative LIMIT and reads
// LIMIT 0 as "no rows", so passing the argument straight through would turn a
// caller's omission into an error or a silently empty answer.
func (s *PipelineStore) ListBuildsByPipelineID(ctx context.Context, projectID int64, pipelineID string, limit int) ([]store.Build, error) {
	if limit <= 0 {
		limit = defaultPipelineBuildsLimit
	}
	rows, err := s.pool.Query(ctx, buildSelectCols+`
		WHERE project_id=$1 AND ci_pipeline_id=$2
		ORDER BY build_order ASC
		LIMIT $3`, projectID, pipelineID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list builds by pipeline id: %w", err)
	}
	defer rows.Close()

	builds, err := scanBuildRowsAll(rows)
	if err != nil {
		return nil, err
	}
	return builds, nil
}
