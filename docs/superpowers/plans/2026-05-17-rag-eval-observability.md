# RAG Eval Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Build local, agent-facing observability for RAG eval experiments so eval runs can be explained with metadata, lifecycle logs, upstream failures, and performance signals.

**Architecture:** `services/eval` becomes the source of structured lifecycle logs and eval orchestration metrics. `go/eval-mcp-service` summarizes durable run state from the eval API, while `go/observability-mcp-service` safely queries local or Grafana-gateway metrics/logs for eval-specific evidence.

**Tech Stack:** Python 3/FastAPI, structlog, Prometheus Python client, pytest, Go MCP services, Go unit tests, local Prometheus config.

---

## File Structure

- Modify `services/eval/app/main.py`: configure shared logging/tracing, pass run correlation context into background work, record run-level metrics/logs, and expose stale-running metric refresh.
- Modify `services/eval/app/evaluator.py`: accept run context, emit item lifecycle logs, measure per-item stages, and propagate bounded upstream failure reasons.
- Modify `services/eval/app/rag_client.py`: time chat `/search` and `/chat` calls, classify upstream failures, and record upstream metrics.
- Modify `services/eval/app/metrics.py`: add eval run, item, upstream, and stale-running metric families.
- Modify `services/eval/app/db.py`: add a read-only stale running run count helper.
- Modify `services/eval/tests/test_main.py`, `services/eval/tests/test_evaluator.py`, `services/eval/tests/test_rag_client.py`, `services/eval/tests/test_db.py`: add focused tests for metrics, logs, failure paths, and stale-running calculations.
- Modify `services/chat/tests/test_metrics.py`: assert existing rerank metrics remain exposed.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: add run evidence summary logic.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: test run evidence and timeout guidance.
- Modify `go/eval-mcp-service/internal/mcpserver/server.go` and `server_test.go`: expose `get_eval_run_evidence`.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: add fields to deserialize evidence-relevant config/results already returned by eval API.
- Modify `go/observability-mcp-service/internal/workflows/catalog.go`: allowlist `eval` and add eval metric queries.
- Modify `go/observability-mcp-service/internal/workflows/service.go`: add `InvestigateEvalRun` and include eval in AI pipeline evidence.
- Modify `go/observability-mcp-service/internal/workflows/service_test.go` and `internal/mcpserver/server_test.go`: test eval allowlist, eval-run investigation, partial source failures, and MCP tool wiring.
- Modify `go/observability-mcp-service/internal/mcpserver/server.go`: expose `investigate_eval_run`.
- Modify `monitoring/prometheus.yml`: scrape `/metrics` for local `chat`, `ingestion`, `debug`, and `eval`.
- Modify `go/observability-mcp-service/README.md` and `go/eval-mcp-service/README.md`: document local evidence workflow and fallback order.

## Task 1: Add Eval Metric Definitions

**Files:**
- Modify: `services/eval/app/metrics.py`
- Test: `services/eval/tests/test_main.py`

- [x] **Step 1: Write the failing metrics exposure test**

Add a test near the existing metrics tests in `services/eval/tests/test_main.py`:

```python
def test_metrics_contains_eval_observability_metrics(client):
    response = client.get("/metrics")

    assert response.status_code == 200
    body = response.text
    assert "eval_run_duration_seconds" in body
    assert "eval_item_duration_seconds" in body
    assert "eval_upstream_request_duration_seconds" in body
    assert "eval_upstream_failures_total" in body
    assert "eval_runs_total" in body
    assert "eval_items_total" in body
    assert "eval_stale_running_runs" in body
```

- [x] **Step 2: Run the failing test**

Run:

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-observability-rag-evidence
PYTHONPATH=services:services/eval pytest services/eval/tests/test_main.py::test_metrics_contains_eval_observability_metrics -q
```

Expected: FAIL because the new metric names are not registered.

- [x] **Step 3: Add metric families**

In `services/eval/app/metrics.py`, keep the existing metrics and add:

```python
eval_runs_total = Counter(
    "eval_runs_total",
    "Evaluation runs by terminal status",
    ["status", "requested_rerank"],
)

