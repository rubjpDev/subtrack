package handler

import (
	"encoding/json"
	"net/http"
)

// errorResponse is the single structured JSON error shape used across all
// edge handlers. It is reused for 400 (validation, malformed body) and 404
// (not found) outcomes, and is intended to be reused by future features
// (update/cancel/auth) so clients have one error contract to parse.
//
// Shape: {"error": {"code": "...", "message": "...", "fields": {...}}}
// "fields" is omitted when there is no per-field detail.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Error codes shared across handlers. Keep these stable — clients may match
// on them.
const (
	errCodeValidation    = "validation_error"
	errCodeMalformedBody = "malformed_body"
	errCodeNotFound      = "not_found"
	errCodeUnauthorized  = "unauthorized"
)

// writeError encodes a structured error response with the given HTTP status.
func writeError(w http.ResponseWriter, statusCode int, code, message string, fields map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{
			Code:    code,
			Message: message,
			Fields:  fields,
		},
	})
}

// writeValidationError renders a 400 with per-field validation detail.
func writeValidationError(w http.ResponseWriter, fields map[string]string) {
	writeError(w, http.StatusBadRequest, errCodeValidation, "validation failed", fields)
}

// writeMalformedBodyError renders a 400 for a request body that could not be
// decoded as the expected JSON shape.
func writeMalformedBodyError(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, errCodeMalformedBody, "request body is malformed or missing required JSON fields", nil)
}

// writeNotFoundError renders a 404 for a missing resource.
func writeNotFoundError(w http.ResponseWriter, message string) {
	writeError(w, http.StatusNotFound, errCodeNotFound, message, nil)
}

// writeUnauthorizedError renders a 401 for a missing or invalid API key. The
// message is deliberately generic — it must not reveal whether the key was
// absent or simply wrong (see RequireAPIKey in auth.go).
func writeUnauthorizedError(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, errCodeUnauthorized, "missing or invalid API key", nil)
}
