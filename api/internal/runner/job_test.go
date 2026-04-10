package runner

import (
	"testing"
	"time"
)

func TestJobStatus_Constants(t *testing.T) {
	if JobStatusPending != "pending" {
		t.Errorf("expected 'pending', got %q", JobStatusPending)
	}
	if JobStatusRunning != "running" {
		t.Errorf("expected 'running', got %q", JobStatusRunning)
	}
	if JobStatusCompleted != "completed" {
		t.Errorf("expected 'completed', got %q", JobStatusCompleted)
	}
	if JobStatusFailed != "failed" {
		t.Errorf("expected 'failed', got %q", JobStatusFailed)
	}
}

func TestJob_Fields(t *testing.T) {
	now := time.Now()
	j := Job{
		ID:        "test-id",
		ProjectID: int64(1),
		Status:    JobStatusPending,
		CreatedAt: now,
		Params: JobParams{
			ExecName:     "CI",
			ExecFrom:     "http://example.com",
			ExecType:     "github",
			StoreResults: true,
			CIBranch:     "main",
			CICommitSHA:  "abc123",
		},
	}

	if j.ID != "test-id" {
		t.Errorf("ID: got %q, want %q", j.ID, "test-id")
	}
	if j.ProjectID != int64(1) {
		t.Errorf("ProjectID: got %d, want %d", j.ProjectID, int64(1))
	}
	if j.Status != JobStatusPending {
		t.Errorf("Status: got %q, want %q", j.Status, JobStatusPending)
	}
	if !j.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch")
	}
	if j.StartedAt != nil {
		t.Errorf("StartedAt should be nil initially")
	}
	if j.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil initially")
	}
	if j.Params.ExecName != "CI" {
		t.Errorf("Params.ExecName: got %q, want %q", j.Params.ExecName, "CI")
	}
	if !j.Params.StoreResults {
		t.Error("Params.StoreResults should be true")
	}
}

func TestJob_StartedAtAndCompletedAt(t *testing.T) {
	start := time.Now()
	end := start.Add(5 * time.Second)

	j := Job{
		Status:      JobStatusCompleted,
		StartedAt:   &start,
		CompletedAt: &end,
		Output:      "Report successfully generated",
	}

	if j.StartedAt == nil || !j.StartedAt.Equal(start) {
		t.Errorf("StartedAt mismatch")
	}
	if j.CompletedAt == nil || !j.CompletedAt.Equal(end) {
		t.Errorf("CompletedAt mismatch")
	}
	if j.Output != "Report successfully generated" {
		t.Errorf("Output: got %q", j.Output)
	}
}

func TestJob_FailedState(t *testing.T) {
	end := time.Now()
	j := Job{
		Status:      JobStatusFailed,
		CompletedAt: &end,
		Error:       "something went wrong",
	}

	if j.Status != JobStatusFailed {
		t.Errorf("Status: got %q, want %q", j.Status, JobStatusFailed)
	}
	if j.Error != "something went wrong" {
		t.Errorf("Error: got %q", j.Error)
	}
}