eval_items_total = Counter(
    "eval_items_total",
    "Evaluation items processed by status",
    ["status", "requested_rerank"],
)

eval_item_duration_seconds = Histogram(
    "eval_item_duration_seconds",
    "Evaluation item duration by stage",
    ["stage", "requested_rerank"],
    buckets=[0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120],
)

eval_upstream_request_duration_seconds = Histogram(
    "eval_upstream_request_duration_seconds",
    "Evaluation upstream request duration",
    ["service", "operation", "outcome", "requested_rerank"],
    buckets=[0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60],
)

eval_upstream_failures_total = Counter(
    "eval_upstream_failures_total",
    "Evaluation upstream failures by bounded reason",
    ["service", "operation", "reason", "requested_rerank"],
)

eval_stale_running_runs = Gauge(
    "eval_stale_running_runs",
    "Running evaluation runs older than the stale threshold",
    ["threshold"],
)
```

Update `eval_run_duration_seconds` to include labels:

```python
eval_run_duration_seconds = Histogram(
    "eval_run_duration_seconds",
    "Duration of a full evaluation run",
    ["status", "requested_rerank"],
    buckets=[10, 30, 60, 120, 300, 600, 1200],
)
```

- [x] **Step 4: Run the metrics test**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_main.py::test_metrics_contains_eval_observability_metrics -q
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add services/eval/app/metrics.py services/eval/tests/test_main.py
git commit -m "feat: add eval observability metrics"
```

## Task 2: Add Structured Eval Logging And Run Context

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_main.py`
- Test: `services/eval/tests/test_evaluator.py`

- [x] **Step 1: Write failing log-context tests**

In `services/eval/tests/test_evaluator.py`, add a unit test around a one-item successful run with `MagicMock`, `AsyncMock`, and the existing `JudgeScores` type:

```python
@pytest.mark.asyncio
async def test_run_evaluation_logs_item_lifecycle(caplog, mock_search_results, mock_chat_answer):
    caplog.set_level("INFO")
    items = [{"query": "Where is the warranty?", "expected_answer": "In the warranty section."}]
    rag_client = MagicMock(spec=RAGClient)
    rag_client.search = AsyncMock(return_value=mock_search_results)
    rag_client.ask = AsyncMock(return_value=mock_chat_answer)
    judge = AsyncMock(
        return_value=JudgeScores(
            faithfulness=0.9,
            answer_relevancy=0.8,
            reasons={
                "faithfulness": "supported",
                "answer_relevancy": "direct",
            },
        )
    )

    aggregate, results = await run_evaluation(
        items=items,
        rag_client=rag_client,
        collection="documents",
        llm_provider="ollama",
        llm_base_url="http://ollama:11434",
        llm_model="qwen2.5:14b",
        llm_api_key="",
        rerank=True,
        judge=judge,
        run_context={
            "eval_id": "eval-1",
            "dataset_id": "dataset-1",
            "collection": "documents",
            "requested_rerank": True,
            "item_count": 1,
        },
    )

    assert aggregate["faithfulness"] == 0.9
    assert len(results) == 1
    events = [record.getMessage() for record in caplog.records]
    assert any("eval_item_started" in event for event in events)
    assert any("eval_item_completed" in event for event in events)
    assert not any("Where is the warranty?" in event for event in events)
```

In `services/eval/tests/test_main.py`, add a background task success test that asserts `eval_run_started` and `eval_run_completed` appear when `_run_evaluation_task` is invoked with fake dependencies already used by the file.

- [x] **Step 2: Run the failing tests**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_evaluator.py::test_run_evaluation_logs_item_lifecycle services/eval/tests/test_main.py -q
```

Expected: FAIL because eval does not yet accept run context or emit structured lifecycle logs.

- [x] **Step 3: Configure shared logging in eval**

