import os
from dataclasses import dataclass




@dataclass(frozen=True)
class Settings:
    api_url: str
    api_key: str

def load_settings() -> Settings:
    """Read config from the enviroment, failing fast if anything is missing."""
    try:
        return Settings(
            api_url=os.environ["SUBTRACK_API_URL"],
            api_key=os.environ["SUBTRACK_API_KEY"],
        )
    except KeyError as missing:
        raise RuntimeError(f"Missing required enviroment variable: {missing}. "
            "Set SUBTRACK_API_URL and SUBTRACK_API_KEY") from missing