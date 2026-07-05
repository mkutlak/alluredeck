package failure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/llm"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PromptVersion is bumped whenever the prompt template or evidence assembly
// changes in a way that must invalidate previously cached summaries (it feeds
// the input hash).
const PromptVersion = 1

// Bounds on the untrusted evidence folded into the prompt. Allure step text and
// attachment bytes are attacker-influenced, so every source is capped.
const (
	maxStepPath      = 20       // step-path entries
	historyDepth     = 20       // builds fetched to compute builds-since
	attachListLimit  = 20       // attachments inspected per test
	attachTextBudget = 4 * 1024 // total attachment bytes folded into the prompt
	singleAttachName = 128      // max attachment-name chars echoed into the prompt
)

// Summarizer is the minimal LLM surface the service needs. *llm.Client
// satisfies it; tests inject a fake.
type Summarizer interface {
	Summarize(ctx context.Context, p llm.Prompt) (llm.Summary, error)
}

// errNoFailureEvidence is returned (as a soft Result.Err) when a request
// carries no objective failure evidence (no error message, no failed-step
// path). Without this gate, any authenticated viewer could request a summary
// for an arbitrary (build_id, history_id) pair — including a test that never
// failed — and trigger an unbounded number of real (paid) LLM calls plus junk
// failure_summaries rows. The gate is checked before the LLM is ever touched
// and before any cache write.
var errNoFailureEvidence = errors.New("no failure evidence for this test")

// errBlankHypothesis is returned (as a soft Result.Err) when the model
// returns HTTP 200 with an empty/whitespace-only hypothesis. It must not be
// cached: caching a blank hypothesis would serve (and re-serve forever) a
// useless summary instead of letting the next request retry generation.
var errBlankHypothesis = errors.New("llm returned an empty hypothesis")

// Result is the outcome of SummaryFor. It is an internal Go type; the REST
// handler maps it onto the frozen JSON response shape.
type Result struct {
	Enabled  bool
	Cached   bool
	Summary  *store.FailureSummary
	LastGood *LastGood
	Model    string
	// Err carries a soft, user-facing generation error (e.g. the LLM call
	// failed). It is surfaced as data.error with enabled:true and is NEVER a
	// 5xx. Nil on success and on the disabled path.
	Err error
}

// ServiceDeps bundles the collaborators a Service needs.
type ServiceDeps struct {
	TestResults store.TestResultStorer
	Attachments store.AttachmentStorer
	Builds      store.BuildStorer
	Summaries   store.FailureSummaryStorer
	Blobs       storage.Store
	LLM         Summarizer
	Config      config.LLMConfig
	Logger      *zap.Logger
}

// Service generates and caches the opt-in LLM failure summary for a single
// failing test.
type Service struct {
	testResults store.TestResultStorer
	attachments store.AttachmentStorer
	builds      store.BuildStorer
	summaries   store.FailureSummaryStorer
	blobs       storage.Store
	llm         Summarizer
	cfg         config.LLMConfig
	logger      *zap.Logger
	group       singleflight.Group
}

// NewService constructs a Service from its dependencies.
func NewService(d ServiceDeps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{
		testResults: d.TestResults,
		attachments: d.Attachments,
		builds:      d.Builds,
		summaries:   d.Summaries,
		blobs:       d.Blobs,
		llm:         d.LLM,
		cfg:         d.Config,
		logger:      logger,
	}
}

// SummaryFor returns the failure summary for (projectID, buildID, historyID).
// When the feature is disabled it short-circuits to Result{Enabled:false}
// without touching the LLM client. Otherwise it assembles evidence, serves a
// matching cached summary, or generates (and caches) a new one. A generation
// failure is returned as a soft error in Result.Err, never as a hard error.
func (s *Service) SummaryFor(ctx context.Context, projectID, buildID int64, historyID string) (Result, error) {
	if !s.cfg.Enabled {
		return Result{Enabled: false}, nil
	}

	build, err := s.builds.GetBuildByID(ctx, projectID, buildID)
	if err != nil {
		return Result{}, fmt.Errorf("failure summary: load build %d: %w", buildID, err)
	}

	ev := s.assembleEvidence(ctx, projectID, &build, historyID)

	// Existence/evidence gate: refuse to generate (or even read the cache) for
	// a test with no objective failure evidence. See errNoFailureEvidence.
	if ev.errorMessage == "" && len(ev.stepPath) == 0 {
		return Result{Enabled: true, LastGood: ev.lastGood, Model: s.cfg.Model, Err: errNoFailureEvidence}, nil
	}

	hash := inputHash(ev, s.cfg.Model)

	// Cache check: a matching input hash serves without touching the LLM.
	if cached, err := s.summaries.Get(ctx, buildID, historyID); err != nil {
		return Result{}, fmt.Errorf("failure summary: read cache: %w", err)
	} else if cached != nil && cached.InputHash == hash {
		return Result{Enabled: true, Cached: true, Summary: cached, LastGood: ev.lastGood, Model: cached.Model}, nil
	}

	// Deduplicate concurrent identical generations; the cache covers repeats.
	key := strconv.FormatInt(buildID, 10) + "|" + historyID
	v, _, _ := s.group.Do(key, func() (any, error) {
		return s.generate(ctx, projectID, buildID, historyID, ev, hash), nil
	})
	return v.(Result), nil
}