In `services/eval/app/main.py`, import and call shared logging/tracing like chat:

```python
import structlog
from shared.logging import RequestLoggingMiddleware, configure_logging
from shared.tracing import configure_tracing, instrument_app

logger = structlog.get_logger()

configure_logging(service_name="eval")
configure_tracing(service_name="eval")
```

Add middleware and instrumentation after app creation:

```python
app.add_middleware(RequestLoggingMiddleware)
instrument_app(app)
```

Remove the old `logging.getLogger(__name__)` logger.

- [x] **Step 4: Pass run context into the background task**

Change `_run_evaluation_task` in `services/eval/app/main.py` to accept:

```python
async def _run_evaluation_task(
    eval_id: str,
    items: list[dict],
    collection: str | None,
    rerank: bool = False,
    dataset_id: str | None = None,
    experiment_id: str | None = None,
    experiment_label: str | None = None,
    baseline_eval_id: str | None = None,
):
```

Create:

```python
run_context = {
    "eval_id": eval_id,
    "dataset_id": dataset_id,
    "experiment_id": experiment_id,
    "experiment_label": experiment_label,
    "collection": coll_name,
    "requested_rerank": rerank,
    "baseline_eval_id": baseline_eval_id,
    "item_count": len(items),
}
```

Log start:

```python
logger.info("eval_run_started", **run_context)
```

Pass `run_context=run_context` into `run_evaluation`.

When adding the background task in `start_evaluation`, pass `body.dataset_id`,
`body.experiment_id`, `body.experiment_label`, and `body.baseline_eval_id`.

- [x] **Step 5: Add item lifecycle logging**

In `services/eval/app/evaluator.py`, add `run_context: dict | None = None` to
`run_evaluation` and `build_evaluation_dataset`.

Add helpers:

```python
def query_hash(query: str) -> str:
    return hashlib.sha256(query.encode("utf-8")).hexdigest()[:12]


def _log_context(run_context: dict | None, item: dict | None = None, index: int | None = None) -> dict:
    context = dict(run_context or {})
    if item is not None:
        context["query_hash"] = query_hash(item["query"])
    if index is not None:
        context["item_index"] = index
    return {key: value for key, value in context.items() if value is not None}
```

Log around each item without raw query text:

```python
item_context = _log_context(run_context, item=item, index=index)
logger.info("eval_item_started", **item_context)
...
logger.info("eval_item_completed", **item_context)
```

- [x] **Step 6: Run the log tests**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_evaluator.py::test_run_evaluation_logs_item_lifecycle services/eval/tests/test_main.py -q
```

Expected: PASS for the new tests and existing eval main tests.

- [x] **Step 7: Commit**

Run:

```bash
git add services/eval/app/main.py services/eval/app/evaluator.py services/eval/tests/test_main.py services/eval/tests/test_evaluator.py
git commit -m "feat: add structured eval lifecycle logs"
```

## Task 3: Instrument Eval Upstream Calls And Item Durations

**Files:**
- Modify: `services/eval/app/rag_client.py`
- Modify: `services/eval/app/evaluator.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_rag_client.py`
- Test: `services/eval/tests/test_evaluator.py`
- Test: `services/eval/tests/test_main.py`

- [x] **Step 1: Write failing upstream failure test**

In `services/eval/tests/test_rag_client.py`, add a test with `httpx.MockTransport` returning 503 for `/search`:

```python
async def test_search_records_upstream_failure_metric():
    transport = httpx.MockTransport(lambda request: httpx.Response(503, json={"detail": "down"}))
    client = RAGClient(base_url="http://chat", transport=transport, telemetry_context={"eval_id": "eval-1", "requested_rerank": True})

    with pytest.raises(httpx.HTTPStatusError):
        await client.search("question", collection="documents", limit=5, rerank=True)

    metrics = generate_latest(REGISTRY).decode()
    assert 'eval_upstream_failures_total{operation="search",reason="http_status",requested_rerank="true",service="chat"}' in metrics
    await client.close()
