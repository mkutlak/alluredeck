package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface compliance check.
var _ store.DefectStorer = (*DefectStore)(nil)

// DefectStore provides operations on the defect_fingerprints, defect_occurrences,
// and related tables using PostgreSQL.
type DefectStore struct {
	pool *pgxpool.Pool
}

// NewDefectStore creates a DefectStore backed by the given PGStore.
func NewDefectStore(s *PGStore) *DefectStore {
	return &DefectStore{pool: s.pool}
}

// UpsertFingerprints bulk-upserts fingerprints in a transaction. On conflict
// (project_id, fingerprint_hash) it updates last_seen_build_id, adds to
// occurrence_count, resets consecutive_clean_builds to 0, and reopens
// previously "fixed" fingerprints (regression).
func (ds *DefectStore) UpsertFingerprints(ctx context.Context, projectID string, buildID int64, fingerprints []store.DefectFingerprint) error {
	if len(fingerprints) == 0 {
		return nil
	}

	tx, err := ds.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("upsert fingerprints begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO defect_fingerprints (
			project_id, fingerprint_hash, normalized_message, sample_trace,
			category, resolution, first_seen_build_id, last_seen_build_id,
			occurrence_count, consecutive_clean_builds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, 0)
		ON CONFLICT (project_id, fingerprint_hash) DO UPDATE SET
			last_seen_build_id        = EXCLUDED.last_seen_build_id,
			occurrence_count          = defect_fingerprints.occurrence_count + EXCLUDED.occurrence_count,
			consecutive_clean_builds  = 0,
			resolution                = CASE
				WHEN defect_fingerprints.resolution = 'fixed' THEN 'open'
				ELSE defect_fingerprints.resolution
			END,
			updated_at = NOW()`

	for i := range fingerprints {
		fp := &fingerprints[i]
		if _, err := tx.Exec(ctx, q,
			projectID,
			fp.FingerprintHash,
			fp.NormalizedMessage,
			fp.SampleTrace,
			fp.Category,
			store.DefectResolutionOpen,
			buildID,
			fp.OccurrenceCount,
		); err != nil {
			return fmt.Errorf("upsert fingerprint %s: %w", fp.FingerprintHash, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("upsert fingerprints commit: %w", err)
	}
	return nil
}

// LinkTestResults associates individual test result rows with a fingerprint for
// a given build. It updates test_results.defect_fingerprint_id and upserts a
// defect_occurrences row with the test_result_count.
func (ds *DefectStore) LinkTestResults(ctx context.Context, fingerprintID string, buildID int64, testResultIDs []int64) error {
	if len(testResultIDs) == 0 {
		return nil
	}

	// Update test_results to reference the fingerprint.
	const updateQ = `UPDATE test_results SET defect_fingerprint_id = $1 WHERE id = ANY($2)`
	if _, err := ds.pool.Exec(ctx, updateQ, fingerprintID, testResultIDs); err != nil {
		return fmt.Errorf("link test results update: %w", err)
	}

	// Upsert defect_occurrences row.
	const upsertQ = `
		INSERT INTO defect_occurrences (defect_fingerprint_id, build_id, test_result_count)
		VALUES ($1, $2, $3)
		ON CONFLICT (defect_fingerprint_id, build_id) DO UPDATE SET
			test_result_count = EXCLUDED.test_result_count`
	if _, err := ds.pool.Exec(ctx, upsertQ, fingerprintID, buildID, len(testResultIDs)); err != nil {
		return fmt.Errorf("link test results upsert occurrence: %w", err)
	}
	return nil
}

// UpdateCleanBuildCounts increments consecutive_clean_builds for all open
// fingerprints in the project that were NOT seen in this build.
func (ds *DefectStore) UpdateCleanBuildCounts(ctx context.Context, projectID string, buildID int64) error {
	const q = `
		UPDATE defect_fingerprints
		SET consecutive_clean_builds = consecutive_clean_builds + 1,
		    updated_at = NOW()
		WHERE project_id = $1
		  AND resolution = 'open'
		  AND id NOT IN (
			SELECT defect_fingerprint_id FROM defect_occurrences WHERE build_id = $2
		  )`
	if _, err := ds.pool.Exec(ctx, q, projectID, buildID); err != nil {
		return fmt.Errorf("update clean build counts: %w", err)
	}
	return nil
}

// AutoResolveFixed sets resolution='fixed' for open fingerprints where
// consecutive_clean_builds >= threshold. Returns count of affected rows.
func (ds *DefectStore) AutoResolveFixed(ctx context.Context, projectID string, threshold int) (int, error) {
	const q = `
		UPDATE defect_fingerprints
		SET resolution = 'fixed', updated_at = NOW()
		WHERE project_id = $1
		  AND resolution = 'open'
		  AND consecutive_clean_builds >= $2`
	tag, err := ds.pool.Exec(ctx, q, projectID, threshold)
	if err != nil {
		return 0, fmt.Errorf("auto resolve fixed: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// DetectRegressions finds fingerprints that were previously fixed but reappeared
// in this build. A regression is a fingerprint where: it appears in this build's
// occurrences, was NOT in the immediately preceding build's occurrences, and
// first_seen_build_id != this build (not new).
func (ds *DefectStore) DetectRegressions(ctx context.Context, projectID string, buildID int64) ([]string, error) {
	const q = `
		WITH prev_build AS (
			SELECT id FROM builds
			WHERE project_id = $1 AND id < $2
			ORDER BY build_order DESC LIMIT 1
		)
		SELECT df.id::text
		FROM defect_fingerprints df
		JOIN defect_occurrences do_cur ON do_cur.defect_fingerprint_id = df.id AND do_cur.build_id = $2
		WHERE df.project_id = $1
		  AND df.first_seen_build_id != $2
		  AND NOT EXISTS (
			SELECT 1 FROM defect_occurrences do_prev
			WHERE do_prev.defect_fingerprint_id = df.id
			  AND do_prev.build_id = (SELECT id FROM prev_build)
		  )`

	rows, err := ds.pool.Query(ctx, q, projectID, buildID)
	if err != nil {
		return nil, fmt.Errorf("detect regressions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan regression id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate regressions: %w", err)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// GetByHash retrieves a fingerprint by its project-scoped hash.
func (ds *DefectStore) GetByHash(ctx context.Context, projectID, hash string) (*store.DefectFingerprint, error) {
	var fp store.DefectFingerprint
	err := ds.pool.QueryRow(ctx, `
		SELECT id, project_id, fingerprint_hash, normalized_message, sample_trace,
		       category, resolution, known_issue_id, first_seen_build_id, last_seen_build_id,
		       occurrence_count, consecutive_clean_builds,
		       created_at::text, updated_at::text
		FROM defect_fingerprints
		WHERE project_id = $1 AND fingerprint_hash = $2`, projectID, hash,
	).Scan(&fp.ID, &fp.ProjectID, &fp.FingerprintHash, &fp.NormalizedMessage, &fp.SampleTrace,
		&fp.Category, &fp.Resolution, &fp.KnownIssueID, &fp.FirstSeenBuildID, &fp.LastSeenBuildID,
		&fp.OccurrenceCount, &fp.ConsecutiveCleanBuilds, &fp.CreatedAt, &fp.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: hash=%s", store.ErrDefectNotFound, hash)
	}
	if err != nil {
		return nil, fmt.Errorf("get defect by hash: %w", err)
	}
	return &fp, nil
}

// listDefects is a shared helper for ListByProject and ListByBuild. When buildID
// is non-nil the query is scoped through defect_occurrences.
func (ds *DefectStore) listDefects(ctx context.Context, projectID string, buildID *int64, filter store.DefectFilter) ([]store.DefectListRow, int, error) {
	// --- build WHERE clause ---
	where := "WHERE df.project_id = $1"
	args := []any{projectID}
	argIdx := 2

	if buildID != nil {
		where += fmt.Sprintf(" AND do.build_id = $%d", argIdx)
		args = append(args, *buildID)
		argIdx++
	}
	if filter.Resolution != "" {
		where += fmt.Sprintf(" AND df.resolution = $%d", argIdx)
		args = append(args, filter.Resolution)
		argIdx++
	}
	if filter.Category != "" {
		where += fmt.Sprintf(" AND df.category = $%d", argIdx)
		args = append(args, filter.Category)
		argIdx++
	}
	if filter.Search != "" {
		where += fmt.Sprintf(" AND df.normalized_message ILIKE $%d", argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	// --- JOIN clause ---
	joinClause := ""
	if buildID != nil {
		joinClause = "JOIN defect_occurrences do ON do.defect_fingerprint_id = df.id"
	}

	// --- count query ---
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM defect_fingerprints df %s %s", joinClause, where)
	var total int
	if err := ds.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count defects: %w", err)
	}

	// --- sort ---
	sortCol := "df.occurrence_count"
	switch filter.SortBy {
	case "first_seen":
		sortCol = "df.first_seen_build_id"
	case "last_seen":
		sortCol = "df.last_seen_build_id"
	case "occurrence_count":
		sortCol = "df.occurrence_count"
	}
	order := "DESC"
	if strings.EqualFold(filter.Order, "asc") {
		order = "ASC"
	}

	// --- pagination ---
	page := max(filter.Page, 1)
	perPage := filter.PerPage
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	// --- data query ---
	// Subquery-based join avoids ambiguity in the select list.
	selectCols := `
		df.id, df.project_id, df.fingerprint_hash, df.normalized_message, df.sample_trace,
		df.category, df.resolution, df.known_issue_id, df.first_seen_build_id, df.last_seen_build_id,
		df.occurrence_count, df.consecutive_clean_builds,
		df.created_at::text, df.updated_at::text,
		b_first.build_order AS first_seen_build_order,
		b_last.build_order AS last_seen_build_order,
		ki.id AS ki_id, ki.project_id AS ki_project_id, ki.test_name AS ki_test_name,
		ki.pattern AS ki_pattern, ki.ticket_url AS ki_ticket_url,
		ki.description AS ki_description, ki.is_active AS ki_is_active,
		ki.created_at AS ki_created_at, ki.updated_at AS ki_updated_at`

	testResultCountCol := "NULL::int"
	if buildID != nil {
		testResultCountCol = "do.test_result_count"
	}
	selectCols += fmt.Sprintf(", %s AS test_result_count_in_build", testResultCountCol)

	dataQ := fmt.Sprintf(`
		SELECT %s
		FROM defect_fingerprints df
		%s
		JOIN builds b_first ON b_first.id = df.first_seen_build_id
		JOIN builds b_last  ON b_last.id  = df.last_seen_build_id
		LEFT JOIN known_issues ki ON ki.id = df.known_issue_id
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`,
		selectCols, joinClause, where, sortCol, order, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := ds.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list defects: %w", err)
	}
	defer rows.Close()

	var result []store.DefectListRow
	for rows.Next() {
		var row store.DefectListRow
		var kiID *int64
		var kiProjectID, kiTestName, kiPattern, kiTicketURL, kiDescription *string
		var kiIsActive *bool
		var kiCreatedAt, kiUpdatedAt *string
		var testResultCount *int

		if err := rows.Scan(
			&row.ID, &row.ProjectID, &row.FingerprintHash, &row.NormalizedMessage, &row.SampleTrace,
			&row.Category, &row.Resolution, &row.KnownIssueID,
			&row.FirstSeenBuildID, &row.LastSeenBuildID,
			&row.OccurrenceCount, &row.ConsecutiveCleanBuilds,
			&row.CreatedAt, &row.UpdatedAt,
			&row.FirstSeenBuildOrder, &row.LastSeenBuildOrder,
			&kiID, &kiProjectID, &kiTestName, &kiPattern, &kiTicketURL,
			&kiDescription, &kiIsActive, &kiCreatedAt, &kiUpdatedAt,
			&testResultCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan defect row: %w", err)
		}

		row.TestResultCountInBuild = testResultCount

		if kiID != nil {
			row.KnownIssue = &store.KnownIssue{
				ID:          *kiID,
				ProjectID:   derefStr(kiProjectID),
				TestName:    derefStr(kiTestName),
				Pattern:     derefStr(kiPattern),
				TicketURL:   derefStr(kiTicketURL),
				Description: derefStr(kiDescription),
				IsActive:    derefBool(kiIsActive),
			}
		}

		// Determine regression / new flags.
		if buildID != nil {
			row.IsNew = row.FirstSeenBuildID == *buildID
		}

		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate defect rows: %w", err)
	}
	if result == nil {
		result = []store.DefectListRow{}
	}
	return result, total, nil
}

// derefStr safely dereferences a *string pointer.
func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

// derefBool safely dereferences a *bool pointer.
func derefBool(p *bool) bool {
	if p != nil {
		return *p
	}
	return false
}

// ListByProject returns a paginated list of defects for a project with optional filters.
func (ds *DefectStore) ListByProject(ctx context.Context, projectID string, filter store.DefectFilter) ([]store.DefectListRow, int, error) {
	return ds.listDefects(ctx, projectID, nil, filter)
}

// ListByBuild returns a paginated list of defects observed in a specific build.
func (ds *DefectStore) ListByBuild(ctx context.Context, projectID string, buildID int64, filter store.DefectFilter) ([]store.DefectListRow, int, error) {
	return ds.listDefects(ctx, projectID, &buildID, filter)
}

// GetByID retrieves a single defect fingerprint by its UUID.
func (ds *DefectStore) GetByID(ctx context.Context, defectID string) (*store.DefectFingerprint, error) {
	var fp store.DefectFingerprint
	err := ds.pool.QueryRow(ctx, `
		SELECT id, project_id, fingerprint_hash, normalized_message, sample_trace,
		       category, resolution, known_issue_id, first_seen_build_id, last_seen_build_id,
		       occurrence_count, consecutive_clean_builds,
		       created_at::text, updated_at::text
		FROM defect_fingerprints
		WHERE id = $1`, defectID,
	).Scan(&fp.ID, &fp.ProjectID, &fp.FingerprintHash, &fp.NormalizedMessage, &fp.SampleTrace,
		&fp.Category, &fp.Resolution, &fp.KnownIssueID, &fp.FirstSeenBuildID, &fp.LastSeenBuildID,
		&fp.OccurrenceCount, &fp.ConsecutiveCleanBuilds, &fp.CreatedAt, &fp.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", store.ErrDefectNotFound, defectID)
	}
	if err != nil {
		return nil, fmt.Errorf("get defect by id: %w", err)
	}
	return &fp, nil
}

// GetTestResults returns paginated test results linked to a defect, optionally
// scoped to a build.
func (ds *DefectStore) GetTestResults(ctx context.Context, defectID string, buildID *int64, page, perPage int) ([]store.TestResult, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	where := "WHERE tr.defect_fingerprint_id = $1"
	args := []any{defectID}
	argIdx := 2

	if buildID != nil {
		where += fmt.Sprintf(" AND tr.build_id = $%d", argIdx)
		args = append(args, *buildID)
		argIdx++
	}

	// Count.
	var total int
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM test_results tr %s", where)
	if err := ds.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count defect test results: %w", err)
	}

	// Data.
	offset := (page - 1) * perPage
	dataQ := fmt.Sprintf(`
		SELECT tr.build_id, tr.project_id, tr.test_name, tr.full_name,
		       tr.status, tr.history_id, tr.duration_ms, tr.flaky, tr.retries,
		       tr.new_failed, tr.new_passed, tr.start_ms, tr.stop_ms,
		       tr.thread, tr.host
		FROM test_results tr
		%s
		ORDER BY tr.id DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := ds.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list defect test results: %w", err)
	}
	defer rows.Close()

	var result []store.TestResult
	for rows.Next() {
		var tr store.TestResult
		if err := rows.Scan(
			&tr.BuildID, &tr.ProjectID, &tr.TestName, &tr.FullName,
			&tr.Status, &tr.HistoryID, &tr.DurationMs, &tr.Flaky, &tr.Retries,
			&tr.NewFailed, &tr.NewPassed, &tr.StartMs, &tr.StopMs,
			&tr.Thread, &tr.Host,
		); err != nil {
			return nil, 0, fmt.Errorf("scan defect test result: %w", err)
		}
		result = append(result, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate defect test results: %w", err)
	}
	if result == nil {
		result = []store.TestResult{}
	}
	return result, total, nil
}

