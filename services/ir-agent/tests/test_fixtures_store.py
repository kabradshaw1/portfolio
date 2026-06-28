import pytest
from app.fixtures_store import (
    get_asset_context_raw,
    get_logs_raw,
    list_incident_ids,
    load_incident,
    lookup_ioc_raw,
    search_alerts_raw,
)
from app.models import Incident


def test_lists_three_incidents():
    ids = list_incident_ids()
    assert set(ids) == {"INC-PHISH-001", "INC-MAL-002", "INC-EXFIL-003"}


def test_loads_incident_as_model():
    inc = load_incident("INC-PHISH-001")
    assert isinstance(inc, Incident)
    assert inc.source == "email-gw"


def test_load_unknown_incident_raises():
    with pytest.raises(KeyError):
        load_incident("INC-NOPE-999")


def test_search_alerts_filters_by_query():
    hits = search_alerts_raw("INC-PHISH-001", "login")
    assert any("login" in h["text"].lower() for h in hits)


def test_lookup_ioc_returns_reputation():
    rep = lookup_ioc_raw("INC-MAL-002", "198.51.100.7")
    assert rep["reputation"] == "malicious"


def test_get_logs_and_asset_context():
    logs = get_logs_raw("INC-EXFIL-003", "msmith")
    assert any("upload" in line for line in logs)
    asset = get_asset_context_raw("INC-EXFIL-003", "msmith")
    assert asset["status"] == "resigning"