// generate produces and caches a summary. It re-checks the cache first (a
// concurrent request may have just written a matching row). An LLM failure —
// or a blank/whitespace-only hypothesis — is returned as a soft error and is
// never cached, so the next request retries generation instead of serving a
// useless result forever.
func (s *Service) generate(ctx context.Context, projectID, buildID int64, historyID string, ev evidence, hash string) Result {
	if cached, err := s.summaries.Get(ctx, buildID, historyID); err == nil && cached != nil && cached.InputHash == hash {
		return Result{Enabled: true, Cached: true, Summary: cached, LastGood: ev.lastGood, Model: cached.Model}
	}

	// The whole-build last-good→current diff is comparatively expensive
	// (CompareBuildsByHistoryID) and only enriches the prompt — it is not part
	// of input_hash — so it is computed here, on the cache-miss path only,
	// rather than unconditionally in assembleEvidence on every request.
	if ev.lastGood != nil {
		if diffs, err := s.testResults.CompareBuildsByHistoryID(ctx, projectID, ev.lastGoodBuildID, buildID); err == nil {
			lgd := BuildLastGoodDiff(diffs, historyID, ev.lastGoodBuildID, buildID)
			ev.diff = &lgd
		}
	}

	sum, err := s.llm.Summarize(ctx, buildPrompt(ev))
	if err != nil {
		s.logger.Warn("failure summary generation failed",
			zap.Int64("project_id", projectID), zap.Int64("build_id", buildID),
			zap.String("history_id", historyID), zap.Error(err))
		return Result{Enabled: true, LastGood: ev.lastGood, Model: s.cfg.Model, Err: err}
	}
	if strings.TrimSpace(sum.Hypothesis) == "" {
		s.logger.Warn("failure summary: llm returned a blank hypothesis",
			zap.Int64("project_id", projectID), zap.Int64("build_id", buildID),
			zap.String("history_id", historyID))
		return Result{Enabled: true, LastGood: ev.lastGood, Model: s.cfg.Model, Err: errBlankHypothesis}
	}

	rec := store.FailureSummary{
		BuildID:       buildID,
		HistoryID:     historyID,
		ProjectID:     projectID,
		InputHash:     hash,
		Hypothesis:    sum.Hypothesis,
		Category:      sum.Category,
		Confidence:    sum.Confidence,
		Evidence:      normalizeEvidenceList(sum.Evidence),
		Model:         s.cfg.Model,
		PromptVersion: PromptVersion,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.summaries.Upsert(ctx, rec); err != nil {
		// Caching is best-effort: return the fresh summary even if the write failed.
		s.logger.Warn("failure summary cache write failed",
			zap.Int64("build_id", buildID), zap.String("history_id", historyID), zap.Error(err))
	}
	return Result{Enabled: true, Cached: false, Summary: &rec, LastGood: ev.lastGood, Model: s.cfg.Model}
}

// normalizeEvidenceList ensures the fresh-generation response carries a
// non-nil (possibly empty) Evidence slice, matching the shape a cache-hit read
// always produces (json.Unmarshal of the stored `[]` JSONB never yields nil).
// Without this, a fresh response with no evidence bullets marshals `evidence`
// as JSON null while a cached read of the same row marshals `[]`.
func normalizeEvidenceList(ev []string) []string {
	if ev == nil {
		return []string{}
	}
	return ev
}

// evidence is the assembled, bounded input to the prompt and the input hash.
type evidence struct {
	errorMessage    string
	stepPath        []string
	lastGood        *LastGood
	lastGoodBuildID int64
	diff            *LastGoodDiff
	attachText      string
}

// assembleEvidence gathers the bounded, untrusted evidence for one failing
// test. Every source is best-effort: a store error degrades that piece to empty
// rather than failing the whole summary.
func (s *Service) assembleEvidence(ctx context.Context, projectID int64, build *store.Build, historyID string) evidence {
	var ev evidence

	if stepPath, errMsg, err := s.testResults.GetFailedStepPath(ctx, projectID, build.ID, historyID); err == nil {
		ev.stepPath = capStrings(stepPath, maxStepPath)
		ev.errorMessage = errMsg
	}

	if lg, err := s.testResults.GetLastPassingBuild(ctx, projectID, historyID, build.BranchID, build.BuildNumber); err != nil {
		s.logger.Warn("failure summary: last-good fetch failed",
			zap.Int64("project_id", projectID), zap.String("history_id", historyID), zap.Error(err))
	} else if lg != nil {
		ev.lastGoodBuildID = lg.BuildID
		pointer := &LastGood{
			BuildID:     lg.BuildID,
			BuildNumber: lg.BuildNumber,
			CreatedAt:   lg.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			BuildsSince: s.buildsSince(ctx, projectID, build, historyID, lg.BuildNumber),
		}
		if lg.CICommitSHA != nil {
			pointer.CommitSHA = *lg.CICommitSHA
		}
		ev.lastGood = pointer
		// NOTE: the whole-build last-good→current diff is deliberately NOT
		// computed here. It only enriches the prompt (it is not part of
		// input_hash), so computing it unconditionally would run the
		// expensive CompareBuildsByHistoryID query on every request,
		// including cache hits. See generate()'s cache-miss-only diff step.
	}

	ev.attachText = s.readAttachmentText(ctx, projectID, build, historyID)
	return ev
}

// buildsSince counts prior builds of this test strictly between the last-good
// build and the current one, mirroring diagnose_failure's semantics. It is
// bounded by historyDepth.
func (s *Service) buildsSince(ctx context.Context, projectID int64, build *store.Build, historyID string, lastGoodOrder int) int {
	rows, err := s.testResults.GetTestHistory(ctx, projectID, historyID, build.BranchID, historyDepth)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if r.BuildID == build.ID {
			continue
		}
		if r.BuildNumber > lastGoodOrder && r.BuildNumber < build.BuildNumber {
			n++
		}
	}
	return n
}

