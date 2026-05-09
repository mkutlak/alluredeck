package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/runner"
)

// progressMockGenerator publishes a fixed sequence of phase/progress updates
// before returning. It implements runner.ReportGenerator and opts in to the
// MemJobManager progressReceiver path via SetProgressReporter so we can drive
// the handler without spinning up the full allure CLI.
type progressMockGenerator struct {
	reporter runner.JobProgressReporter
	steps    []struct {
		phase runner.JobPhase
		done  int
		total int
	}
	hold chan struct{} // closed by the test once it has observed the in-flight state
	out  string
}

func (p *progressMockGenerator) SetProgressReporter(r runner.JobProgressReporter) { p.reporter = r }

func (p *progressMockGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	for _, s := range p.steps {
		if p.reporter != nil {
			p.reporter(s.phase, s.done, s.total)
		}
	}
	if p.hold != nil {
		<-p.hold
	}
	return p.out, nil
}

// TestGetJobStatus_ExposesPhaseAndProgress verifies the JSON response from
// /jobs/{id} carries the new phase and progress fields once the runner has
// published progress updates for the job.
func TestGetJobStatus_ExposesPhaseAndProgress(t *testing.T) {
	hold := make(chan struct{})
	gen := &progressMockGenerator{
		steps: []struct {
			phase runner.JobPhase
			done  int
			total int
		}{
			{runner.JobPhasePreparingLocal, 1234, 5000},
		},
		hold: hold,
		out:  "1",
	}
	defer close(hold)

	projectsDir := t.TempDir()
	h, mocks := newTestReportHandlerWithJobManager(t, projectsDir, gen)

	proj, err := mocks.Projects.CreateProject(context.Background(), "myproject")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	// Submit the job.
	genRR := httptest.NewRecorder()
	h.GenerateReport(genRR, makeGenerateReportReq(t, projectIDStr))
	if genRR.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", genRR.Code, genRR.Body.String())
	}
	var genResp map[string]any
	if err := json.Unmarshal(genRR.Body.Bytes(), &genResp); err != nil {
		t.Fatal(err)
	}
	jobID := genResp["data"].(map[string]any)["job_id"].(string)

	// Poll the status endpoint until phase=preparing_local is observed.
	deadline := time.Now().Add(2 * time.Second)
	var data map[string]any
	for {
		rr := httptest.NewRecorder()
		h.GetJobStatus(rr, makeGetJobStatusReq(t, projectIDStr, jobID))
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		data = resp["data"].(map[string]any)
		if phase, _ := data["phase"].(string); phase == string(runner.JobPhasePreparingLocal) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never observed phase=preparing_local in /jobs/{id}, last data: %v", data)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// progress must be a typed object with done/total integers.
	prog, ok := data["progress"].(map[string]any)
	if !ok {
		t.Fatalf("expected progress object in data, got %v", data["progress"])
	}
	if got := prog["done"]; got != float64(1234) {
		t.Errorf("progress.done: got %v (%T), want 1234", got, got)
	}
	if got := prog["total"]; got != float64(5000) {
		t.Errorf("progress.total: got %v (%T), want 5000", got, got)
	}
}

// TestGetJobStatus_OmitsProgressFieldsWhenAbsent verifies that brand-new jobs
// (no progress reported yet) still encode without the progress key, preserving
// backward compatibility for clients that ignore unknown fields.
func TestGetJobStatus_OmitsProgressFieldsWhenAbsent(t *testing.T) {
	gen := &progressMockGenerator{hold: make(chan struct{}), out: "1"}
	defer close(gen.hold)

	projectsDir := t.TempDir()
	h, mocks := newTestReportHandlerWithJobManager(t, projectsDir, gen)

	proj, err := mocks.Projects.CreateProject(context.Background(), "myproject")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	genRR := httptest.NewRecorder()
	h.GenerateReport(genRR, makeGenerateReportReq(t, projectIDStr))
	var genResp map[string]any
	_ = json.Unmarshal(genRR.Body.Bytes(), &genResp)
	jobID := genResp["data"].(map[string]any)["job_id"].(string)

	rr := httptest.NewRecorder()
	h.GetJobStatus(rr, makeGetJobStatusReq(t, projectIDStr, jobID))

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	// progress must be absent (omitempty) when the job has not yet published any.
	if _, present := data["progress"]; present {
		t.Errorf("progress should be omitted before any update, got %v", data["progress"])
	}
}