```

Use the test file's existing import style. If it already uses isolated
registries, follow that pattern instead of the global `REGISTRY`.

- [x] **Step 2: Run the failing upstream test**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_rag_client.py::test_search_records_upstream_failure_metric -q
```

Expected: FAIL because `RAGClient` does not record upstream metrics.

- [x] **Step 3: Add upstream telemetry to `RAGClient`**

In `services/eval/app/rag_client.py`, accept optional context:

```python
def __init__(
    self,
    base_url: str,
    transport: httpx.AsyncBaseTransport | None = None,
    telemetry_context: dict | None = None,
):
    self._telemetry_context = telemetry_context or {}
```

Add helpers:

```python
def _rerank_label(self) -> str:
    return str(bool(self._telemetry_context.get("requested_rerank"))).lower()


def _classify_failure(exc: Exception) -> str:
    if isinstance(exc, httpx.HTTPStatusError):
        return "http_status"
    if isinstance(exc, httpx.TimeoutException):
        return "timeout"
    if isinstance(exc, httpx.ConnectError):
        return "connect"
    return exc.__class__.__name__
```

Wrap `/search` and `/chat` calls with timing, success metric, failure metric,
and `logger.warning("eval_upstream_call_failed", ...)`.

- [x] **Step 4: Record item stage durations**

In `services/eval/app/evaluator.py`, observe `eval_item_duration_seconds` for:

- `search`
- `chat`
- `judge`
- `score`
- `total`

Use `requested_rerank` string labels `"true"` or `"false"`.

- [x] **Step 5: Update run-level metrics**

In `services/eval/app/main.py`, update existing duration observation:

```python
eval_run_duration_seconds.labels(
    status="completed",
    requested_rerank=str(rerank).lower(),
).observe(time.perf_counter() - start)
eval_runs_total.labels(status="completed", requested_rerank=str(rerank).lower()).inc()
```

In the exception path, observe failed duration and increment `eval_runs_total`
with `status="failed"` after `db.fail_evaluation`.

- [x] **Step 6: Run eval telemetry tests**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py services/eval/tests/test_main.py -q
```

Expected: PASS.

- [x] **Step 7: Commit**

Run:

```bash
git add services/eval/app/rag_client.py services/eval/app/evaluator.py services/eval/app/main.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py services/eval/tests/test_main.py
git commit -m "feat: instrument eval upstream calls"
```

## Task 4: Add Stale Running Eval Evidence

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/app/main.py`
- Test: `services/eval/tests/test_db.py`
- Test: `services/eval/tests/test_main.py`

- [x] **Step 1: Write failing DB stale-count test**

In `services/eval/tests/test_db.py`, add:

```python
async def test_count_running_evaluations_older_than(tmp_path):
    db = EvalDB(str(tmp_path / "eval.db"))
    await db.init()
    dataset_id = await db.create_dataset("stale-dataset", [{"query": "q", "expected_answer": "a"}])
    eval_id = await db.create_evaluation(dataset_id=dataset_id, collection="documents")
    await db._db.execute(
        "UPDATE evaluations SET created_at = ? WHERE id = ?",
        ("2000-01-01T00:00:00+00:00", eval_id),
    )
    await db._db.commit()

    assert await db.count_running_evaluations_older_than(1800) == 1
    await db.close()
```

- [x] **Step 2: Run the failing DB test**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_db.py::test_count_running_evaluations_older_than -q
```

Expected: FAIL because the helper does not exist.

- [x] **Step 3: Implement stale-running DB helper**

In `services/eval/app/db.py`, add:

```python
async def count_running_evaluations_older_than(self, stale_after_seconds: int) -> int:
    cutoff = datetime.now(_UTC).timestamp() - stale_after_seconds
    cursor = await self._db.execute(
        "SELECT created_at FROM evaluations WHERE status = 'running'"
    )
    rows = await cursor.fetchall()
    count = 0
    for row in rows:
        created_at = datetime.fromisoformat(row["created_at"]).timestamp()
        if created_at < cutoff:
            count += 1
    return count
