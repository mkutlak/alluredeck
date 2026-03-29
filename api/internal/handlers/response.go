package handlers

import "net/http"

// ResponseMeta holds metadata for API responses.
type ResponseMeta struct {
	Message string `json:"message"`
}

// writeSuccess writes a typed JSON response with the standard envelope:
//
//	{"data": ..., "metadata": {"message": "..."}}
func writeSuccess(w http.ResponseWriter, status int, data any, msg string) {
	writeJSON(w, status, map[string]any{
		"data":     data,
		"metadata": ResponseMeta{Message: msg},
	})
}

// writePagedSuccess writes a typed paginated JSON response:
//
//	{"data": ..., "metadata": {"message": "..."}, "pagination": {...}}
func writePagedSuccess(w http.ResponseWriter, status int, data any, msg string, pg PaginationMeta) {
	writeJSON(w, status, map[string]any{
		"data":       data,
		"metadata":   ResponseMeta{Message: msg},
		"pagination": pg,
	})
}
