from __future__ import annotations
from datetime import date, timedelta

import functools
from collections.abc import Awaitable, Callable
from typing import TypeVar

import httpx
from fastmcp import FastMCP
from fastmcp.exceptions import ToolError

from subtrack_mcp.api_client import api_get, api_post, api_patch
from subtrack_mcp.models import (
    Subscription,
    Summary,
    SubscriptionLine,
    NewSubscription,
    SubscriptionUpdate
)


# INIT LOGIC
mcp = FastMCP("subtrack-mcp")
R = TypeVar("R")

def surface_errors(fn: Callable[..., Awaitable[R]]) -> Callable[..., Awaitable[R]]:
   """Turn backed HTTP failures into clean ToolErrors, no stack traces"""

   @functools.wraps(fn)
   async def wrapper(*args: object, **kwargs: object) -> R:
       try:
           return await fn(*args, **kwargs)
       except httpx.HTTPStatusError as exc:
           raise ToolError(f"subtrack-api returned HTTP {exc.response.status_code}") from exc
       except httpx.RequestError as exc:
           raise ToolError(f"Could not reach subtrack-api: {exc}") from exc
   return wrapper

@mcp.tool
@surface_errors
async def list_subscriptions(active: bool = True) -> list[Subscription]:
    """List all subscriptions. By default only the active ones are returned."""
    data = await api_get("/v1/subscriptions", params={"active": str(active).lower()})
    items = data["subscriptions"]
    return [Subscription.model_validate(item) for item in items]

@mcp.tool
@surface_errors
async def get_subscription(subscription_id: str) -> Subscription:
    """Fetch a single subscription by id"""
    data = await api_get(f"/v1/subscriptions/{subscription_id}")
    return Subscription.model_validate(data)


@mcp.tool
@surface_errors
async def spending_summary() -> Summary:
    """Monthly and annual totals plus a per-subscription breakdown"""
    return await _fetch_summary()

@mcp.tool()
@surface_errors
async def upcoming_charges(days: int = 30) -> list[SubscriptionLine]:
    """Subscriptions whose next charge falls within the next N days"""
    summary = await _fetch_summary()
    cutoff = date.today() + timedelta(days = days)
    upcoming = [line for line in summary.subscriptions if line.next_charge_date <= cutoff]
    return sorted(upcoming, key=lambda line: line.next_charge_date)


async def _fetch_summary() -> Summary:
    """Plain helper so other tools can reuse the summary (no tool calling tool loop)"""
    data = await api_get("/v1/subscriptions/summary")
    return Summary.model_validate(data)


@mcp.tool
@surface_errors
async def add_subscription(subscription: NewSubscription) -> Subscription:
    """Create a subscription. Inputs are validated before the call"""
    data = await api_post("/v1/subscriptions", json=subscription.model_dump(mode="json"))
    return Subscription.model_validate(data)

@mcp.tool
@surface_errors
async def update_subscription(
    subscription_id: str, changes: SubscriptionUpdate) -> Subscription:
    """Patch mutable fields. Only the fields you set are sent"""
    payload = changes.model_dump(mode="json", exclude_none = True)
    data = await api_patch(f"/v1/subscriptions/{subscription_id}", json=payload)
    return Subscription.model_validate(data)

@mcp.tool
@surface_errors
async def cancel_subscription(subscription_id: str) -> Subscription:
    """Cancel a subscription (sets active=False)"""
    data = await api_post(f"/v1/subscriptions/{subscription_id}/cancel")
    return Subscription.model_validate(data)

def main() -> None:
    mcp.run()
if __name__ == "__main__":
    main()