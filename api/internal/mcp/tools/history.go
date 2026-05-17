package tools

import (
	"context"
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
	ProjectID int    `json:"project_id"`
	BuildID   int64  `json:"build_id"`
	HistoryID string `json:"history_id"`
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

// FingerprintInfo holds defect fingerprint data included in get_test_failure output.
type FingerprintInfo struct {
	Hash     string `json:"hash"`
	Category string `json:"category"`
}

// KnownIssueRef holds a matched known issue reference.
type KnownIssueRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GetTestFailureOutput is the structured output for get_test_failure.
type GetTestFailureOutput struct {
	Status        string           `json:"status"`
	StatusMessage string           `json:"status_message,omitempty"`
	StatusTrace   string           `json:"status_trace,omitempty"`
	DurationMs    int64            `json:"duration_ms"`
	Attachments   []AttachmentRef  `json:"attachments"`
	CI            *CIInfo          `json:"ci,omitempty"`
	Fingerprint   *FingerprintInfo `json:"fingerprint,omitempty"`
	KnownIssue    *KnownIssueRef   `json:"known_issue,omitempty"`
}

// ---------------------------------------------------------------------------
// get_test_history
// ---------------------------------------------------------------------------

// GetTestHistoryInput holds parameters for get_test_history.
type GetTestHistoryInput struct {
	ProjectID int    `json:"project_id"`
	HistoryID string `json:"history_id"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

// TestHistoryItem is one entry in the get_test_history response.
type TestHistoryItem struct {
	BuildID     int64  `json:"build_id"`
	BuildNumber int    `json:"build_number"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// GetTestHistoryOutput is the structured output for get_test_history.
type GetTestHistoryOutput struct {
	Items      []TestHistoryItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// ---------------------------------------------------------------------------
// compare_builds
// ---------------------------------------------------------------------------

// CompareBuildsInput holds parameters for compare_builds.
type CompareBuildsInput struct {
	ProjectID     int   `json:"project_id"`
	BaseBuildID   int64 `json:"base_build_id"`
	TargetBuildID int64 `json:"target_build_id"`
	// Format controls output verbosity:
	//   "full"    (default) — current shape with all fields
	//   "compact" — omit history_id and test_name (~50% token reduction)
	//   "summary" — counts only (~95% token reduction)
	Format string `json:"format,omitempty"`
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

// CompareBuildsOutput is the structured output for compare_builds.
type CompareBuildsOutput struct {
	Regressed []DiffItem      `json:"regressed,omitempty"`
	Fixed     []DiffItem      `json:"fixed,omitempty"`
	NewPassed []DiffItem      `json:"new_passed,omitempty"`
	NewFailed []DiffItem      `json:"new_failed,omitempty"`
	Removed   []DiffItem      `json:"removed,omitempty"`
	Build     *BuildRef       `json:"build,omitempty"`
	Summary   *CompareSummary `json:"summary,omitempty"`
}

// ---------------------------------------------------------------------------
// RegisterHistoryTools
// ---------------------------------------------------------------------------

// RegisterHistoryTools registers get_test_failure, get_test_history, and
// compare_builds on s.
func RegisterHistoryTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_test_failure",
		Description: "Get detailed failure information for a specific test in a build: status, message, stack trace, attachments, CI context, and defect fingerprint. URL build_number is NOT build_id — call resolve_url first or use list_recent_builds.",
	}, getTestFailureHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "get_test_history",
		Description: "Get the run history of a test across builds. Shows status trends, duration, and commit SHA per build.",
	}, getTestHistoryHandler(stores, logger))

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "compare_builds",
		Description: "Compare two builds for a project. Returns tests that regressed, became fixed, appeared new, or were removed between the base and target builds. Supports format=full|compact|summary. URL build_number is NOT build_id — call resolve_url first or use list_recent_builds.",
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

		// Fetch failing tests for this build and find the one matching history_id.
		rows, err := stores.TestResult.ListFailedByBuild(ctx, int64(in.ProjectID), in.BuildID, 1000)
		if err != nil {
			return nil, GetTestFailureOutput{}, fmt.Errorf("fetching test results: %w", err)
		}

		var matched *store.TestResult
		for i := range rows {
			if rows[i].HistoryID == in.HistoryID {
				matched = &rows[i]
				break
			}
		}
		if matched == nil {
			return nil, GetTestFailureOutput{}, fmt.Errorf("test with history_id %q not found in build %d", in.HistoryID, in.BuildID)
		}

		out := GetTestFailureOutput{
			Status:     matched.Status,
			DurationMs: matched.DurationMs,
		}

		// Fetch attachments for this build.
		attachments, _, err := stores.Attachment.ListByBuild(ctx, int64(in.ProjectID), in.BuildID, "", "", 200, 0)
		if err != nil {
			// Non-fatal: continue without attachments.
			attachments = nil
		}
		// attachments are scoped to the build, not per-test result;
		// allure-store association is not currently tracked at this call site.
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

		return nil, out, nil
	}
}

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

		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return nil, GetTestHistoryOutput{}, fmt.Errorf("invalid cursor: %w", err)
		}
		_ = offset // history store does not support offset pagination; cursor is for future use

		entries, err := stores.TestResult.GetTestHistory(ctx, int64(in.ProjectID), in.HistoryID, nil, in.Limit+1)
		if err != nil {
			return nil, GetTestHistoryOutput{}, fmt.Errorf("fetching test history: %w", err)
		}

		hasMore := len(entries) > in.Limit
		if hasMore {
			entries = entries[:in.Limit]
		}

		items := make([]TestHistoryItem, len(entries))
		for i, e := range entries {
			item := TestHistoryItem{
				BuildID:     e.BuildID,
				BuildNumber: e.BuildNumber,
				Status:      e.Status,
				DurationMs:  e.DurationMs,
				CreatedAt:   e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			}
			if e.CICommitSHA != nil {
				item.CommitSHA = *e.CICommitSHA
			}
			items[i] = item
		}

		var nextCursor string
		if hasMore {
			nextCursor = encodeCursor(offset + in.Limit)
		}

		return nil, GetTestHistoryOutput{Items: items, NextCursor: nextCursor}, nil
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

		// Validate base build belongs to this project.
		baseOK, err := stores.Build.BuildExists(ctx, int64(in.ProjectID), in.BaseBuildID)
		if err != nil {
			return nil, CompareBuildsOutput{}, fmt.Errorf("checking base build existence: %w", err)
		}
		if !baseOK {
			return nil, CompareBuildsOutput{}, fmt.Errorf(
				"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map)",
				in.BaseBuildID, in.ProjectID,
			)
		}

		// Fetch the target build row — serves as both existence check and the
		// source for the Build field in the output (post-state context).
		// Build is the TARGET because that is what the LLM needs to anchor further
		// calls (e.g. list_failing_tests for the regressions just found).
		targetBuild, err := stores.Build.GetBuildByID(ctx, int64(in.ProjectID), in.TargetBuildID)
		if err != nil {
			return nil, CompareBuildsOutput{}, fmt.Errorf(
				"build_id %d not found in project %d (hint: build_number from the UI URL is not build_id; use resolve_url to map): %w",
				in.TargetBuildID, in.ProjectID, err,
			)
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
			out := CompareBuildsOutput{Build: targetRef}
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
			return nil, out, nil
		}

		// full or compact mode: build per-test lists.
		out := CompareBuildsOutput{
			Build:     targetRef,
			Regressed: []DiffItem{},
			Fixed:     []DiffItem{},
			NewPassed: []DiffItem{},
			NewFailed: []DiffItem{},
			Removed:   []DiffItem{},
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

		return nil, out, nil
	}
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
