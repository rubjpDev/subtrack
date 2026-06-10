// Command api boots the subtrack-api HTTP server: it loads configuration,
// connects to PostgreSQL via pgxpool (failing fast if unreachable), and
// serves the HTTP API.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/config"
	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/postgres"
	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/server"
	"github.com/rubenjpdev/subtrack-mcp/subtrack-api/internal/subscription"
)

const startupPingTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatalf("subtrack-api: %v", err)
	}
}

// run wires configuration, the database pool and the HTTP server together.
// It returns an error rather than calling os.Exit directly so it stays
// testable in isolation from process-level concerns.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := connectToDatabase(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	subscriptionRepo := postgres.NewSubscriptionRepository(pool)
	subscriptionService := subscription.NewService(subscriptionRepo)

	addr := ":" + cfg.Port
	srv := server.New(addr, pool, subscriptionService, cfg.APIKey)

	log.Printf("subtrack-api listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// connectToDatabase creates a pgx connection pool and pings it once with a
// timeout so that startup fails fast (with a clear error) when Postgres is
// unreachable, rather than serving traffic against a broken dependency.
func connectToDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, errFailedToCreatePool(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), startupPingTimeout)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errFailedToReachDatabase(err)
	}

	return pool, nil
}

func errFailedToCreatePool(err error) error {
	return &startupError{stage: "creating database pool", cause: err}
}

func errFailedToReachDatabase(err error) error {
	return &startupError{stage: "pinging database", cause: err}
}

// startupError wraps a failure that occurred during startup with a stage
// label, producing a clear, actionable message for operators.
type startupError struct {
	stage string
	cause error
}

func (e *startupError) Error() string {
	return e.stage + ": " + e.cause.Error()
}

func (e *startupError) Unwrap() error {
	return e.cause
}
