import pytest

@pytest.fixture(autouse=True)
def _backend_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """Every test runs as if the backend lives at http://test with a known key"""
    monkeypatch.setenv("SUBTRACK_API_URL","http://test")
    monkeypatch.setenv("SUBTRACK_API_KEY", "test-key")
