from fastmcp.exceptions import ToolError
import httpx
import respx
from fastmcp import Client
import pytest

from subtrack_mcp.server import mcp

SAMPLE = {
    "id": "sub_1", "name": "Netflix", "cost_cents": 1599, "currency": "EUR",
    "cycle": "monthly", "billing_day": 5, "start_date": "2024-01-05", "active": True,
    "created_at": "2024-01-05T00:00:00Z", "updated_at": "2024-01-05T00:00:00Z",
}

@respx.mock
async def test_list_subscriptions_returns_models() -> None:
    respx.get("http://test/v1/subscriptions").mock(
        return_value=httpx.Response(200, json={"subscriptions": [SAMPLE]}))
    async with Client(mcp) as client:
        result = await client.call_tool("list_subscriptions",{"active": True})
    subs = result.data
    assert len(subs) == 1
    assert subs[0].name == "Netflix"


@respx.mock
async def test_upcoming_charges_filters_by_window() -> None:
    summary = {
            "monthly_total_cents": 1599, "annual_total_cents": 19188,
            "subscriptions": [
                {"id": "a", "name": "Soon", "paid_to_date_cents": 0,
                 "next_charge_date": "2099-01-05"},   # fuera de ventana
                {"id": "b", "name": "Now", "paid_to_date_cents": 0,
                 "next_charge_date": "2000-01-05"},    # dentro
            ],
        }
    respx.get("http://test/v1/subscriptions/summary").mock(
        return_value=httpx.Response(200, json=summary)
    )
    async with Client(mcp) as client:
        result = await client.call_tool("upcoming_charges", {"days": 30})
    names = [line.name for line in result.data]
    assert names == ["Now"]

@respx.mock
async def test_get_subscription_404_becomes_tool_error() -> None:
    respx.get("http://test/v1/subscriptions/blabla").mock(
        return_value=httpx.Response(404, json={"error": "not found"})
    )
    with pytest.raises(ToolError):
        async with Client(mcp) as client:
            await client.call_tool("get_subscription", {"subscription_id": "blabla"})