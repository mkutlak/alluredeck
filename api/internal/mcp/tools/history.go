package tools

import (
	"context"
	"errors"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ---------------------------------------------------------------------------
// get_test_failure
// ---------------------------------------------------------------------------

// GetTestFailureInput holds parameters for get_test_failure.
type GetTestFailureInput struct {
	ProjectID int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	BuildID   int64  `json:"build_id" jsonschema:"Internal build id from resolve_url or list_recent_builds. The build_number in a UI URL is NOT the build_id."`
	HistoryID string `json:"history_id" jsonschema:"Cross-build test identifier. Obtain it from find_test_by_name, list_failing_tests or diagnose_failure; never construct one."`
}

// AttachmentRef is a lightweight reference to an attachment used in tool output.
type AttachmentRef struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Mime        string `json:"mime"`
	SizeBytes   int64  `json:"size_bytes"`
	ResourceURI string `json:"resource_uri"`
}

// CIInfo holds CI metadata included in get_test_failure output.
type CIInfo struct {
	CommitSHA   string `json:"commit_sha,omitempty"`
	Branch      string `json:"branch,omitempty"`
	PipelineURL string `json:"pipeline_url,omitempty"`
}

// FingerprintInfo holds defect fingerprint data included in get_test_failure
// and diagnose_failure output.
type FingerprintInfo struct {
	Hash     string `json:"hash"`
	Category string `json:"category"`
	// OccurrenceCount is how many times this fingerprint has been seen across
	// the project's builds — the difference between a one-off and a standing
	// defect.
	OccurrenceCount int `json:"occurrence_count,omitempty"`
	// Resolution is the triage verdict recorded for the fingerprint, if any.
	Resolution string `json:"resolution,omitempty"`
	// FirstSeenBuildID is the build where the fingerprint first appeared; it
	// bounds any bisect for the change that introduced the defect.
	FirstSeenBuildID int64 `json:"first_seen_build_id,omitempty"`
}

// KnownIssueRef holds a matched known issue reference.
type KnownIssueRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GetTestFailureOutput is the structured output for get_test_failure.
type GetTestFailureOutput struct {
	Status        string            `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	StatusTrace   string            `json:"status_trace,omitempty"`
	DurationMs    int64             `json:"duration_ms"`
	Attachments   []AttachmentRef   `json:"attachments"`
	CI            *CIInfo           `json:"ci,omitempty"`
	Fingerprint   *FingerprintInfo  `json:"fingerprint,omitempty"`
	KnownIssue    *KnownIssueRef    `json:"known_issue,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
}

// ---------------------------------------------------------------------------
// get_test_history
// ---------------------------------------------------------------------------

// GetTestHistoryInput holds parameters for get_test_history.
type GetTestHistoryInput struct {
	ProjectID int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	HistoryID string `json:"history_id" jsonschema:"Cross-build test identifier. Obtain it from find_test_by_name, list_failing_tests or diagnose_failure; never construct one."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of runs to return, most recent first. Defaults to 20, clamped to 100. Older runs exist when the item count equals the limit."`
	// Cursor is accepted and ignored. The underlying history query has no
	// offset pagination, so the cursor never advanced; it is kept only so
	// existing callers do not break on an unknown-argument error.
	Cursor string `json:"cursor,omitempty" jsonschema:"Deprecated: accepted but ignored. This tool has no cursor pagination; raise limit instead."`
	Branch string `json:"branch,omitempty" jsonschema:"Restrict the history to a single branch by name. An unknown branch returns an empty list rather than an error."`
}

