package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testAPIKey = "test-api-key-12345"

// sentinelNext records whether it was invoked, letting tests assert that the
// middleware short-circuits on rejection and delegates on success.
func sentinelNext(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}
}

func TestRequireAPIKey_MissingHeaderReturnsUnauthorized(t *testing.T) {
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	rec := httptest.NewRecorder()

	RequireAPIKey(testAPIKey, sentinelNext(&called)).ServeHTTP(rec, req)

	if called {
		t.Error("next was called, want it to be skipped when the API key header is missing")
	}
	assertUnauthorized(t, rec)
}

func TestRequireAPIKey_WrongKeyReturnsUnauthorized(t *testing.T) {
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	req.Header.Set(apiKeyHeader, "wrong-key")
	rec := httptest.NewRecorder()

	RequireAPIKey(testAPIKey, sentinelNext(&called)).ServeHTTP(rec, req)

	if called {
		t.Error("next was called, want it to be skipped when the API key is wrong")
	}
	assertUnauthorized(t, rec)
}

func TestRequireAPIKey_CorrectKeyCallsNext(t *testing.T) {
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	req.Header.Set(apiKeyHeader, testAPIKey)
	rec := httptest.NewRecorder()

	RequireAPIKey(testAPIKey, sentinelNext(&called)).ServeHTTP(rec, req)

	if !called {
		t.Error("next was not called, want it to be invoked when the API key matches")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// assertUnauthorized checks that rec carries a 401 with the structured
// errorResponse shape and the "unauthorized" code.
func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Error.Code != errCodeUnauthorized {
		t.Errorf("error code = %q, want %q", body.Error.Code, errCodeUnauthorized)
	}
	if body.Error.Message == "" {
		t.Error("error message should not be empty")
	}
}
