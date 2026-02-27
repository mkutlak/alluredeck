package handlers

import (
	"net/http"
	"strconv"
)

// PaginationParams holds validated page and per-page values.
type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// PaginationMeta holds pagination metadata for API responses.
type PaginationMeta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

const (
	defaultPage    = 1
	defaultPerPage = 20
	maxPerPage     = 100
)

// parsePagination extracts page and per_page from query parameters with
// defaults (page=1, per_page=20) and a max per_page of 100.
func parsePagination(r *http.Request) PaginationParams {
	page := queryInt(r, "page", defaultPage)
	perPage := queryInt(r, "per_page", defaultPerPage)

	if page < 1 {
		page = defaultPage
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return PaginationParams{Page: page, PerPage: perPage}
}

// newPaginationMeta creates pagination metadata from the given parameters.
func newPaginationMeta(page, perPage, total int) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	return PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}

func queryInt(r *http.Request, key string, fallback int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
