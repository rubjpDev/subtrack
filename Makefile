.PHONY: demo down

# Spin up Postgres + the API in Docker, apply migrations, seed sample
# subscriptions, and print the spending summary. Safe to re-run.
demo:
	cd api && docker compose up -d --wait
	cd api && \
	  DATABASE_URL="postgres://subtrack:devpassword@localhost:5432/subtrack?sslmode=disable" \
	  API_KEY=dev-local-key \
	  go run ./cmd/migrate up
	cd api && BASE_URL=http://localhost:8080 API_KEY=dev-local-key ./scripts/demo.sh

# Stop the stack and remove its data volume.
down:
	cd api && docker compose down -v