// GetProjectSummary returns aggregated defect counts for a project:
// count by resolution + count open by category.
func (ds *DefectStore) GetProjectSummary(ctx context.Context, projectID string) (*store.DefectProjectSummary, error) {
	summary := &store.DefectProjectSummary{
		ByCategory: make(map[string]int),
	}

	// Count by resolution.
	const resQ = `
		SELECT resolution, COUNT(*)
		FROM defect_fingerprints
		WHERE project_id = $1
		GROUP BY resolution`
	rows, err := ds.pool.Query(ctx, resQ, projectID)
	if err != nil {
		return nil, fmt.Errorf("project summary resolutions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var res string
		var cnt int
		if err := rows.Scan(&res, &cnt); err != nil {
			return nil, fmt.Errorf("scan project summary resolution: %w", err)
		}
		switch res {
		case store.DefectResolutionOpen:
			summary.Open = cnt
		case store.DefectResolutionFixed:
			summary.Fixed = cnt
		case store.DefectResolutionMuted:
			summary.Muted = cnt
		case store.DefectResolutionWontFix:
			summary.WontFix = cnt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project summary resolutions: %w", err)
	}
	rows.Close()

	// Count open by category.
	const catQ = `
		SELECT category, COUNT(*)
		FROM defect_fingerprints
		WHERE project_id = $1 AND resolution = 'open'
		GROUP BY category`
	catRows, err := ds.pool.Query(ctx, catQ, projectID)
	if err != nil {
		return nil, fmt.Errorf("project summary categories: %w", err)
	}
	defer catRows.Close()
	for catRows.Next() {
		var cat string
		var cnt int
		if err := catRows.Scan(&cat, &cnt); err != nil {
			return nil, fmt.Errorf("scan project summary category: %w", err)
		}
		summary.ByCategory[cat] = cnt
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project summary categories: %w", err)
	}

	// Regressions in the last build.
	const regQ = `
		WITH latest AS (
			SELECT id FROM builds WHERE project_id = $1 ORDER BY build_order DESC LIMIT 1
		),
		prev AS (
			SELECT id FROM builds WHERE project_id = $1
			  AND build_order < (SELECT build_order FROM builds WHERE id = (SELECT id FROM latest))
			ORDER BY build_order DESC LIMIT 1
		)
		SELECT COUNT(*)
		FROM defect_fingerprints df
		JOIN defect_occurrences do_cur ON do_cur.defect_fingerprint_id = df.id
		  AND do_cur.build_id = (SELECT id FROM latest)
		WHERE df.project_id = $1
		  AND df.first_seen_build_id != (SELECT id FROM latest)
		  AND NOT EXISTS (
			SELECT 1 FROM defect_occurrences do_prev
			WHERE do_prev.defect_fingerprint_id = df.id
			  AND do_prev.build_id = (SELECT id FROM prev)
		  )`
	_ = ds.pool.QueryRow(ctx, regQ, projectID).Scan(&summary.RegressionsLastBuild)

	return summary, nil
}

// GetBuildSummary returns aggregated defect counts for a single build.
func (ds *DefectStore) GetBuildSummary(ctx context.Context, projectID string, buildID int64) (*store.DefectBuildSummary, error) {
	summary := &store.DefectBuildSummary{
		ByCategory:   make(map[string]int),
		ByResolution: make(map[string]int),
	}

	// Total groups and affected tests.
	const totQ = `
		SELECT COUNT(DISTINCT do.defect_fingerprint_id), COALESCE(SUM(do.test_result_count), 0)
		FROM defect_occurrences do
		JOIN defect_fingerprints df ON df.id = do.defect_fingerprint_id
		WHERE df.project_id = $1 AND do.build_id = $2`
	if err := ds.pool.QueryRow(ctx, totQ, projectID, buildID).Scan(&summary.TotalGroups, &summary.AffectedTests); err != nil {
		return nil, fmt.Errorf("build summary totals: %w", err)
	}

	// New defects (first_seen_build_id = queried build).
	const newQ = `
		SELECT COUNT(*)
		FROM defect_fingerprints df
		JOIN defect_occurrences do ON do.defect_fingerprint_id = df.id AND do.build_id = $2
		WHERE df.project_id = $1 AND df.first_seen_build_id = $2`
	if err := ds.pool.QueryRow(ctx, newQ, projectID, buildID).Scan(&summary.NewDefects); err != nil {
		return nil, fmt.Errorf("build summary new defects: %w", err)
	}

	// Regressions.
	regressions, err := ds.DetectRegressions(ctx, projectID, buildID)
	if err != nil {
		return nil, fmt.Errorf("build summary regressions: %w", err)
	}
	summary.Regressions = len(regressions)

	// Category breakdown.
	const catQ = `
		SELECT df.category, COUNT(*)
		FROM defect_fingerprints df
		JOIN defect_occurrences do ON do.defect_fingerprint_id = df.id AND do.build_id = $2
		WHERE df.project_id = $1
		GROUP BY df.category`
	catRows, err := ds.pool.Query(ctx, catQ, projectID, buildID)
	if err != nil {
		return nil, fmt.Errorf("build summary categories: %w", err)
	}
	defer catRows.Close()
	for catRows.Next() {
		var cat string
		var cnt int
		if err := catRows.Scan(&cat, &cnt); err != nil {
			return nil, fmt.Errorf("scan build summary category: %w", err)
		}
		summary.ByCategory[cat] = cnt
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate build summary categories: %w", err)
	}
	catRows.Close()

	// Resolution breakdown.
	const resQ = `
		SELECT df.resolution, COUNT(*)
		FROM defect_fingerprints df
		JOIN defect_occurrences do ON do.defect_fingerprint_id = df.id AND do.build_id = $2
		WHERE df.project_id = $1
		GROUP BY df.resolution`
	resRows, err := ds.pool.Query(ctx, resQ, projectID, buildID)
	if err != nil {
		return nil, fmt.Errorf("build summary resolutions: %w", err)
	}
	defer resRows.Close()
	for resRows.Next() {
		var res string
		var cnt int
		if err := resRows.Scan(&res, &cnt); err != nil {
			return nil, fmt.Errorf("scan build summary resolution: %w", err)
		}
		summary.ByResolution[res] = cnt
	}
	if err := resRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate build summary resolutions: %w", err)
	}

	return summary, nil
}

// UpdateDefect updates the category, resolution, or known-issue link for a
// single defect. Only non-nil fields are set. When knownIssueID is 0 the
// known_issue_id column is set to NULL.
func (ds *DefectStore) UpdateDefect(ctx context.Context, defectID string, category, resolution *string, knownIssueID *int64) error {
	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
	argIdx := 1

	if category != nil {
		setClauses = append(setClauses, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *category)
		argIdx++
	}
	if resolution != nil {
		setClauses = append(setClauses, fmt.Sprintf("resolution = $%d", argIdx))
		args = append(args, *resolution)
		argIdx++
	}
	if knownIssueID != nil {
		if *knownIssueID == 0 {
			setClauses = append(setClauses, "known_issue_id = NULL")
		} else {
			setClauses = append(setClauses, fmt.Sprintf("known_issue_id = $%d", argIdx))
			args = append(args, *knownIssueID)
			argIdx++
		}
	}

	// If only updated_at was set, nothing to do but verify it exists.
	q := fmt.Sprintf("UPDATE defect_fingerprints SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, defectID)

	tag, err := ds.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update defect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", store.ErrDefectNotFound, defectID)
	}
	return nil
}

// BulkUpdate applies category and/or resolution changes to multiple defects atomically.
func (ds *DefectStore) BulkUpdate(ctx context.Context, defectIDs []string, category, resolution *string) error {
	if len(defectIDs) == 0 {
		return nil
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
	argIdx := 1

	if category != nil {
		setClauses = append(setClauses, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *category)
		argIdx++
	}
	if resolution != nil {
		setClauses = append(setClauses, fmt.Sprintf("resolution = $%d", argIdx))
		args = append(args, *resolution)
		argIdx++
	}

	q := fmt.Sprintf("UPDATE defect_fingerprints SET %s WHERE id = ANY($%d)",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, defectIDs)

	if _, err := ds.pool.Exec(ctx, q, args...); err != nil {
		return fmt.Errorf("bulk update defects: %w", err)
	}
	return nil
}
