package handlers

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/runner"
)

// progressMockGenerator publishes one phase/progress update when phase is set,
// then holds its job in flight until the test ends. The MemJobManager wires
// the reporter through SetProgressReporter.
type progressMockGenerator struct {
	reporter    runner.JobProgressReporter
	phase       runner.JobPhase
	done, total int
	hold        chan struct{}
}

func (p *progressMockGenerator) SetProgressReporter(r runner.JobProgressReporter) { p.reporter = r }

func (p *progressMockGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	if p.phase != "" && p.reporter != nil {
		p.reporter(p.phase, p.done, p.total)
	}
	<-p.hold
	return "1", nil
}

// newReportJobHandler serves a ReportHandler over a MemJobManager running gen,
// with projects 1 and 2 and project 3, "child-proj", nested under 1.
func newReportJobHandler(t *testing.T, gen *progressMockGenerator) *ReportHandler {
	t.Helper()
	gen.hold = make(chan struct{})
	h, mocks := newTestReportHandlerWithJobManager(t, t.TempDir(), gen)
	t.Cleanup(func() { close(gen.hold) }) // runs before the job manager's shutdown
	ctx := context.Background()
	a, _ := mocks.Projects.CreateProject(ctx, "project-a")
	_, _ = mocks.Projects.CreateProject(ctx, "project-b")
	_, _ = mocks.Projects.CreateProjectWithParent(ctx, "child-proj", a.ID)
	return h
}

func makeGenerateReportReq(t *testing.T, projectID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/reports", nil)
	req.SetPathValue("project_id", projectID)
	return req
}

func makeGetJobStatusReq(t *testing.T, projectID, jobID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/jobs/"+jobID, nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("job_id", jobID)
	return req
}

// queueReport asks for a report of projectID and returns the queued job's ID.
func queueReport(t *testing.T, h *ReportHandler, projectID string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.GenerateReport(rr, makeGenerateReportReq(t, projectID))
	var resp struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
		Metadata ResponseMeta `json:"metadata"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || rr.Code != http.StatusAccepted {
		t.Fatalf("generate: status = %d, want 202: %s", rr.Code, rr.Body.String())
	}
	if resp.Data.JobID == "" || resp.Metadata.Message != "Report generation queued" {
		t.Fatalf("generate: got %+v, want a job_id and message %q", resp, "Report generation queued")
	}
	return resp.Data.JobID
}

func TestReportHandler_GenerateReport(t *testing.T) {
	t.Parallel()
	h := newReportJobHandler(t, &progressMockGenerator{})
	queueReport(t, h, "1")
	for _, project := range []string{"login", "../evil"} { // reserved name, path traversal
		rr := httptest.NewRecorder()
		h.GenerateReport(rr, makeGenerateReportReq(t, project))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("project %q: status = %d, want 400", project, rr.Code)
		}
	}
}

func TestReportHandler_GetJobStatus(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name     string
		gen      progressMockGenerator
		queueFor string // project the job is queued for; "" polls an unknown job
		pollAs   string // project_id path value of the status request
		want     int
		progress map[string]int // published progress to wait for; nil asserts none
	}{
		// The JSON keys are snake_case to match the UI's JobData type.
		{"own project", progressMockGenerator{}, "1", "1", http.StatusOK, nil},
		{"unknown job", progressMockGenerator{}, "", "1", http.StatusNotFound, nil},
		{"job of another project", progressMockGenerator{}, "1", "2", http.StatusNotFound, nil},
		// f9fd7f6: CI scripts poll with the nested child's slug.
		{"nested child by slug", progressMockGenerator{}, "3", "child-proj", http.StatusOK, nil},
		{"phase and progress", progressMockGenerator{phase: runner.JobPhasePreparingLocal, done: 1234, total: 5000},
			"1", "1", http.StatusOK, map[string]int{"done": 1234, "total": 5000}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gen := tc.gen
			h := newReportJobHandler(t, &gen)
			jobID := "nonexistent-job-id"
			if tc.queueFor != "" {
				jobID = queueReport(t, h, tc.queueFor)
			}
			var data map[string]any
			for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(2 * time.Millisecond) {
				rr := httptest.NewRecorder()
				h.GetJobStatus(rr, makeGetJobStatusReq(t, tc.pollAs, jobID))
				if rr.Code != tc.want {
					t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
				}
				var resp struct {
					Data map[string]any `json:"data"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				data = resp.Data
				if tc.progress == nil || data["phase"] == string(tc.gen.phase) || time.Now().After(deadline) {
					break
				}
			}
			if tc.want != http.StatusOK {
				return
			}
			for _, key := range []string{"job_id", "status", "project_id", "created_at"} {
				if v, ok := data[key]; !ok || v == "" {
					t.Errorf("data[%q] = %v, want it set", key, v)
				}
			}
			// progress is omitted until the runner publishes one, so clients
			// that ignore unknown fields stay compatible.
			raw, published := data["progress"]
			if published != (tc.progress != nil) {
				t.Fatalf("progress = %v, want %v", raw, tc.progress)
			}
			if got, _ := raw.(map[string]any); tc.progress != nil && !maps.EqualFunc(got, tc.progress, func(v any, w int) bool { return v == float64(w) }) {
				t.Errorf("progress = %v, want %v", got, tc.progress)
			}
		})
	}
}
