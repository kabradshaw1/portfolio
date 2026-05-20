import httpx
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
        closed = False

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

        async def close(self):
            self.closed = True
            return None

    service = FakeService()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post("/triage/eval-run", json={"eval_id": "eval-1"})

    assert response.status_code == 200
    assert response.json()["diagnosis"]["primary_failure_mode"] == (
        "retrieval_precision"
    )
    assert service.closed


def test_triage_eval_run_endpoint_maps_upstream_errors(monkeypatch):
    class FakeService:
        closed = False

        async def triage_eval_run(self, eval_id, metric, limit):
            raise httpx.ConnectError("connection refused")

        async def close(self):
            self.closed = True

    service = FakeService()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post("/triage/eval-run", json={"eval_id": "eval-1"})

    assert response.status_code == 502
    assert response.json()["detail"] == "eval API request failed"
    assert service.closed


def test_triage_comparison_endpoint(monkeypatch):
    class FakeService:
        closed = False

        async def triage_comparison(
            self,
            baseline_eval_id,
            candidate_eval_id,
            metric,
            limit,
        ):
            assert baseline_eval_id == "base"
            assert candidate_eval_id == "cand"
            assert metric is None
            assert limit is None
            return TriageResponse(
                subject=TriageSubject(
                    type="comparison",
                    baseline_eval_id="base",
                    candidate_eval_id="cand",
                ),
                status="completed",
                aggregate_scores=Scores(context_precision=0.3),
                config={"metric_delta": -0.5},
                diagnosis=Diagnosis(
                    primary_failure_mode="retrieval_precision",
                    confidence="high",
                    summary="summary",
                ),
                clusters=[],
                cases=[],
                recommendations=[],
                metric="context_precision",
            )

        async def close(self):
            self.closed = True
            return None

    service = FakeService()
    monkeypatch.setattr("app.main.build_service", lambda: service)
    client = TestClient(app)

    response = client.post(
        "/triage/comparison",
        json={"baseline_eval_id": "base", "candidate_eval_id": "cand"},
    )

    assert response.status_code == 200
    assert response.json()["config"]["metric_delta"] == -0.5
    assert service.closed
