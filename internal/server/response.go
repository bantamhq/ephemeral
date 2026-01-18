package server

import (
	"encoding/json"
	"net/http"
)

// Response is the standard JSON response format.
type Response struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// ListResponse is the paginated list response format.
type ListResponse struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Data: data})
}

// JSONError writes a JSON error response with the given status code.
func JSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Error: msg})
}

// JSONList writes a paginated JSON list response.
func JSONList(w http.ResponseWriter, data any, cursor *string, hasMore bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ListResponse{
		Data:       data,
		NextCursor: cursor,
		HasMore:    hasMore,
	})
}