```

- [x] **Step 4: Add metric refresh helper**

In `services/eval/app/main.py`, add:

```python
STALE_RUNNING_THRESHOLD_SECONDS = 30 * 60


async def refresh_stale_running_metric(db: EvalDB) -> int:
    stale_count = await db.count_running_evaluations_older_than(
        STALE_RUNNING_THRESHOLD_SECONDS
    )
    eval_stale_running_runs.labels(
        threshold=f"{STALE_RUNNING_THRESHOLD_SECONDS}s"
    ).set(stale_count)
    if stale_count:
        logger.warning(
            "eval_run_stale_detected",
            stale_count=stale_count,
            stale_after_seconds=STALE_RUNNING_THRESHOLD_SECONDS,
        )
    return stale_count
```

Call this helper in `list_evaluations` and `get_evaluation` before returning.

- [x] **Step 5: Run stale-running tests**

Run:

```bash
PYTHONPATH=services:services/eval pytest services/eval/tests/test_db.py services/eval/tests/test_main.py -q
```

Expected: PASS.

- [x] **Step 6: Commit**

Run:

```bash
git add services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_db.py services/eval/tests/test_main.py
git commit -m "feat: surface stale eval runs"
```

## Task 5: Add Eval MCP Run Evidence

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`

- [x] **Step 1: Write failing service test**

In `go/eval-mcp-service/internal/evalworkflow/service_test.go`, add:

```go
func TestGetRunEvidenceSummarizesRunningRun(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339)
	rerank := true
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:        "eval-1",
			Status:    "running",
			DatasetID: "dataset-1",
			CreatedAt: createdAt,
			Config: map[string]any{
				"requested_rerank": rerank,
			},
		}},
	}}
	svc := newTestService(api)

	got, err := svc.GetRunEvidence(ctx, "eval-1", 30*time.Minute)
	if err != nil {
		t.Fatalf("GetRunEvidence error: %v", err)
	}
	if got.EvalID != "eval-1" || got.Status != "running" || !got.StaleRunning {
		t.Fatalf("evidence = %#v", got)
	}
	if got.RequestedRerank == nil || !*got.RequestedRerank {
		t.Fatalf("RequestedRerank = %#v", got.RequestedRerank)
	}
}
```

