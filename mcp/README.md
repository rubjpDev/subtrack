# mcp

A [FastMCP](https://gofastmcp.com/) server that exposes a curated set of subscription-tracking
tools to MCP clients such as Claude Desktop. It holds no business logic of its own: every tool is
a thin HTTP call to the `api` Go backend. See the [root README](../README.md) for the architecture
and full quickstart.

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

The surface is curated, not auto-generated from the REST API. Tools are task-shaped
(`upcoming_charges`, `spending_summary`) rather than a 1:1 endpoint mirror, which keeps the
agent's choices small and unambiguous. Inputs are validated with Pydantic before any HTTP call;
HTTP failures from the backend are mapped to clean `ToolError`s, never raw tracebacks.

## Auth

Every request to the backend carries the shared secret as an `X-API-Key` header
(`subtrack_mcp/api_client.py`), read from the `SUBTRACK_API_KEY` environment variable via
`subtrack_mcp/config.py`. The MCP itself has no separate auth — it's a single trusted client of
the backend.

## Quickstart

Requires [Docker](https://docs.docker.com/) and [uv](https://docs.astral.sh/uv/).

```sh
# 1. Start the backend (Go API + Postgres) from the sibling directory.
cd ../api
docker compose up -d
make migrate-up

# 2. Run the MCP server.
cd ../mcp
uv sync
export SUBTRACK_API_URL=http://localhost:8080
export SUBTRACK_API_KEY=dev-local-key   # matches api/.env.example
uv run subtrack-mcp
```

Both environment variables are mandatory — the server fails fast at startup if either is missing.
Point your MCP client at `uv run subtrack-mcp` with the same two variables set, and it connects
over stdio.

Dev gates (the same ones CI runs):

```sh
uv run ruff check .
uv run mypy .
uv run pytest -q
```

## See also

- [Root README](../README.md) — architecture, full quickstart, design decisions.
- [`api/README.md`](../api/README.md) — the Go backend that owns all business logic and the
  database.
