package tools

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/triage"
)

// ---------------------------------------------------------------------------
// diagnose_pipeline
//
// Playwright CI shards a suite across parallel jobs and each shard uploads its
// own build, all sharing one ci_pipeline_id within the same project (see
// store.Build.CIPipelineID). diagnose_failure only ever looks at one build, so
// an agent diagnosing a single shard's report never learns that three sibling
// shards also failed — or that all four failed the same way because a shared
// dependency (a staging API, a Docker image) broke for everyone at once.
// diagnose_pipeline finds every shard build under a pipeline ID, diagnoses each
// one compactly, and clusters the UNION of failures across all shards so a
// single root cause spanning several shards reads as ONE cluster instead of N.
//
// Unlike diagnose_failure, the multi-query per-test work (build history,
// last-good pointers and their whole-build diff, retry attempts, defect
// fingerprint and known-issue FK) stays OFF here: each is several queries per
// test, and running them once per test per shard would turn a four-shard
// pipeline into an N×M fan-out. Use diagnose_failure on the specific build+test
// once diagnose_pipeline has pointed you at it.
//
// The one per-test query that IS run is GetFailedStepPath: a single indexed
// lookup, bounded by max_tests_per_build per shard. It buys the deepest failed
// step's own error text and the failure phase, which is what clustering keys
// on — without it the clusters degrade to message-only grouping and every
// cluster reports an empty failure_phase and no status pattern, i.e. structure
// that looks populated but never is.
// ---------------------------------------------------------------------------

// diagnosePipelineDefaultMaxTests is the default cap on failing tests examined
// per shard build.
const diagnosePipelineDefaultMaxTests = 10

// diagnosePipelineAbsoluteMaxTests is the hard upper bound on
// max_tests_per_build.
const diagnosePipelineAbsoluteMaxTests = 50

// diagnosePipelineMaxBuilds caps how many shard builds are examined in one
// call. A pipeline id is normally shared by a handful of shards; a value large
// enough to hold a pathological (or mis-set) pipeline id is what keeps the
// per-test query fan-out bounded at maxBuilds × max_tests_per_build.
const diagnosePipelineMaxBuilds = 50

// diagnosePipelineMessageMax bounds a per-test error message in the output.
// ListFailedByBuild already caps status_message at this length at the query;
// the failed-step message used in preference to it is uncapped, so the cap is
// re-applied here to keep both sources to one budget.
const diagnosePipelineMessageMax = 500

// DiagnosePipelineInput holds parameters for the diagnose_pipeline tool.
type DiagnosePipelineInput struct {
	ProjectID  int    `json:"project_id" jsonschema:"Internal numeric project id. Call list_projects if you only have a project name."`
	PipelineID string `json:"pipeline_id" jsonschema:"CI pipeline identifier shared by every shard build (builds.ci_pipeline_id). Obtain it from list_recent_builds, resolve_url, diagnose_failure, list_failing_tests, or compare_builds — all now surface ci_pipeline_id on the build."`
	// MaxTestsPerBuild caps the number of failing tests examined per shard
	// build. Defaults to 10, clamped to 50.
	MaxTestsPerBuild int `json:"max_tests_per_build,omitempty" jsonschema:"Maximum number of failing tests examined per shard build. Defaults to 10, clamped to 50."`
}

// PipelineBuildStats is a shard build's headline test counts.
type PipelineBuildStats struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Broken int `json:"broken"`
}

// PipelineFailingTest is one failing test within a shard build's compact
// per-build block. It deliberately omits the heavier diagnose_failure fields
// (failed_step_path, signals, last_good, attachments, retries): those require
// per-test queries that would multiply across every shard's failures.
type PipelineFailingTest struct {
	FullName string `json:"full_name"`
	Status   string `json:"status"`
	// ErrorMessage is capped at 500 characters — the same cap
	// stores.TestResult.ListFailedByBuild applies at the query. Empty for a
	// non-representative cluster member; read the cluster's shared_error.
	ErrorMessage string `json:"error_message,omitempty"`
	// ClusterID names the cross-shard cluster (see
	// DiagnosePipelineOutput.Clusters) this test belongs to.
	ClusterID string `json:"cluster_id,omitempty"`
}