- [x] **Step 2: Run the failing Go test**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow -run TestGetRunEvidenceSummarizesRunningRun -count=1
```

Expected: FAIL because `GetRunEvidence` does not exist.

- [x] **Step 3: Implement evidence types and method**

In `service.go`, add:

```go
type RunEvidence struct {
	EvalID             string         `json:"eval_id"`
	Status             string         `json:"status"`
	DatasetID          string         `json:"dataset_id"`
	Collection         *string        `json:"collection,omitempty"`
	BaselineEvalID     *string        `json:"baseline_eval_id,omitempty"`
	RequestedRerank    *bool          `json:"requested_rerank,omitempty"`
	CreatedAt          string         `json:"created_at"`
	CompletedAt        *string        `json:"completed_at,omitempty"`
	AgeSeconds         int64          `json:"age_seconds"`
	StaleRunning       bool           `json:"stale_running"`
	StaleAfterSeconds  int64          `json:"stale_after_seconds"`
	Error              *string        `json:"error,omitempty"`
	AggregateScores    *evalapi.Scores `json:"aggregate_scores,omitempty"`
	ConfigCaptured     bool           `json:"config_captured"`
	ResultCount        int            `json:"result_count"`
	ScoredResultCount  int            `json:"scored_result_count"`
	NextSteps          []string       `json:"next_steps"`
}
```

Add `GetRunEvidence(ctx, evalID string, staleAfter time.Duration)` that calls
`s.api.GetEvaluation`, parses `CreatedAt`, calculates age/stale state, extracts
`requested_rerank` from `run.Config`, counts results with non-empty scores, and
returns next steps:

```go
[]string{
	"Use observability MCP investigate_eval_run with this eval_id for logs and metrics.",
	"Use get_eval_run again after upstream failures are fixed to inspect terminal state.",
}
```

- [x] **Step 4: Add MCP tool**

In `go/eval-mcp-service/internal/mcpserver/server.go`, extend `EvalService`
with:

```go
GetRunEvidence(context.Context, string, time.Duration) (evalworkflow.RunEvidence, error)
```

Register `get_eval_run_evidence` with input schema:

```json
{"type":"object","properties":{"eval_id":{"type":"string"},"stale_after":{"type":"string"}},"required":["eval_id"],"additionalProperties":false}
```

The handler should parse `stale_after` with `time.ParseDuration`, default to
`30m`, and call `service.GetRunEvidence`.

- [x] **Step 5: Update wait timeout guidance**

Change `WaitResult` to include optional `EvidenceHint string` or `NextSteps []string`.
On timeout, populate guidance with `get_eval_run_evidence`.

- [x] **Step 6: Run eval MCP tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: PASS.

- [x] **Step 7: Commit**

Run:

```bash
git add go/eval-mcp-service
git commit -m "feat: add eval run evidence MCP tool"
```

## Task 6: Add Observability MCP Eval Investigation

**Files:**
- Modify: `go/observability-mcp-service/internal/workflows/catalog.go`
- Modify: `go/observability-mcp-service/internal/workflows/service.go`
- Modify: `go/observability-mcp-service/internal/workflows/service_test.go`
- Modify: `go/observability-mcp-service/internal/mcpserver/server.go`
- Modify: `go/observability-mcp-service/internal/mcpserver/server_test.go`

- [x] **Step 1: Write failing allowlist and workflow tests**

In `go/observability-mcp-service/internal/workflows/service_test.go`, add:

```go
func TestEvalIsAllowlisted(t *testing.T) {
	if !AllowedService("eval") {
		t.Fatal("eval should be allowlisted")
	}
}

func TestInvestigateEvalRunFiltersLogsByEvalID(t *testing.T) {
	prom := &fakePrometheus{}
	loki := &fakeLoki{}
	svc := NewService(prom, loki, nil, 20)

	bundle := svc.InvestigateEvalRun(context.Background(), "eval-123", 15*time.Minute)

	if bundle.Tool != "investigate_eval_run" {
		t.Fatalf("Tool = %q", bundle.Tool)
	}
	if loki.lastQuery.Service != "eval" || loki.lastQuery.Pattern != "eval-123" {
		t.Fatalf("log query = %#v", loki.lastQuery)
	}
}
```

Adapt fake names to the existing test fakes.

- [x] **Step 2: Run failing observability workflow tests**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/workflows -run 'TestEvalIsAllowlisted|TestInvestigateEvalRunFiltersLogsByEvalID' -count=1
```

Expected: FAIL because eval is not allowlisted and the workflow does not exist.

- [x] **Step 3: Add eval allowlist and metric queries**

In `catalog.go`, add:

```go
"eval": {},
```

Add:

```go
evalRunQueries = []querySpec{
	{Name: "eval_run_duration_p95", Query: `histogram_quantile(0.95, sum by (le, status, requested_rerank) (rate(eval_run_duration_seconds_bucket[5m])))`, Unit: "seconds"},
	{Name: "eval_item_duration_p95", Query: `histogram_quantile(0.95, sum by (le, stage, requested_rerank) (rate(eval_item_duration_seconds_bucket[5m])))`, Unit: "seconds"},
	{Name: "eval_upstream_latency_p95", Query: `histogram_quantile(0.95, sum by (le, service, operation, outcome) (rate(eval_upstream_request_duration_seconds_bucket[5m])))`, Unit: "seconds"},
	{Name: "eval_upstream_failures", Query: `sum by (service, operation, reason) (increase(eval_upstream_failures_total[15m]))`, Unit: "failures"},
	{Name: "eval_stale_running", Query: `sum(eval_stale_running_runs)`, Unit: "runs"},
	{Name: "chat_rerank_latency_p95", Query: `histogram_quantile(0.95, sum by (le, model, outcome) (rate(rag_rerank_duration_seconds_bucket[5m])))`, Unit: "seconds"},
	{Name: "chat_rerank_fallbacks", Query: `sum by (reason) (increase(rag_rerank_fallbacks_total[15m]))`, Unit: "fallbacks"},
}
```

