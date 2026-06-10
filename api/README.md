# api

The Go backend for SubTrack — system of record for recurring subscriptions and spending totals.
See the [root README](../README.md) for the architecture and full quickstart.

## Layering

Strict one-direction dependency chain:

- **handler** (`internal/handler`) — decodes HTTP requests, maps domain errors to status codes,
  encodes JSON wire shapes. No business logic.
- **service** (`internal/subscription`) — the domain: validation, create/update/cancel/list,
  and the spending-total and next-charge calculations. No HTTP or SQL.
- **repository** (`internal/postgres`) — persistence via `pgxpool`. No domain rules.
- **model** (`internal/subscription`) — the `Subscription` domain type and its input shapes.

Entry points are `cmd/api` (the server) and `cmd/migrate` (schema migrations).

## Data model

One table, `subscriptions`, defined in `migrations/`:

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | server-generated (`gen_random_uuid()`) |
| `name` | `text` | non-empty |
| `cost_cents` | `int` | `> 0` — money is integer cents, never floats |
| `currency` | `char(3)` | e.g. `EUR` |
| `cycle` | `text` | `monthly` or `yearly` |
| `billing_day` | `int` | `1`–`28` |
| `start_date` | `date` | |
| `active` | `bool` | defaults `true`; cleared by cancel |
| `created_at` / `updated_at` | `timestamptz` | server-managed |

All monetary fields on the wire are integer cents.

## Endpoints

| Method | Path | Auth | Returns |
|---|---|---|---|
| GET | `/healthz` | none | `{"status":"ok"}` when the DB is reachable |
| POST | `/v1/subscriptions` | `X-API-Key` | 201 + created subscription |
| GET | `/v1/subscriptions?active=` | `X-API-Key` | `{"subscriptions": [...]}` (all, or active only) |
| GET | `/v1/subscriptions/summary` | `X-API-Key` | monthly/annual totals + per-subscription breakdown |
| GET | `/v1/subscriptions/{id}` | `X-API-Key` | the subscription, or 404 |
| PATCH | `/v1/subscriptions/{id}` | `X-API-Key` | updated subscription (partial update) |
| POST | `/v1/subscriptions/{id}/cancel` | `X-API-Key` | subscription with `active=false` (idempotent) |

`/healthz` is registered unwrapped so monitoring can reach it without a key. Every `/v1/*` route is
wrapped by the API-key middleware.

## Auth

Every `/v1/*` route requires a shared secret in the `X-API-Key` header, compared in constant time
(`crypto/subtle`) so response timing cannot be used to probe the key. A missing or wrong key both
return a generic 401. This is a deliberately minimal scheme for a single trusted client; OAuth is
the production follow-up.

## Quickstart

```sh
cp .env.example .env          # optional — compose ships working dev defaults
docker compose up -d          # Postgres 16 (5432) + API (8080)
make migrate-up                # apply migrations
curl http://localhost:8080/healthz
```

`make migrate-down` reverts all migrations. `make demo` seeds three subscriptions and prints the
spending summary (requires the stack up and migrated first).

Key env vars: `PORT` (default `8080`), `API_KEY` (required — the server won't start without it;
dev default `dev-local-key`), and either `DATABASE_URL` or the discrete `POSTGRES_*` parts.

## Migrations

Schema lives only in `migrations/`, applied with [golang-migrate](https://github.com/golang-migrate/migrate)
via `cmd/migrate`, which reuses the server's config loader so both target the same database. No
schema is created from application code.

## See also

- [Root README](../README.md) — architecture, MCP tools, full quickstart.
- [`mcp/README.md`](../mcp/README.md) — the FastMCP server exposing this backend to LLM agents.
