// Package server wires HTTP routes to handlers and exposes a configured
// http.Server ready to run.
package server

import (
	"net/http"
	"time"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/handler"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
)

// New builds an *http.Server listening on addr, with routes wired to their
// handlers. The pinger is used by the /healthz route to check DB connectivity,
// subscriptionService backs the /v1/subscriptions routes, and apiKey is the
// shared secret required (via X-API-Key) on every /v1/* route. /healthz is
// registered directly on the mux, unwrapped, so it stays reachable without a
// key for orchestration/monitoring.
func New(addr string, pinger handler.Pinger, subscriptionService handler.SubscriptionService, apiKey string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.Health(pinger))

	registerSubscriptionRoutes(mux, subscriptionService, apiKey)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
	}
}

// registerSubscriptionRoutes wires the /v1/subscriptions routes to their
// handlers, using Go 1.22+ method+path patterns and PathValue for the id.
// Every route is wrapped with the X-API-Key middleware via protect, so it is
// obvious at a glance that none of the six routes is left unauthenticated.
func registerSubscriptionRoutes(mux *http.ServeMux, svc handler.SubscriptionService, apiKey string) {
	protect := func(h http.HandlerFunc) http.HandlerFunc {
		return handler.RequireAPIKey(apiKey, h)
	}

	mux.HandleFunc("POST /v1/subscriptions", protect(handler.CreateSubscription(svc)))
	mux.HandleFunc("GET /v1/subscriptions", protect(handler.ListSubscriptions(svc)))
	mux.HandleFunc("GET /v1/subscriptions/summary", protect(handler.SummarySubscriptions(svc)))
	mux.HandleFunc("GET /v1/subscriptions/{id}", protect(handler.GetSubscription(svc)))
	mux.HandleFunc("PATCH /v1/subscriptions/{id}", protect(handler.UpdateSubscription(svc)))
	mux.HandleFunc("POST /v1/subscriptions/{id}/cancel", protect(handler.CancelSubscription(svc)))
}