- [x] **Step 4: Implement `InvestigateEvalRun`**

In `service.go`, add:

```go
func (s *Service) InvestigateEvalRun(ctx context.Context, evalID string, window time.Duration) EvidenceBundle {
	b := s.bundle("investigate_eval_run", window)
	if evalID == "" {
		b.AddError("input", "validate_eval_id", "eval_id is required")
		s.finalize(&b)
		return b
	}
	s.addPrometheusSignals(ctx, &b, evalRunQueries)
	s.addLogs(ctx, &b, []string{"eval"}, evalID)
	s.addLogs(ctx, &b, []string{"chat"}, evalID)
	s.finalize(&b)
	return b
}
```

Update `InvestigateAIPipeline` to include eval queries and eval logs:

```go
s.addPrometheusSignals(ctx, &b, evalRunQueries)
s.addLogs(ctx, &b, []string{"go-ai-service", "chat", "ingestion", "debug", "eval"}, "")
```

- [x] **Step 5: Add MCP tool wiring**

In `mcpserver/server.go`, extend `WorkflowService` with:

```go
InvestigateEvalRun(context.Context, string, time.Duration) workflows.EvidenceBundle
```

Register tool:

```go
addTool(srv, "investigate_eval_run", "Build eval run logs, metrics, and upstream evidence.", evalRunSchema(), investigateEvalRunHandler(cfg, service))
```

Add schema:

```go
func evalRunSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"window":{"type":"string"},"eval_id":{"type":"string"}},"required":["eval_id"],"additionalProperties":false}`)
}
```

Add handler that parses `window` with `cfg.WindowOrDefault` and calls
`service.InvestigateEvalRun`.

- [x] **Step 6: Run observability MCP tests**

Run:

```bash
cd go/observability-mcp-service
go test ./...
```

Expected: PASS.

- [x] **Step 7: Commit**

Run:

```bash
git add go/observability-mcp-service
git commit -m "feat: add eval observability MCP evidence"
```

## Task 7: Fix Local Prometheus Scraping

**Files:**
- Modify: `monitoring/prometheus.yml`
- Test: add or modify an existing monitoring config test under `tests/`

- [x] **Step 1: Write failing config test**

Create or update `tests/test_local_prometheus_config.py`:

```python
from pathlib import Path

import yaml


def test_local_prometheus_scrapes_python_service_metrics():
    config = yaml.safe_load(Path("monitoring/prometheus.yml").read_text())
    jobs = {job["job_name"]: job for job in config["scrape_configs"]}

    for name in ("ingestion", "chat", "debug", "eval"):
        assert jobs[name]["metrics_path"] == "/metrics"
```

- [x] **Step 2: Run the failing config test**

Run:

```bash
pytest tests/test_local_prometheus_config.py -q
```

Expected: FAIL because local config does not include all services on `/metrics`.

- [x] **Step 3: Update local Prometheus config**

In `monitoring/prometheus.yml`, make these jobs use `/metrics`:

```yaml
  - job_name: "ingestion"
    metrics_path: /metrics
    static_configs:
      - targets: ["ingestion:8000"]

  - job_name: "chat"
    metrics_path: /metrics
    static_configs:
      - targets: ["chat:8000"]

  - job_name: "debug"
    metrics_path: /metrics
    static_configs:
      - targets: ["debug:8000"]

  - job_name: "eval"
    metrics_path: /metrics
    static_configs:
      - targets: ["eval:8000"]
