// Package handler contains HTTP edge handlers. They decode/encode JSON,
// delegate to services or pingers, and shape responses. No business logic.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger checks connectivity to a dependency (e.g. the database). It is the
// seam that lets health handlers be unit-tested without a live database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PingerFunc adapts a plain function to the Pinger interface.
type PingerFunc func(ctx context.Context) error

// Ping calls f(ctx).
func (f PingerFunc) Ping(ctx context.Context) error {
	return f(ctx)
}

const healthCheckTimeout = 2 * time.Second

type healthResponse struct {
	Status string `json:"status"`
}

// Health returns an http.HandlerFunc that responds 200 {"status":"ok"} when
// the pinger reports a healthy dependency, and 503 otherwise.
func Health(pinger Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		if err := pinger.Ping(ctx); err != nil {
			writeHealthResponse(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		writeHealthResponse(w, http.StatusOK, "ok")
	}
}

// writeHealthResponse encodes the health status as JSON with the given
// HTTP status code.
func writeHealthResponse(w http.ResponseWriter, statusCode int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: status})
}
