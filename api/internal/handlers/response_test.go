package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	writeSuccess(rr, 200, map[string]string{"id": "proj1"}, "Project created")

	if rr.Code != 200 {
		t.Fatalf("want 200, got %d", rr.Code)
	}

	var resp struct {
		Data     map[string]string `json:"data"`
		Metadata struct {
			Message string `json:"message"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data["id"] != "proj1" {
		t.Errorf("data.id = %q, want %q", resp.Data["id"], "proj1")
	}
	if resp.Metadata.Message != "Project created" {
		t.Errorf("metadata.message = %q, want %q", resp.Metadata.Message, "Project created")
	}
}

func TestWritePagedSuccess(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	pg := newPaginationMeta(1, 20, 42)
	writePagedSuccess(rr, []string{"a", "b"}, "Items listed", pg)

	if rr.Code != 200 {
		t.Fatalf("want 200, got %d", rr.Code)
	}

	var resp struct {
		Data     []string `json:"data"`
		Metadata struct {
			Message string `json:"message"`
		} `json:"metadata"`
		Pagination struct {
			Page       int `json:"page"`
			PerPage    int `json:"per_page"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Pagination.Total != 42 {
		t.Errorf("pagination.total = %d, want 42", resp.Pagination.Total)
	}
	if resp.Pagination.TotalPages != 3 {
		t.Errorf("pagination.total_pages = %d, want 3", resp.Pagination.TotalPages)
	}
	if resp.Metadata.Message != "Items listed" {
		t.Errorf("metadata.message = %q, want %q", resp.Metadata.Message, "Items listed")
	}
}