// TestHistoryItem is one entry in the get_test_history response.
type TestHistoryItem struct {
	BuildID     int64  `json:"build_id"`
	BuildNumber int    `json:"build_number"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
	// Branch names the branch the run came from. Empty for builds recorded
	// before branch tracking, or when CI supplied no branch. Without it a
	// cross-branch history reads as one timeline and status flips look like
	// regressions when they are only a different line of development.
	Branch string `json:"branch,omitempty"`
}

// GetTestHistoryOutput is the structured output for get_test_history.
//
// There is no next_cursor: the underlying query has no offset pagination, so
// the cursor this tool used to emit could never be honoured. A caller detects
// that older runs exist by len(items) == limit.
type GetTestHistoryOutput struct {
	Items []TestHistoryItem `json:"items"`
}

// ---------------------------------------------------------------------------
// compare_builds
// ---------------------------------------------------------------------------

// CompareBuildsInput holds parameters for compare_builds.
type CompareBuildsInput struct {
	ProjectID     int   `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	BaseBuildID   int64 `json:"base_build_id" jsonschema:"Internal build id of the earlier (baseline) build. The build_number in a UI URL is NOT the build_id."`
	TargetBuildID int64 `json:"target_build_id" jsonschema:"Internal build id of the later build being compared against the baseline."`
	// Format controls output verbosity:
	//   "full"    (default) — current shape with all fields
	//   "compact" — omit history_id and test_name (~50% token reduction)
	//   "summary" — counts only (~95% token reduction)
	Format string `json:"format,omitempty" jsonschema:"Output verbosity: full (default, all fields), compact (omit history_id and test_name), or summary (counts only). Any other value is rejected."`
}

// DiffItem is one test in a compare_builds diff list.
type DiffItem struct {
	TestName  string `json:"test_name,omitempty"`
	FullName  string `json:"full_name"`
	HistoryID string `json:"history_id,omitempty"`
	StatusA   string `json:"status_a,omitempty"`
	StatusB   string `json:"status_b,omitempty"`
}

// CompareSummary holds counts-only output for format="summary".
type CompareSummary struct {
	Regressed int `json:"regressed"`
	Fixed     int `json:"fixed"`
	NewPassed int `json:"new_passed"`
	NewFailed int `json:"new_failed"`
	Removed   int `json:"removed"`
}

// BranchMismatchWarning is populated in compare_builds output when the base and
// target builds belong to different branches. The diff is still returned; the
// warning exists so callers know regressions may reflect branch differences.
type BranchMismatchWarning struct {
	BaseBranch   string `json:"base_branch"`
	TargetBranch string `json:"target_branch"`
	Message      string `json:"message"`
}

// CompareBuildsOutput is the structured output for compare_builds.
type CompareBuildsOutput struct {
	Regressed      []DiffItem             `json:"regressed,omitempty"`
	Fixed          []DiffItem             `json:"fixed,omitempty"`
	NewPassed      []DiffItem             `json:"new_passed,omitempty"`
	NewFailed      []DiffItem             `json:"new_failed,omitempty"`
	Removed        []DiffItem             `json:"removed,omitempty"`
	Build          *BuildRef              `json:"build,omitempty"`
	Summary        *CompareSummary        `json:"summary,omitempty"`
	BranchMismatch *BranchMismatchWarning `json:"branch_mismatch,omitempty"`
}

// ---------------------------------------------------------------------------
// RegisterHistoryTools
// ---------------------------------------------------------------------------

// RegisterHistoryTools registers get_test_failure, get_test_history, and
// compare_builds on s.
func RegisterHistoryTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_test_failure",
		Title:       "Get AllureDeck test failure",
		Annotations: readOnlyAnnotations(),
		Description: "Get detailed information for one test in a build: status, message, stack trace, the attachments belonging to THAT test, CI context, defect fingerprint, and the test environment metadata (Allure environment.properties: base URLs, versions, and any debug links the CI recorded). Works for a test in any state, not only a failing one — a passing test returns its actual status rather than a not-found error. URL build_number is NOT build_id — call resolve_url first or use list_recent_builds.",
	}, getTestFailureHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_test_history",
		Title:       "Get AllureDeck test history",
		Annotations: readOnlyAnnotations(),
		Description: "Get the run history of a test across builds, most recent first. Shows status trends, duration, commit SHA, and the branch each run came from. Optional branch parameter filters history to a single branch (unknown branch names return empty results, not an error). There is no cursor pagination: when the item count equals `limit`, older runs may exist — raise `limit` to see them.",
	}, getTestHistoryHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "compare_builds",
		Title:       "Compare AllureDeck builds",
		Annotations: readOnlyAnnotations(),
		Description: "Compare two builds for a project. Returns tests that regressed, became fixed, appeared new, or were removed between the base and target builds. Supports format=full|compact|summary. When the two builds are from different branches, a branch_mismatch warning is included in the output — the diff is still returned but regressions may reflect branch differences rather than true regressions. URL build_number is NOT build_id — call resolve_url first or use list_recent_builds.",
	}, compareBuildsHandler(stores, logger))
}

func getTestFailureHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetTestFailureInput) (*mcpsdk.CallToolResult, GetTestFailureOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in GetTestFailureInput) (*mcpsdk.CallToolResult, GetTestFailureOutput, error) {
		if in.ProjectID <= 0 {
			return nil, GetTestFailureOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.BuildID <= 0 {
			return nil, GetTestFailureOutput{}, fmt.Errorf("build_id must be positive")
		}
		if in.HistoryID == "" {
			return nil, GetTestFailureOutput{}, fmt.Errorf("history_id must not be empty")
		}

		// Fetch the build row — this serves as both an existence check and the
		// source of CI metadata. Using GetBuildByID avoids a separate BuildExists
		// round-trip and works for any build, not just the latest.
		build, err := stores.Build.GetBuildByID(ctx, int64(in.ProjectID), in.BuildID)
		if err != nil {
			return nil, GetTestFailureOutput{}, fmt.Errorf(
				"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map): %w",
				in.BuildID, in.ProjectID, err,
			)
		}

		// Look the row up directly by its key. The previous implementation
		// listed the build's first 1000 FAILING tests and scanned them
		// linearly, so a passing test was reported as absent and a build with
		// more than 1000 failures could lose the row entirely.
		matched, err := stores.TestResult.GetByHistoryID(ctx, int64(in.ProjectID), in.BuildID, in.HistoryID)
		if err != nil {
			return nil, GetTestFailureOutput{}, fmt.Errorf("fetching test result: %w", err)
		}
		if matched == nil {
			return nil, GetTestFailureOutput{}, fmt.Errorf("test with history_id %q not found in build %d", in.HistoryID, in.BuildID)
		}

		out := GetTestFailureOutput{
			Status:        matched.Status,
			StatusMessage: matched.StatusMessage,
			DurationMs:    matched.DurationMs,
		}

		// Attachments scoped to THIS test result, not the whole build. The
		// previous ListByBuild call handed every test in the build the same
		// build-wide attachment list, so a screenshot from an unrelated test
		// looked like evidence for this one.
		attachments, err := stores.Attachment.ListByTestResult(ctx, int64(in.ProjectID), in.BuildID, in.HistoryID, testFailureAttachmentsLimit)
		if err != nil {
			// Non-fatal: continue without attachments.
			attachments = nil
		}
		out.Attachments = make([]AttachmentRef, 0, len(attachments))
		for _, a := range attachments {
			out.Attachments = append(out.Attachments, attachmentToRef(a))
		}

		// Populate CI metadata from the already-fetched build row.
		ci := &CIInfo{}
		if build.CICommitSHA != nil {
			ci.CommitSHA = *build.CICommitSHA
		}
		if build.CIBranch != nil {
			ci.Branch = *build.CIBranch
		}
		if build.CIPipelineURL != nil {
			ci.PipelineURL = *build.CIPipelineURL
		}
		if ci.CommitSHA != "" || ci.Branch != "" || ci.PipelineURL != "" {
			out.CI = ci
		}

		// Populate environment metadata from the already-fetched build row.
		out.Environment = build.Environment

		// Fetch defect fingerprint via the test_results.defect_fingerprint_id FK.
		// history_id is the cross-build test identifier, NOT a fingerprint hash,
		// so it must not be passed to GetByHash. Instead resolve the linked
		// fingerprint UUID from the test row, then load the fingerprint by ID.
		if fpID, err := stores.TestResult.GetDefectFingerprintID(ctx, int64(in.ProjectID), in.BuildID, in.HistoryID); err == nil && fpID != nil {
			if defect, err := stores.Defect.GetByID(ctx, *fpID); err == nil && defect != nil {
				out.Fingerprint = &FingerprintInfo{
					Hash:     defect.FingerprintHash,
					Category: defect.Category,
				}
				if defect.KnownIssueID != nil {
					ki, err := stores.KnownIssue.Get(ctx, *defect.KnownIssueID)
					if err == nil && ki != nil {
						out.KnownIssue = &KnownIssueRef{
							ID:   ki.ID,
							Name: ki.TestName,
						}
					}
				}
			}
		}

		digest := fmt.Sprintf("%s: %s in build %d (%dms), %d attachment(s)",
			matched.FullName, matched.Status, in.BuildID, matched.DurationMs, len(out.Attachments))
		return textResult(digest), out, nil
	}
}

