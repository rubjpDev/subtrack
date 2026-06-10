#!/bin/sh
# Seeds 3 sample subscriptions into a running subtrack-api and prints the
# spending summary. Assumes the stack is already up and migrated:
#
#   docker compose up -d && make migrate-up
#
# Respects:
#   BASE_URL - API base URL (default http://localhost:8080)
#   API_KEY  - X-API-Key header value (default dev-local-key)
set -eu

BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-local-key}"

pretty() {
  if command -v jq >/dev/null 2>&1; then
    jq .
  else
    cat
  fi
}

post_subscription() {
  body="$1"
  curl -sS -X POST "$BASE_URL/v1/subscriptions" \
    -H "X-API-Key: $API_KEY" \
    -H "Content-Type: application/json" \
    -d "$body"
}

echo "Seeding subscriptions..."

post_subscription '{
  "name": "Netflix",
  "cost_cents": 1599,
  "currency": "EUR",
  "cycle": "monthly",
  "billing_day": 5,
  "start_date": "2024-01-05"
}' | pretty

post_subscription '{
  "name": "Spotify",
  "cost_cents": 1199,
  "currency": "USD",
  "cycle": "monthly",
  "billing_day": 15,
  "start_date": "2024-02-15"
}' | pretty

post_subscription '{
  "name": "Amazon Prime",
  "cost_cents": 8999,
  "currency": "GBP",
  "cycle": "yearly",
  "billing_day": 20,
  "start_date": "2024-03-20"
}' | pretty

echo ""
echo "Spending summary:"

curl -sS "$BASE_URL/v1/subscriptions/summary" \
  -H "X-API-Key: $API_KEY" | pretty
