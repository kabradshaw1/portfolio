from app.tools import build_tools


def _by_name(tools):
    return {t.name: t for t in tools}


def test_build_tools_returns_four_named_tools():
    tools = _by_name(build_tools("INC-PHISH-001"))
    assert set(tools) == {
        "search_alerts",
        "get_logs",
        "lookup_ioc",
        "get_asset_context",
    }


def test_search_alerts_tool_finds_login_alert():
    tools = _by_name(build_tools("INC-PHISH-001"))
    out = tools["search_alerts"].invoke({"query": "login"})
    assert "login" in out.lower()


def test_lookup_ioc_tool_reports_malicious():
    tools = _by_name(build_tools("INC-MAL-002"))
    out = tools["lookup_ioc"].invoke({"indicator": "198.51.100.7"})
    assert "malicious" in out.lower()


def test_get_logs_and_asset_tools():
    tools = _by_name(build_tools("INC-EXFIL-003"))
    logs = tools["get_logs"].invoke({"selector": "msmith"})
    assert "upload" in logs.lower()
    asset = tools["get_asset_context"].invoke({"selector": "msmith"})
    assert "resigning" in asset.lower()