// testFailureAttachmentsLimit caps how many attachment refs get_test_failure
// returns for a single test — ample for any realistic test while bounding the
// output size.
const testFailureAttachmentsLimit = 200

func attachmentToRef(a store.TestAttachment) AttachmentRef {
	return AttachmentRef{
		ID:          a.ID,
		Name:        a.Name,
		Mime:        a.MimeType,
		SizeBytes:   a.SizeBytes,
		ResourceURI: fmt.Sprintf("alluredeck://attachment/%d", a.ID),
	}
}

func getTestHistoryHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in GetTestHistoryInput) (*mcpsdk.CallToolResult, GetTestHistoryOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in GetTestHistoryInput) (*mcpsdk.CallToolResult, GetTestHistoryOutput, error) {
		if in.ProjectID <= 0 {
			return nil, GetTestHistoryOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.HistoryID == "" {
			return nil, GetTestHistoryOutput{}, fmt.Errorf("history_id must not be empty")
		}
		if in.Limit <= 0 {
			in.Limit = 20
		}
		if in.Limit > 100 {
			in.Limit = 100
		}

		// in.Cursor is deliberately ignored: GetTestHistory has no offset
		// pagination, so the cursor this tool used to emit could never move the
		// window. It stays in the input schema, marked deprecated, so existing
		// callers do not fail on an unknown argument.

		var branchID *int64
		if in.Branch != "" {
			br, err := stores.Branch.GetByName(ctx, int64(in.ProjectID), in.Branch)
			if err != nil {
				// A genuinely absent branch is not a user mistake (the caller may
				// be probing) — return empty, not an error. Any other error is an
				// infrastructure failure and must surface, otherwise a transient DB
				// fault masquerades as "no history on this branch".
				if errors.Is(err, store.ErrBranchNotFound) {
					return textResult(fmt.Sprintf("branch %q not found in project %d; no history", in.Branch, in.ProjectID)),
						GetTestHistoryOutput{Items: nil}, nil
				}
				return nil, GetTestHistoryOutput{}, fmt.Errorf("resolving branch %q: %w", in.Branch, err)
			}
			branchID = &br.ID
		}

		// Fetch exactly limit rows: without a cursor there is no next page to
		// probe for, and len(items) == limit already tells the caller older
		// runs may exist.
		entries, err := stores.TestResult.GetTestHistory(ctx, int64(in.ProjectID), in.HistoryID, branchID, in.Limit)
		if err != nil {
			return nil, GetTestHistoryOutput{}, fmt.Errorf("fetching test history: %w", err)
		}

		items := make([]TestHistoryItem, len(entries))
		for i, e := range entries {
			item := TestHistoryItem{
				BuildID:     e.BuildID,
				BuildNumber: e.BuildNumber,
				Status:      e.Status,
				DurationMs:  e.DurationMs,
				CreatedAt:   e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				Branch:      e.BranchName,
			}
			if e.CICommitSHA != nil {
				item.CommitSHA = *e.CICommitSHA
			}
			items[i] = item
		}

		digest := fmt.Sprintf("%d run(s) for history_id %s", len(items), in.HistoryID)
		if in.Branch != "" {
			digest += " on branch " + in.Branch
		}
		if len(items) == in.Limit {
			digest += fmt.Sprintf(" (limit %d reached; older runs may exist)", in.Limit)
		}
		return textResult(digest), GetTestHistoryOutput{Items: items}, nil
	}
}

