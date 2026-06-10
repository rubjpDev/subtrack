.PHONY: migrate-up migrate-down demo

# Apply all pending migrations against the configured database (DATABASE_URL
# or POSTGRES_* environment variables — see internal/config).
migrate-up:
	go run ./cmd/migrate up

# Fully revert all applied migrations against the configured database.
migrate-down:
	go run ./cmd/migrate down

# Seed 3 sample subscriptions and print the spending summary.
# Prerequisite: the stack must already be up and migrated:
#   docker compose up -d && make migrate-up
# Respects BASE_URL (default http://localhost:8080) and API_KEY
# (default dev-local-key).
demo:
	./scripts/demo.sh
