package runner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
)

// TestReportIDFromMetadata verifies the helper that extracts report_id from
// River job metadata stored under the "output" key.
func TestReportIDFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta []byte
		want string
	}{
		{
			name: "output key present",
			meta: []byte(`{"output":"42"}`),
			want: "42",
		},
		{
			name: "output key with other keys present",
			meta: []byte(`{"trace":"x","output":"99"}`),
			want: "99",
		},
		{
			name: "only other keys, no output",
			meta: []byte(`{"trace":"x"}`),
			want: "",
		},
		{
			name: "nil metadata",
			meta: nil,
			want: "",
		},
		{
			name: "empty metadata",
			meta: []byte(""),
			want: "",
		},
		{
			name: "malformed JSON",
			meta: []byte("{"),
			want: "",
		},
		{
			name: "output is non-string number",
			meta: []byte(`{"output":123}`),
			want: "",
		},
		{
			name: "output is null",
			meta: []byte(`{"output":null}`),
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reportIDFromMetadata(tc.meta)
			if got != tc.want {
				t.Errorf("reportIDFromMetadata(%q) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}

// TestRiverRowToJob_ReportIDFromMetadata verifies that riverRowToJob populates
// ReportID from the job metadata "output" key when present.
func TestRiverRowToJob_ReportIDFromMetadata(t *testing.T) {
	now := time.Now()

	argsWithReportID := func(projectID int64) []byte {
		b, _ := json.Marshal(GenerateReportArgs{ProjectID: projectID, Slug: "test-slug"})
		return b
	}

	tests := []struct {
		name          string
		metadata      []byte
		wantReportID  string
		wantStatus    JobStatus
		wantProjectID int64
	}{
		{
			name:          "metadata has output key",
			metadata:      []byte(`{"output":"42"}`),
			wantReportID:  "42",
			wantStatus:    JobStatusCompleted,
			wantProjectID: 7,
		},
		{
			name:          "metadata has no output key",
			metadata:      []byte(`{"trace":"abc"}`),
			wantReportID:  "",
			wantStatus:    JobStatusCompleted,
			wantProjectID: 7,
		},
		{
			name:          "nil metadata",
			metadata:      nil,
			wantReportID:  "",
			wantStatus:    JobStatusCompleted,
			wantProjectID: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := &rivertype.JobRow{
				ID:          1001,
				State:       rivertype.JobStateCompleted,
				EncodedArgs: argsWithReportID(tc.wantProjectID),
				Metadata:    tc.metadata,
				CreatedAt:   now,
				FinalizedAt: &now,
			}

			j := riverRowToJob(row)

			if j.ReportID != tc.wantReportID {
				t.Errorf("ReportID = %q, want %q", j.ReportID, tc.wantReportID)
			}
			if j.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", j.Status, tc.wantStatus)
			}
			if j.ProjectID != tc.wantProjectID {
				t.Errorf("ProjectID = %d, want %d", j.ProjectID, tc.wantProjectID)
			}
		})
	}
}