func compareBuildsHandler(stores *bootstrap.Stores, _ *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in CompareBuildsInput) (*mcpsdk.CallToolResult, CompareBuildsOutput, error) {
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in CompareBuildsInput) (*mcpsdk.CallToolResult, CompareBuildsOutput, error) {
		if in.ProjectID <= 0 {
			return nil, CompareBuildsOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.BaseBuildID <= 0 {
			return nil, CompareBuildsOutput{}, fmt.Errorf("base_build_id must be positive")
		}
		if in.TargetBuildID <= 0 {
			return nil, CompareBuildsOutput{}, fmt.Errorf("target_build_id must be positive")
		}
		// Reject an unknown format rather than silently falling through to the
		// full shape: a caller that asked for "brief" and got the full payload
		// has no way to notice it paid for 20x the tokens it wanted.
		switch in.Format {
		case "", "full", "compact", "summary":
		default:
			return nil, CompareBuildsOutput{}, fmt.Errorf(
				"format must be one of full, compact, summary (got %q)", in.Format)
		}

		// Fetch base build row — serves as both existence check and source of
		// branch info for the cross-branch warning.
		baseBuild, err := stores.Build.GetBuildByID(ctx, int64(in.ProjectID), in.BaseBuildID)
		if err != nil {
			if errors.Is(err, store.ErrBuildNotFound) {
				return nil, CompareBuildsOutput{}, fmt.Errorf(
					"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map)",
					in.BaseBuildID, in.ProjectID,
				)
			}
			return nil, CompareBuildsOutput{}, fmt.Errorf("fetching base build %d: %w", in.BaseBuildID, err)
		}

		// Fetch the target build row — serves as both existence check and the
		// source for the Build field in the output (post-state context).
		// Build is the TARGET because that is what the LLM needs to anchor further
		// calls (e.g. list_failing_tests for the regressions just found).
		targetBuild, err := stores.Build.GetBuildByID(ctx, int64(in.ProjectID), in.TargetBuildID)
		if err != nil {
			if errors.Is(err, store.ErrBuildNotFound) {
				return nil, CompareBuildsOutput{}, fmt.Errorf(
					"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map)",
					in.TargetBuildID, in.ProjectID,
				)
			}
			return nil, CompareBuildsOutput{}, fmt.Errorf("fetching target build %d: %w", in.TargetBuildID, err)
		}

		// Build the top-level BuildRef from the target build so the caller has
		// build_number/branch/commit without a separate list_recent_builds call.
		targetRef := &BuildRef{BuildNumber: targetBuild.BuildNumber}
		if targetBuild.CIBranch != nil {
			targetRef.Branch = *targetBuild.CIBranch
		}
		if targetBuild.CICommitSHA != nil {
			targetRef.CommitSHA = *targetBuild.CICommitSHA
		}
		if targetBuild.CIPipelineID != nil {
			targetRef.CIPipelineID = *targetBuild.CIPipelineID
		}
		if targetBuild.CIPipelineURL != nil {
			targetRef.CIPipelineURL = *targetBuild.CIPipelineURL
		}

		// Warn only when BOTH branches are known and differ. A NULL branch_id
		// (builds predating branch tracking, or CI that supplied no branch) is
		// deliberately treated as "no mismatch" rather than warning on every
		// comparison that touches a legacy build — those warnings would be noise,
		// not signal. The mismatch warning fires only when we are certain.
		var branchMismatch *BranchMismatchWarning
		if baseBuild.BranchID != nil && targetBuild.BranchID != nil && *baseBuild.BranchID != *targetBuild.BranchID {
			baseBranch := ""
			if baseBuild.CIBranch != nil {
				baseBranch = *baseBuild.CIBranch
			}
			targetBranch := ""
			if targetBuild.CIBranch != nil {
				targetBranch = *targetBuild.CIBranch
			}
			branchMismatch = &BranchMismatchWarning{
				BaseBranch:   baseBranch,
				TargetBranch: targetBranch,
				Message:      "Builds are from different branches; regressions may reflect branch differences rather than true regressions.",
			}
		}

		diffs, err := stores.TestResult.CompareBuildsByHistoryID(ctx, int64(in.ProjectID), in.BaseBuildID, in.TargetBuildID)
		if err != nil {
			return nil, CompareBuildsOutput{}, fmt.Errorf("comparing builds: %w", err)
		}

		// Normalise format: default is "full".
		format := in.Format
		if format == "" {
			format = "full"
		}

		// summary mode: counts only, no per-test lists.
		if format == "summary" {
			out := CompareBuildsOutput{Build: targetRef, BranchMismatch: branchMismatch}
			var regressed, fixed, newPassed, newFailed, removed int
			for _, d := range diffs {
				switch d.Category {
				case store.DiffRegressed:
					regressed++
				case store.DiffFixed:
					fixed++
				case store.DiffAdded:
					if d.StatusB == string(store.TestStatusPassed) {
						newPassed++
					} else {
						newFailed++
					}
				case store.DiffRemoved:
					removed++
				}
			}
			out.Summary = &CompareSummary{
				Regressed: regressed,
				Fixed:     fixed,
				NewPassed: newPassed,
				NewFailed: newFailed,
				Removed:   removed,
			}
			return textResult(compareDigest(in, regressed, fixed, newPassed+newFailed, removed, branchMismatch != nil)), out, nil
		}

		// full or compact mode: build per-test lists.
		out := CompareBuildsOutput{
			Build:          targetRef,
			BranchMismatch: branchMismatch,
			Regressed:      []DiffItem{},
			Fixed:          []DiffItem{},
			NewPassed:      []DiffItem{},
			NewFailed:      []DiffItem{},
			Removed:        []DiffItem{},
		}

		for _, d := range diffs {
			item := diffItemFromEntry(d, format)
			switch d.Category {
			case store.DiffRegressed:
				out.Regressed = append(out.Regressed, item)
			case store.DiffFixed:
				out.Fixed = append(out.Fixed, item)
			case store.DiffAdded:
				if d.StatusB == string(store.TestStatusPassed) {
					out.NewPassed = append(out.NewPassed, item)
				} else {
					out.NewFailed = append(out.NewFailed, item)
				}
			case store.DiffRemoved:
				out.Removed = append(out.Removed, item)
			}
		}

		digest := compareDigest(in, len(out.Regressed), len(out.Fixed),
			len(out.NewPassed)+len(out.NewFailed), len(out.Removed), branchMismatch != nil)
		return textResult(digest), out, nil
	}
}

// compareDigest renders the one-line unstructured summary of a build
// comparison. It names the mismatch explicitly because a cross-branch diff's
// "regressions" are frequently not regressions at all.
func compareDigest(in CompareBuildsInput, regressed, fixed, added, removed int, branchMismatch bool) string {
	d := fmt.Sprintf("build %d vs %d: %d regressed, %d fixed, %d new, %d removed",
		in.BaseBuildID, in.TargetBuildID, regressed, fixed, added, removed)
	if branchMismatch {
		d += "; WARNING different branches"
	}
	return d
}

// diffItemFromEntry converts a store.DiffEntry to a DiffItem, honouring the
// requested format ("full" keeps all fields; "compact" omits history_id and
// test_name since full_name already contains the test name).
func diffItemFromEntry(d store.DiffEntry, format string) DiffItem {
	item := DiffItem{
		FullName: d.FullName,
		StatusA:  d.StatusA,
		StatusB:  d.StatusB,
	}
	if format != "compact" {
		item.TestName = d.TestName
		item.HistoryID = d.HistoryID
	}
	return item
}