```

- [x] **Step 4: Run the config test**

Run:

```bash
pytest tests/test_local_prometheus_config.py -q
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add monitoring/prometheus.yml tests/test_local_prometheus_config.py
git commit -m "fix: scrape local AI service metrics"
```

## Task 8: Document Local Evidence Workflow

**Files:**
- Modify: `go/eval-mcp-service/README.md`
- Modify: `go/observability-mcp-service/README.md`
- Test: none beyond markdown review

- [x] **Step 1: Update eval MCP README**

Add a short section to `go/eval-mcp-service/README.md`:

```markdown
## Eval Run Evidence

Use `get_eval_run_evidence` when a run is slow, failed, or timed out in
`wait_for_eval_run`. It returns durable eval API state: status, age,
stale-running determination, config capture status, rerank request state,
aggregate scores, error, and compact result counts.

For logs and metrics, call the observability MCP `investigate_eval_run` tool
with the same `eval_id`.
```

- [x] **Step 2: Update observability MCP README**

Add a local fallback note to `go/observability-mcp-service/README.md`:

```markdown
## Eval Run Investigation

Use `investigate_eval_run` with an `eval_id` to collect bounded local evidence
for one RAG eval run. The tool queries eval logs, eval metrics, and chat
rerank signals over the requested window.

Preferred local access is Grafana gateway mode. Direct `localhost` Prometheus
and Loki endpoints only work when the local monitoring stack is running. If a
source is unreachable, the tool returns partial evidence with an explicit
source error instead of hiding the failure.
```

- [x] **Step 3: Review markdown diff**

Run:

```bash
git diff -- go/eval-mcp-service/README.md go/observability-mcp-service/README.md
```

Expected: README changes describe local evidence workflow and do not mention
production dashboards or alerts.

- [x] **Step 4: Commit**

Run:

```bash
git add go/eval-mcp-service/README.md go/observability-mcp-service/README.md
git commit -m "docs: document eval evidence workflow"
```

## Task 9: Final Verification

**Files:**
- All files changed by previous tasks.

- [x] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: PASS.

- [x] **Step 2: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: PASS.

- [x] **Step 3: Run targeted Go tests**

Run:

```bash
(cd go/eval-mcp-service && go test ./...)
(cd go/observability-mcp-service && go test ./...)
```

Expected: PASS.

- [x] **Step 4: Run local smoke when services are available**

Run only when local Docker Compose services and MCP env vars are configured:

```bash
docker compose up -d prometheus grafana qdrant chat ingestion eval
```

Then use MCP tools:

```text
start_eval_run
wait_for_eval_run
get_eval_run_evidence
investigate_eval_run
```

Expected: `get_eval_run_evidence` returns durable run state, and
`investigate_eval_run` returns either logs/metrics or explicit partial source
errors.

- [x] **Step 5: Check final diff**

Run:

```bash
git status --short
git diff --stat main...HEAD
```

Expected: only scoped eval observability, MCP, local monitoring, tests, and
README files changed.

- [x] **Step 6: Push branch and open PR to `qa`**

Run:

```bash
git push -u origin eval-observability-rag-evidence
gh pr create --base qa --head eval-observability-rag-evidence --title "Improve local RAG eval observability" --body "Adds local agent-facing eval run observability through structured eval telemetry, eval MCP evidence, observability MCP eval investigation, and local Prometheus scrape fixes."
```

Expected: PR created against `qa`. Do not watch CI unless explicitly asked.

## Plan Self-Review

- Spec coverage: lifecycle logs, correlation fields, eval/chat metrics, eval
  MCP evidence, observability MCP allowlist/tooling, local Prometheus access,
  and tests are all covered by tasks.
- Scope: local-only; no production dashboards, alerts, rate-limit policy, or
  rerank reliability changes.
- Type consistency: MCP tools use `eval_id`, `stale_after`, and existing
  evidence bundle patterns. Prometheus labels remain bounded and exclude raw
  query/error/eval IDs.
