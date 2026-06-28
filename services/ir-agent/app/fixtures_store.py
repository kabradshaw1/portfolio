"""Load synthetic incident scenarios from bundled JSON fixtures.

The fixtures are the deterministic evidence corpus the investigator's tools
read from, so the whole graph is reproducible without external services.
"""

from __future__ import annotations

import json
from functools import lru_cache
from pathlib import Path

from app.models import Incident

FIXTURES_DIR = Path(__file__).resolve().parent.parent / "fixtures"


@lru_cache(maxsize=1)
def _load_all() -> dict[str, dict]:
    """Map incident id -> full fixture dict."""
    store: dict[str, dict] = {}
    for path in sorted(FIXTURES_DIR.glob("*.json")):
        data = json.loads(path.read_text())
        store[data["incident"]["id"]] = data
    return store


def _fixture(incident_id: str) -> dict:
    store = _load_all()
    if incident_id not in store:
        raise KeyError(f"Unknown incident: {incident_id}")
    return store[incident_id]


def list_incident_ids() -> list[str]:
    return sorted(_load_all().keys())


def load_incident(incident_id: str) -> Incident:
    return Incident.model_validate(_fixture(incident_id)["incident"])


def search_alerts_raw(incident_id: str, query: str) -> list[dict]:
    alerts = _fixture(incident_id).get("alerts", [])
    q = query.lower().strip()
    if not q:
        return alerts
    return [a for a in alerts if q in json.dumps(a).lower()]


def get_logs_raw(incident_id: str, selector: str) -> list[str]:
    logs = _fixture(incident_id).get("logs", {})
    if selector in logs:
        return logs[selector]
    out: list[str] = []
    for lines in logs.values():
        out.extend(line for line in lines if selector.lower() in line.lower())
    return out


def lookup_ioc_raw(incident_id: str, indicator: str) -> dict:
    ioc = _fixture(incident_id).get("ioc", {})
    return ioc.get(indicator, {"reputation": "unknown", "categories": []})


def get_asset_context_raw(incident_id: str, selector: str) -> dict:
    assets = _fixture(incident_id).get("assets", {})
    return assets.get(selector, {})