// PipelineBuildBlock is one shard build's compact diagnosis.
type PipelineBuildBlock struct {
	BuildID      int64                 `json:"build_id"`
	BuildNumber  int                   `json:"build_number"`
	Branch       string                `json:"branch,omitempty"`
	CreatedAt    string                `json:"created_at"`
	Stats        PipelineBuildStats    `json:"stats"`
	FailingTests []PipelineFailingTest `json:"failing_tests,omitempty"`
	// Truncated is true when this build has more failing tests than
	// max_tests_per_build examined in detail.
	Truncated bool `json:"truncated,omitempty"`
}

// PipelineTotals rolls up counts across every shard build in the pipeline.
type PipelineTotals struct {
	Builds       int `json:"builds"`
	FailedBuilds int `json:"failed_builds"`
	TotalTests   int `json:"total_tests"`
	FailedTests  int `json:"failed_tests"`
}

// DiagnosePipelineOutput is the structured output for diagnose_pipeline.
type DiagnosePipelineOutput struct {
	PipelineID  string               `json:"pipeline_id"`
	PipelineURL string               `json:"pipeline_url,omitempty"`
	Builds      []PipelineBuildBlock `json:"builds"`
	Totals      PipelineTotals       `json:"totals"`
	// TruncatedBuilds is true when the pipeline holds more shard builds than
	// the per-call cap examined here.
	TruncatedBuilds bool `json:"truncated_builds,omitempty"`
	// Warnings surfaces per-build anomalies (e.g. duplicate history_id twins
	// merged by full_name) so counts stay auditable, mirroring
	// diagnose_failure's warnings field.
	Warnings []string `json:"warnings,omitempty"`
	// Clusters groups the UNION of failing tests across every shard build by
	// root cause, largest cluster first — the same clustering diagnose_failure
	// uses (see clusterFailingTests). One environment outage spanning four
	// shards reads as ONE cluster here instead of four separate failure lists.
	Clusters []DiagnoseCluster `json:"clusters,omitempty"`
}

// RegisterPipelineTools registers the diagnose_pipeline tool on s.
func RegisterPipelineTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "diagnose_pipeline",
		Title:       "Diagnose AllureDeck CI pipeline",
		Annotations: readOnlyAnnotations(),
		Description: "Diagnose every shard build of a CI pipeline run in ONE call. Playwright CI shards a suite across parallel jobs and each shard uploads its OWN build under a shared ci_pipeline_id (builds.ci_pipeline_id) within the same project — diagnose_failure only ever sees one of those builds, so an agent that diagnoses a single shard's report never learns that sibling shards also failed, or that they all failed for the SAME reason. Prefer diagnose_pipeline over diagnose_failure whenever a build's ci_pipeline_id is non-empty (list_recent_builds, resolve_url, diagnose_failure, list_failing_tests, and compare_builds all now surface it) and you want the whole run's picture rather than one shard's. Returns `builds[]`, one compact block per shard {build_id, build_number, branch, created_at, stats, failing_tests, truncated}, `totals` {builds, failed_builds, total_tests, failed_tests} summed across every shard, and `clusters` — the UNION of every shard's failing tests grouped by root cause (same clustering diagnose_failure uses), so one environment outage spanning four shards reads as ONE cluster instead of four separate failure lists. Per-test detail is intentionally thin here: full_name, status, and a 500-char-capped error_message, with per-test triage, last-good pointers, retry attempts, and build history left OFF — those require several queries per test and would multiply across every shard's failures. Once diagnose_pipeline points at the shard and test that matter, call diagnose_failure on that specific build for the full per-test diagnosis. max_tests_per_build caps detailed examination per shard (default 10, clamped to 50); a shard with more failures than that is reported with truncated=true. An unknown pipeline_id returns a clear error — check list_recent_builds for the project's actual ci_pipeline_id values.",
	}, diagnosePipelineHandler(stores, logger))
}

