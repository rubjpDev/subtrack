import httpx
from typing import Any
from .config import load_settings



def _client() -> httpx.AsyncClient:
    settings = load_settings()
    return httpx.AsyncClient(
        base_url=settings.api_url,
        headers={"X-API-Key": settings.api_key},
        timeout=10.0,
    )


async def api_get(path: str, params: dict[str, str] | None = None) -> Any:
    async with _client() as client:
        response = await client.get(path, params=params)
        response.raise_for_status()
        return response.json()

async def api_post(path: str, json: dict[str, object] | None = None) -> Any:
    async with _client() as client:
        response = await client.post(path, json=json)
        response.raise_for_status()
        return response.json()

async def api_patch(path: str, json: dict[str, object]) -> Any:
    async with _client() as client:
        response = await client.patch(path, json=json)
        response.raise_for_status()
        return response.json()