// readAttachmentText reads a bounded amount of text-like attachment content for
// the failing test. Only text/* and application/json attachments are read, each
// source is validated against path traversal, and the total is capped at
// attachTextBudget. Everything here is untrusted and delimited as data in the
// prompt.
func (s *Service) readAttachmentText(ctx context.Context, projectID int64, build *store.Build, historyID string) string {
	if s.blobs == nil {
		return ""
	}
	rows, err := s.attachments.ListByTestResult(ctx, projectID, build.ID, historyID, attachListLimit)
	if err != nil || len(rows) == 0 {
		return ""
	}

	var sb strings.Builder
	remaining := attachTextBudget
	for _, a := range rows {
		if remaining <= 0 {
			break
		}
		if !isTextMIME(a.MimeType) {
			continue
		}
		loc, err := s.attachments.GetLocation(ctx, a.ID)
		if err != nil || loc == nil {
			continue
		}
		if !validAttachmentSource(loc.Source) {
			continue
		}
		rc, _, err := s.blobs.OpenReportFile(ctx, loc.StorageKey, strconv.Itoa(loc.BuildNumber), "data/attachments/"+loc.Source)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(rc, int64(remaining)))
		_ = rc.Close()
		if len(data) == 0 {
			continue
		}
		sb.WriteString("\n--- attachment: ")
		sb.WriteString(truncate(a.Name, singleAttachName))
		sb.WriteString(" ---\n")
		sb.Write(data)
		remaining -= len(data)
	}
	return sb.String()
}

// inputHash derives the cache key material from the assembled evidence, the
// model, and the prompt version. A change in any of these regenerates the
// summary.
func inputHash(ev evidence, model string) string {
	h := sha256.New()
	writeField := func(s string) {
		_, _ = io.WriteString(h, s)
		_, _ = h.Write([]byte{0})
	}
	writeField(ev.errorMessage)
	for _, p := range ev.stepPath {
		writeField(p)
	}
	writeField(ev.attachText)
	writeField(strconv.FormatInt(ev.lastGoodBuildID, 10))
	writeField(model)
	writeField(strconv.Itoa(PromptVersion))
	return hex.EncodeToString(h.Sum(nil))
}

// capStrings returns at most n elements of in.
func capStrings(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

// truncate shortens s to at most n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// isTextMIME reports whether a MIME type is text-like enough to fold into the
// prompt (text/* or application/json only).
func isTextMIME(mime string) bool {
	l := strings.ToLower(strings.TrimSpace(mime))
	return strings.HasPrefix(l, "text/") || l == "application/json"
}

// validAttachmentSource rejects attachment source filenames that could be used
// for path traversal. It mirrors mcp.ValidateAttachmentSource; the guard is
// re-implemented here (rather than imported) because internal/mcp imports
// internal/mcp/tools which imports this package — importing mcp would create a
// cycle. The rule is a bare filename: no separators, no parent refs, no NUL.
func validAttachmentSource(source string) bool {
	return source != "" &&
		!strings.Contains(source, "/") &&
		!strings.Contains(source, "\\") &&
		!strings.Contains(source, "..") &&
		!strings.ContainsRune(source, 0)
}
