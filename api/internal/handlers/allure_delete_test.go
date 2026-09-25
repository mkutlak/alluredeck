package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDeleteProject(t *testing.T) {
	tests := []struct {
		name   string
		onDisk bool   // the project directory exists, not just the DB row
		id     string // "" targets the seeded project
		want   int
	}{
		{name: "removes directory and row", onDisk: true, want: http.StatusOK},
		// A project in the DB but not on disk (half-synced) is still cleaned up
		// instead of 404ing and leaving the stale row behind.
		{name: "stale db row", want: http.StatusOK},
		{name: "unknown project", id: "999", want: http.StatusNotFound},
		{name: "invalid id", id: "../evil", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectsDir := t.TempDir()
			h, mocks := newTestProjectHandler(t, projectsDir)
			proj, err := mocks.Projects.CreateProject(context.Background(), "myproject")
			if err != nil {
				t.Fatal(err)
			}
			if tc.onDisk {
				for _, sub := range []string{"results", "reports"} {
					if err := os.MkdirAll(filepath.Join(projectsDir, "myproject", sub), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			id := tc.id
			if id == "" {
				id = strconv.FormatInt(proj.ID, 10)
			}
			code, body := serveJSON(t, h.DeleteProject, http.MethodDelete, "/api/v1/projects/"+id, "", "project_id", id)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			if code != http.StatusOK {
				return
			}
			wantJSON(t, body, map[string]any{"metadata.message": "Project successfully deleted"})
			if _, err := os.Stat(filepath.Join(projectsDir, "myproject")); !os.IsNotExist(err) {
				t.Errorf("project directory still exists (stat err %v)", err)
			}
			if exists, _ := mocks.Projects.ProjectExists(context.Background(), proj.ID); exists {
				t.Error("project row still exists")
			}
		})
	}
}

func TestDeleteReport(t *testing.T) {
	tests := []struct {
		name     string
		reportID string
		want     int
	}{
		{name: "removes report directory", reportID: "3", want: http.StatusOK},
		{name: "unknown report", reportID: "999", want: http.StatusNotFound},
		{name: "missing report id", reportID: "", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectsDir := t.TempDir()
			h, mocks := newTestReportHandler(t, projectsDir)
			proj, err := mocks.Projects.CreateProject(context.Background(), "myproject")
			if err != nil {
				t.Fatal(err)
			}
			reportDir := filepath.Join(projectsDir, "myproject", "reports", "3")
			if err := os.MkdirAll(reportDir, 0o755); err != nil {
				t.Fatal(err)
			}
			id := strconv.FormatInt(proj.ID, 10)
			code, body := serveJSON(t, h.DeleteReport, http.MethodDelete, "/api/v1/projects/"+id+"/reports/"+tc.reportID, "",
				"project_id", id, "report_id", tc.reportID)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			if code != http.StatusOK {
				return
			}
			wantJSON(t, body, map[string]any{"metadata.message": `Report "3" successfully deleted`})
			if _, err := os.Stat(reportDir); !os.IsNotExist(err) {
				t.Errorf("report directory still exists (stat err %v)", err)
			}
		})
	}
}
