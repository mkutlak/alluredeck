package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// writeReportTree writes files (path relative to root → content), creating
// parent directories.
func writeReportTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportHandler_GetReportHistory(t *testing.T) {
	t.Parallel()
	const summary = `{"statistic":{"passed":5,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":6},"time":{"stop":1700000000000,"duration":3000}}`
	ci := store.CIMetadata{Provider: "GitHub Actions", BuildURL: "https://github.com/org/repo/actions/runs/123", Branch: "main", CommitSHA: "abc1234"}
	rows := []struct {
		name      string
		summaries []string          // report dirs given a widgets/summary.json
		files     map[string]string // other files, relative to the project's reports dir
		ci        bool              // stamp CI metadata on build 1
		project   string            // project_id path value; "" uses the seeded project
		want      int
		wantIDs   string // report_id of each entry, space separated
		check     func(t *testing.T, latest map[string]any)
	}{
		{name: "no reports", want: http.StatusOK},
		// "latest" comes from storage and is prepended; numbered builds come
		// from the build store, newest first.
		{name: "latest then numbered builds", summaries: []string{"1", "3", "latest"}, want: http.StatusOK, wantIDs: "latest 3 1",
			check: func(t *testing.T, e map[string]any) {
				if e["is_latest"] != true {
					t.Errorf("is_latest = %v, want true", e["is_latest"])
				}
			}},
		{name: "build without summary", files: map[string]string{"1/.keep": ""}, want: http.StatusOK, wantIDs: "1",
			check: func(t *testing.T, e map[string]any) {
				if e["statistic"] != nil {
					t.Errorf("statistic = %v, want nil", e["statistic"])
				}
			}},
		{name: "CI metadata", summaries: []string{"1"}, ci: true, want: http.StatusOK, wantIDs: "1",
			check: func(t *testing.T, e map[string]any) {
				for k, v := range map[string]string{"ci_provider": ci.Provider, "ci_build_url": ci.BuildURL, "ci_branch": ci.Branch, "ci_commit_sha": ci.CommitSHA} {
					if e[k] != v {
						t.Errorf("%s = %v, want %q", k, e[k], v)
					}
				}
			}},
		// Allure 3 "latest" has statistic.json but no timing: the duration is
		// the sum of the test-result durations (2000 + 4000).
		{name: "Allure 3 latest timing", want: http.StatusOK, wantIDs: "latest", files: map[string]string{
			"latest/widgets/statistic.json":   `{"passed":3,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":4}`,
			"latest/data/test-results/a.json": `{"start":1700000000000,"stop":1700000002000}`,
			"latest/data/test-results/b.json": `{"start":1700000001000,"stop":1700000005000}`,
		}, check: func(t *testing.T, e map[string]any) {
			if e["is_latest"] != true || e["duration_ms"] != float64(6000) || e["generated_at"] == nil || e["generated_at"] == "" {
				t.Errorf("latest = %v, want is_latest, duration_ms 6000 and generated_at", e)
			}
		}},
		{name: "invalid project id", project: "../evil", want: http.StatusBadRequest},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			reportsDir := filepath.Join(dir, "proj", "reports")
			for _, name := range tc.summaries {
				writeSummaryJSON(t, filepath.Join(reportsDir, name), summary)
			}
			writeReportTree(t, reportsDir, tc.files)
			h, mocks := newTestReportHandler(t, dir)
			p, _ := mocks.Projects.CreateProject(context.Background(), "proj")
			if tc.ci {
				if err := h.buildStore.UpdateBuildCIMetadata(context.Background(), p.ID, 1, ci); err != nil {
					t.Fatal(err)
				}
			}
			project := tc.project
			if project == "" {
				project = strconv.FormatInt(p.ID, 10)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project+"/reports", nil)
			req.SetPathValue("project_id", project)
			rr := httptest.NewRecorder()
			h.GetReportHistory(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			var resp struct {
				Data struct {
					Reports []map[string]any `json:"reports"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, r := range resp.Data.Reports {
				id, _ := r["report_id"].(string)
				ids = append(ids, id)
			}
			if resp.Data.Reports == nil || strings.Join(ids, " ") != tc.wantIDs {
				t.Fatalf("report ids = %q, want %q (null reports: %v)", ids, tc.wantIDs, resp.Data.Reports == nil)
			}
			if tc.check != nil {
				tc.check(t, resp.Data.Reports[0])
			}
		})
	}
}
