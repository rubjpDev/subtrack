// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration for the API process.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
	// DatabaseURL is the connection string used to reach PostgreSQL.
	DatabaseURL string
	// APIKey is the shared secret that callers must present via the
	// X-API-Key header on every /v1/* route (see internal/handler.RequireAPIKey).
	// It is required: an empty key would make auth meaningless, so Load fails
	// fast rather than ever running with an empty/guessable key.
	APIKey string
}

const defaultPort = "8080"

// Load reads configuration from environment variables, applying defaults
// where appropriate, and returns an error if required values are missing.
func Load() (Config, error) {
	cfg := Config{
		Port:        envOrDefault("PORT", defaultPort),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIKey:      os.Getenv("API_KEY"),
	}

	if cfg.DatabaseURL == "" {
		dsn, err := databaseURLFromParts()
		if err != nil {
			return Config{}, err
		}
		cfg.DatabaseURL = dsn
	}

	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("config: missing required API_KEY: set it to the shared secret callers must present via X-API-Key")
	}

	return cfg, nil
}

// databaseURLFromParts builds a Postgres DSN from discrete POSTGRES_* env
// vars when DATABASE_URL is not set directly.
func databaseURLFromParts() (string, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := envOrDefault("POSTGRES_PORT", "5432")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	if host == "" || user == "" || dbName == "" {
		return "", fmt.Errorf("config: missing database configuration: set DATABASE_URL or POSTGRES_HOST/POSTGRES_USER/POSTGRES_DB (and optionally POSTGRES_PASSWORD/POSTGRES_PORT)")
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbName), nil
}

// envOrDefault returns the environment variable value for key, or fallback
// if it is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
