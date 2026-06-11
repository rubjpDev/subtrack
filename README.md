# SubTrack

Track recurring subscriptions from an LLM agent — a thin MCP server over a Go backend.

![SubTrack demo](docs/demo.gif)

SubTrack is a small monorepo: a Go HTTP backend (`api/`) that owns all subscription data and
business logic, and a Python [FastMCP](https://gofastmcp.com/) server (`mcp/`) that exposes a
curated set of tools to MCP clients such as Claude Desktop. The MCP layer holds no business logic
of its own — it only speaks REST to the backend, so it's language-agnostic and the same backend
could sit behind any other client.

## Architecture

```mermaid
flowchart TD
    A["Claude Desktop / MCP client"] -->|"MCP · stdio"| B["mcp<br/>FastMCP · 7 curated tools"]
    B -->|"REST · X-API-Key"| C["api<br/>Go · net/http"]
    C -->|pgxpool| D[("PostgreSQL 16")]
```

## Tools

| Tool | Kind | Backend call |
|---|---|---|
| `list_subscriptions` | read | `GET /v1/subscriptions` |
| `get_subscription` | read | `GET /v1/subscriptions/{id}` |
| `spending_summary` | read | `GET /v1/subscriptions/summary` |
| `upcoming_charges` | read | `GET /v1/subscriptions/summary` (filtered client-side) |
| `add_subscription` | write | `POST /v1/subscriptions` |
| `update_subscription` | write | `PATCH /v1/subscriptions/{id}` |
| `cancel_subscription` | write | `POST /v1/subscriptions/{id}/cancel` |

The surface is curated, not auto-generated from the REST API: tools are task-shaped
(`upcoming_charges`, `spending_summary`) rather than a 1:1 endpoint mirror, which keeps the
agent's choices small and unambiguous.

## Quickstart

Requires [Docker](https://docs.docker.com/), Go (see `api/go.mod`), and
[uv](https://docs.astral.sh/uv/).

### One command demo

```sh
make demo
```

This starts Postgres 16 (5432) and the API (8080) in Docker, applies migrations, seeds 3 sample
subscriptions, and prints the spending summary. Run `make down` afterwards to stop the stack and
remove its data volume.

### Manual setup

```sh
# 1. Start the backend: Postgres 16 (5432) and the API (8080).
cd api
cp .env.example .env   # optional — compose ships working dev defaults
docker compose up -d
DATABASE_URL="postgres://subtrack:devpassword@localhost:5432/subtrack?sslmode=disable" \
  API_KEY=dev-local-key make migrate-up

# 2. Sanity check.
curl http://localhost:8080/healthz
curl -H "X-API-Key: dev-local-key" http://localhost:8080/v1/subscriptions

# 3. Run the MCP server.
cd ../mcp
uv sync
export SUBTRACK_API_URL=http://localhost:8080
export SUBTRACK_API_KEY=dev-local-key   # matches api/.env.example
uv run subtrack-mcp
```

> Note: `make migrate-up` runs on the host via `go run`, so it needs `DATABASE_URL` (or
> `POSTGRES_*`) and `API_KEY` set in your shell — `.env` is only read by `docker compose`, not by
> the Go binaries.

`SUBTRACK_API_URL` and `SUBTRACK_API_KEY` are mandatory — the MCP server fails fast at startup if
either is missing. Point your MCP client at `uv run subtrack-mcp` with the same two variables set,
and it connects over stdio.

## Key design decisions

- **Logic in the backend, not the MCP.** The MCP server is stateless and thin; all validation,
  spending-total and next-charge calculations live in Go.
- **Curated tool surface.** Seven task-shaped tools instead of an auto-exposed endpoint mirror.
- **Money as integer cents.** No floats anywhere in the wire shapes — enforced by a
  `CHECK (cost_cents > 0)` constraint in the schema.
- **Schema only in migrations.** The database is defined in `api/migrations/`, never created from
  application code.
- **Constant-time API-key auth.** Every `/v1/*` route requires a shared secret via `X-API-Key`,
  compared with `crypto/subtle`.

## Repo structure

```
.
├── api/   # Go backend: handler -> service -> repository -> model, Postgres, migrations
├── mcp/   # Python FastMCP server: thin REST client, curated tools
└── docs/  # demo.gif for this README
```

- [`api/README.md`](api/README.md) — backend layering, data model, endpoints, auth, migrations.
- [`mcp/README.md`](mcp/README.md) — MCP tools, auth against the backend, dev gates.

## License

MIT.
