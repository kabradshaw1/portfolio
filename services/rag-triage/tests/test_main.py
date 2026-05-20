from app.main import app
from app.models import Diagnosis, Scores, TriageResponse, TriageSubject
from fastapi.testclient import TestClient


def test_health_returns_healthy():
    client = TestClient(app)

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json()["status"] == "healthy"


def test_triage_eval_run_endpoint(monkeypatch):
    class FakeService:
        async def triage_eval_run(self, eval_id, metric, limit):
            assert eval_id == "eval-1"
            assert metric is None
            assert limit is None
            return TriageResponse(
                subject=TriageSubject(type="eval_run", eval_id="eval-1"),
                status="completed",
                aggregate_scores=Scores(context_precision=0.4),
                config={},
                diagnosis=Diagnosis(
                    primary_failure_mode="retrieval_precision",
                    confidence="medium",
                    summary="summary",
                ),
                clusters=[],
                cases=[],
                recommendations=[],
                metric="context_precision",
            )

    class FakeClient:
        async def close(self):
            return None

    service = FakeService()
    service._eval_client = FakeClient()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post("/triage/eval-run", json={"eval_id": "eval-1"})

    assert response.status_code == 200
    assert response.json()["diagnosis"]["primary_failure_mode"] == (
        "retrieval_precision"
    )