func diagnosePipelineHandler(stores *bootstrap.Stores, logger *zap.Logger) func(ctx context.Context, req *mcpsdk.CallToolRequest, in DiagnosePipelineInput) (*mcpsdk.CallToolResult, DiagnosePipelineOutput, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(ctx context.Context, _ *mcpsdk.CallToolRequest, in DiagnosePipelineInput) (*mcpsdk.CallToolResult, DiagnosePipelineOutput, error) {
		if in.ProjectID <= 0 {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf("project_id must be positive")
		}
		if in.PipelineID == "" {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf("pipeline_id must not be empty")
		}

		maxTests := in.MaxTestsPerBuild
		if maxTests <= 0 {
			maxTests = diagnosePipelineDefaultMaxTests
		}
		if maxTests > diagnosePipelineAbsoluteMaxTests {
			maxTests = diagnosePipelineAbsoluteMaxTests
		}

		proj, err := stores.Project.GetProject(ctx, int64(in.ProjectID))
		if err != nil {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf("project not found (id=%d): %w", in.ProjectID, err)
		}
		if proj == nil {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf("project %d not found", in.ProjectID)
		}

		// The store returns up to diagnosePipelineMaxBuilds+1 rows; a full
		// extra row means the pipeline holds more shards than examined here.
		builds, err := stores.Pipeline.ListBuildsByPipelineID(ctx, proj.ID, in.PipelineID, diagnosePipelineMaxBuilds)
		if err != nil {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf("listing builds for pipeline %q: %w", in.PipelineID, err)
		}
		if len(builds) == 0 {
			return nil, DiagnosePipelineOutput{}, fmt.Errorf(
				"no builds found in project %d with pipeline_id %q (hint: call list_recent_builds for this project — it now returns each build's ci_pipeline_id — to find a valid value)",
				proj.ID, in.PipelineID)
		}

		out := DiagnosePipelineOutput{
			PipelineID: in.PipelineID,
		}
		if len(builds) > diagnosePipelineMaxBuilds {
			builds = builds[:diagnosePipelineMaxBuilds]
			out.TruncatedBuilds = true
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"pipeline has more than %d shard builds; only the first %d (by build_order) were examined",
				diagnosePipelineMaxBuilds, diagnosePipelineMaxBuilds))
		}
		out.Builds = make([]PipelineBuildBlock, len(builds))

		// allTests collects every examined failing test across every shard build
		// so clusterFailingTests groups root causes across the WHOLE pipeline
		// rather than per shard. buildRanges maps each build's slice of allTests
		// back to its PipelineBuildBlock; clustering mutates allTests in place
		// (tagging ClusterID, stripping non-representative error_message) but
		// never changes its length, so indices recorded before clustering stay
		// valid after.
		var allTests []DiagnoseTest
		buildRanges := make([][2]int, len(builds)) // [start, end) into allTests, per build

		for i := range builds {
			b := &builds[i]
			if out.PipelineURL == "" && b.CIPipelineURL != nil && *b.CIPipelineURL != "" {
				out.PipelineURL = *b.CIPipelineURL
			}

			block := PipelineBuildBlock{
				BuildID:     b.ID,
				BuildNumber: b.BuildNumber,
				CreatedAt:   b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			}
			if b.CIBranch != nil {
				block.Branch = *b.CIBranch
			}
			if b.StatTotal != nil {
				block.Stats.Total = *b.StatTotal
			}
			if b.StatPassed != nil {
				block.Stats.Passed = *b.StatPassed
			}
			if b.StatFailed != nil {
				block.Stats.Failed = *b.StatFailed
			}
			if b.StatBroken != nil {
				block.Stats.Broken = *b.StatBroken
			}

			out.Totals.TotalTests += block.Stats.Total
			if block.Stats.Failed+block.Stats.Broken > 0 {
				out.Totals.FailedBuilds++
			}

			// Same over-fetch-then-dedupe pattern diagnose_failure uses:
			// legacy builds may hold two rows per test (twin history_id
			// schemes), so ask for 2*maxTests+2 to still have maxTests
			// distinct tests left after the collapse.
			failing, err := stores.TestResult.ListFailedByBuild(ctx, proj.ID, b.ID, 2*maxTests+2)
			if err != nil {
				return nil, DiagnosePipelineOutput{}, fmt.Errorf("listing failing tests for build %d: %w", b.ID, err)
			}
			var dedupeWarnings []string
			failing, _, dedupeWarnings = dedupeByFullName(failing)
			for _, w := range dedupeWarnings {
				out.Warnings = append(out.Warnings, fmt.Sprintf("build #%d: %s", b.BuildNumber, w))
			}

			// Totals count distinct failing tests, not raw rows: stat counters
			// on legacy builds are twin-inflated while failing_tests below is
			// deduped, and summing the stats would make the totals disagree
			// with the very list they summarize. CountFailedByBuild is one
			// indexed COUNT(DISTINCT full_name); a zero/error falls back to
			// the deduped examined count (an undercount only when truncated).
			if distinct, cErr := stores.TestResult.CountFailedByBuild(ctx, proj.ID, b.ID); cErr == nil && distinct > 0 {
				out.Totals.FailedTests += distinct
			} else {
				out.Totals.FailedTests += len(failing)
			}

			if len(failing) > maxTests {
				failing = failing[:maxTests]
				block.Truncated = true
			}

			// Per-test failed-step lookup: ONE indexed query per examined test
			// (bounded by max_tests_per_build × maxBuilds). It buys the deepest
			// failed step's own error text and, via the pure-Go triage.Analyze,
			// the failure phase and status pattern — the fields clusterKey and
			// the cluster's shared_status_pattern read. The heavier per-test
			// work (history, last-good, retry attempts, fingerprints) stays
			// off; use diagnose_failure on a specific build for that.
			start := len(allTests)
			for j := range failing {
				errMsg := failing[j].StatusMessage
				var stepPath []string
				if failing[j].HistoryID != "" {
					if sp, stepErr, err := stores.TestResult.GetFailedStepPath(ctx, proj.ID, b.ID, failing[j].HistoryID); err == nil {
						stepPath = sp
						if stepErr != "" {
							errMsg = stepErr
						}
					}
				}
				if len(errMsg) > diagnosePipelineMessageMax {
					errMsg = errMsg[:diagnosePipelineMessageMax]
				}
				signals := triage.Analyze(triage.Input{
					DurationMs:     failing[j].DurationMs,
					ErrorMessage:   errMsg,
					FailedStepPath: stepPath,
				})
				allTests = append(allTests, DiagnoseTest{
					FullName:       failing[j].FullName,
					Status:         failing[j].Status,
					ErrorMessage:   errMsg,
					FailedStepPath: stepPath,
					Signals:        signals,
				})
			}
			buildRanges[i] = [2]int{start, len(allTests)}
			out.Builds[i] = block
		}

		out.Totals.Builds = len(builds)

		// Cluster the UNION of every shard's failing tests together, then read
		// the (now cluster-tagged, trimmed) entries back per build.
		if len(allTests) > 0 {
			clusters, _ := clusterFailingTests(allTests)
			// Known-issue regex matching runs once per call against each
			// cluster's shared error, exactly as diagnose_failure does.
			matcher := newKnownIssueMatcher(ctx, stores, logger, proj.ID)
			for ci := range clusters {
				clusters[ci].KnownIssueRegexMatches = matcher.match(clusters[ci].SharedError)
			}
			out.Clusters = clusters
		}
		for i := range out.Builds {
			start, end := buildRanges[i][0], buildRanges[i][1]
			for j := start; j < end; j++ {
				out.Builds[i].FailingTests = append(out.Builds[i].FailingTests, PipelineFailingTest{
					FullName:     allTests[j].FullName,
					Status:       allTests[j].Status,
					ErrorMessage: allTests[j].ErrorMessage,
					ClusterID:    allTests[j].ClusterID,
				})
			}
		}

		digest := fmt.Sprintf("pipeline %s: %d build(s) (%d failed), %d failing test(s) in %d cluster(s)",
			in.PipelineID, out.Totals.Builds, out.Totals.FailedBuilds, out.Totals.FailedTests, len(out.Clusters))
		return textResult(digest), out, nil
	}
}
