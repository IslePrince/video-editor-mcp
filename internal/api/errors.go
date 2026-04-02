package api

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type errorResponse struct {
	Error APIError `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{
		Error: APIError{Code: code, Message: message},
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", msg)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, "BAD_REQUEST", msg)
}

func internalError(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", msg)
}
