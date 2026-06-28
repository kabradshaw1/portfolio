import app.main as main_module
from app.models import IRReport, TriageResult
from fastapi.testclient import TestClient


def test_health_ok(monkeypatch):
    client = TestClient(main_module.app)
    resp = client.get("/health")
    assert resp.status_code in (200, 503)
    assert "status" in resp.json()


def test_investigate_streams_report(monkeypatch):
    # Replace the compiled graph with a fake that streams node outputs.
    class _FakeGraph:
        def stream(self, _state):
            yield {
                "triage": TriageResult(
                    severity="high", category="phishing", confidence=0.9, rationale="r"
                )
            }
            yield {
                "report": IRReport(
                    executive_summary="e", severity="high", confidence=0.8
                )
            }

    monkeypatch.setattr(main_module, "_graph_app", _FakeGraph())
    # Bypass auth dependency
    main_module.app.dependency_overrides[main_module.require_auth] = lambda: "tester"

    client = TestClient(main_module.app)
    resp = client.post("/investigate", json={"incident_id": "INC-PHISH-001"})
    assert resp.status_code == 200
    body = resp.text
    assert "triage" in body
    assert "report" in body
    main_module.app.dependency_overrides.clear()


def test_investigate_unknown_incident_400(monkeypatch):
    main_module.app.dependency_overrides[main_module.require_auth] = lambda: "tester"
    client = TestClient(main_module.app)
    resp = client.post("/investigate", json={"incident_id": "NOPE"})
    assert resp.status_code == 400
    main_module.app.dependency_overrides.clear()
