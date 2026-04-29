import os
from copy import deepcopy
from pathlib import Path

import yaml

DEFAULT_CONFIG = {
    "groups": [{"name": "全部", "funds": []}],
    "refresh_interval": 60,
    "alerts": {"highlight_threshold": 2.0},
}


def _resolve_config_path() -> Path | None:
    cwd_config = Path.cwd() / "funda.yaml"
    if cwd_config.is_file():
        return cwd_config

    xdg_config_home = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))
    xdg_config = xdg_config_home / "funda" / "funda.yaml"
    if xdg_config.is_file():
        return xdg_config

    return None


def load_config() -> dict:
    config_path = _resolve_config_path()
    if config_path is None:
        return deepcopy(DEFAULT_CONFIG)

    try:
        with config_path.open(encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
            if isinstance(data, dict):
                return data
            raise ValueError("Config file content must be a mapping")
    except Exception as e:
        print(f"Failed to load config: {e}")
        return deepcopy(DEFAULT_CONFIG)
