package runner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
)

// TestReportIDFromMetadata pins where job status reads report_id: the durable
// River "output" metadata written at completion (not a process-local map), so
// any replica — or a restarted pod — can answer a job poll.
func TestReportIDFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta []byte
		want string
	}{
		{"output key present", []byte(`{"output":"42"}`), "42"},
		{"output key with other keys present", []byte(`{"trace":"x","output":"99"}`), "99"},
		{"only other keys, no output", []byte(`{"trace":"x"}`), ""},
		{"nil metadata", nil, ""},
		{"empty metadata", []byte(""), ""},
		{"malformed JSON", []byte("{"), ""},
		{"output is non-string number", []byte(`{"output":123}`), ""},
		{"output is null", []byte(`{"output":null}`), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportIDFromMetadata(tc.meta); got != tc.want {
				t.Errorf("reportIDFromMetadata(%q) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}

	// riverRowToJob must surface it alongside the row's status and args.
	now := time.Now()
	args, _ := json.Marshal(GenerateReportArgs{ProjectID: 7, Slug: "test-slug"})
	j := riverRowToJob(&rivertype.JobRow{
		ID: 1001, State: rivertype.JobStateCompleted, EncodedArgs: args,
		Metadata: []byte(`{"output":"42"}`), CreatedAt: now, FinalizedAt: &now,
	})
	if j.ReportID != "42" || j.Status != JobStatusCompleted || j.ProjectID != 7 {
		t.Errorf("riverRowToJob = {ReportID: %q, Status: %q, ProjectID: %d}, want {42, completed, 7}", j.ReportID, j.Status, j.ProjectID)
	}
}
