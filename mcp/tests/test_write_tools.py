import httpx
import pytest
import respx
from fastmcp import Client
from fastmcp.exceptions import ToolError
from subtrack_mcp.server import mcp

CREATED = {
    "id": "sub_9", "name": "Spotify", "cost_cents": 999, "currency": "EUR",
    "cycle": "monthly", "billing_day": 12, "start_date": "2025-02-01", "active": True,
    "created_at": "2025-02-01T00:00:00Z", "updated_at": "2025-02-01T00:00:00Z",
}

@respx.mock
async def test_add_subscription_happy_path() -> None:
    route = respx.post("http://test/v1/subscriptions").mock(
        return_value=httpx.Response(201, json=CREATED)
    )
    payload = {
            "name": "Spotify", "cost_cents": 999, "currency": "EUR",
            "cycle": "monthly", "billing_day": 12, "start_date": "2025-02-01",
        }
    async with Client(mcp) as client:
        result = await client.call_tool("add_subscription", {"subscription": payload})
    assert route.called
    assert result.data.id == "sub_9"

@respx.mock
async def test_update_only_sends_changed_fields() -> None:
    route = respx.patch("http://test/v1/subscriptions/sub_9").mock(
        return_value=httpx.Response(200, json={**CREATED, "cost_cents": 1099})
    )
    async with Client(mcp) as client:
        result = await client.call_tool(
            "update_subscription", {"subscription_id": "sub_9", "changes": {"cost_cents": 1099}})
    import json as _json
    assert _json.loads(route.calls.last.request.content) == {"cost_cents": 1099}
    assert result.data.cost_cents == 1099


@respx.mock
async def test_cancel_returns_inactive() -> None:
    respx.post("http://test/v1/subscriptions/sub_9/cancel").mock(
        return_value=httpx.Response(200, json={**CREATED, "active": False})
    )
    async with Client(mcp) as client:
        result = await client.call_tool("cancel_subscription", {"subscription_id": "sub_9"})
    assert result.data.active is False



async def test_add_rejects_bad_input() -> None:
    bad = {
        "name": "Bad", "cost_cents": -5, "currency": "EUR",
        "cycle": "monthly", "billing_day": 12, "start_date": "2025-02-01",
    }
    with pytest.raises(ToolError):
        async with Client(mcp) as client:
            await client.call_tool("add_subscription", {"subscription": bad})


@respx.mock
async def test_add_subscription_billing_day_one_accepted() -> None:
    route = respx.post("http://test/v1/subscriptions").mock(
        return_value=httpx.Response(201, json={**CREATED, "billing_day": 1})
    )
    payload = {
            "name": "Spotify", "cost_cents": 999, "currency": "EUR",
            "cycle": "monthly", "billing_day": 1, "start_date": "2025-02-01",
        }
    async with Client(mcp) as client:
        result = await client.call_tool("add_subscription", {"subscription": payload})
    assert route.called
    assert result.data.billing_day == 1


@respx.mock
async def test_update_missing_id_surfaces_404() -> None:
    respx.patch("http://test/v1/subscriptions/ghost").mock(
        return_value=httpx.Response(404, json={"error": "not found"})
    )
    with pytest.raises(ToolError):
        async with Client(mcp) as client:
            await client.call_tool(
                "update_subscription",
                {"subscription_id": "ghost", "changes": {"name": "x"}},
            )