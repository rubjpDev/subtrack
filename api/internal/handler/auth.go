package handler

import (
	"crypto/subtle"
	"net/http"
)

// apiKeyHeader is the request header carrying the shared-secret API key.
const apiKeyHeader = "X-API-Key"

// RequireAPIKey wraps next with a check that the request carries a valid
// X-API-Key header, comparing it to apiKey using a constant-time comparison
// so response timing cannot be used to probe the key byte by byte. A missing
// header or a mismatch both yield a generic 401 — the response intentionally
// does not distinguish "missing" from "wrong" so callers cannot use it as an
// oracle to enumerate valid keys.
//
// This is a deliberately-minimal shared-secret scheme (KD-5) suitable for a
// single trusted MCP client talking to its own backend. OAuth (per-client
// credentials, token expiry, scopes) is the production follow-up and is out
// of scope here.
func RequireAPIKey(apiKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get(apiKeyHeader)

		if !constantTimeEqual(provided, apiKey) {
			writeUnauthorizedError(w)
			return
		}

		next(w, r)
	}
}

// constantTimeEqual reports whether a and b are equal using
// subtle.ConstantTimeCompare. It first checks length — which is not secret —
// and only feeds equal-length byte slices to ConstantTimeCompare, since that
// function requires its inputs to be the same length.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
