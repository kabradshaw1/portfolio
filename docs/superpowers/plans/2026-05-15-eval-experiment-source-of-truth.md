# Eval Experiment Source Of Truth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move durable experiment recording into `services/eval` and make `go/eval-mcp-service` operate on those backend experiment records instead of local experiment SQLite.

**Architecture:** `services/eval` owns experiment state, evidence, conclusions, and labeled run attachments. `go/eval-mcp-service` becomes a typed client and workflow adapter around the eval API, using string experiment IDs returned by Python. Frontend work is intentionally excluded because issue 240 owns `/ai/eval` UI changes.

**Tech Stack:** Python 3. FastAPI, Pydantic, aiosqlite, pytest; Go, stdlib `net/http`, MCP Go SDK, `go test`.

---

## File Structure

- Modify `services/eval/app/models.py`: add experiment focus metric and evidence fields to request/response models.
- Modify `services/eval/app/db.py`: add idempotent migrations, JSON persistence, and update/create signatures for new experiment fields.
- Modify `services/eval/app/main.py`: wire new fields through create/update endpoints and enforce completed-experiment requirements.
- Modify `services/eval/tests/test_models.py`: model validation coverage.
- Modify `services/eval/tests/test_db.py`: persistence and migration coverage.
- Modify `services/eval/tests/test_main.py`: endpoint behavior coverage.
- Modify `go/eval-mcp-service/internal/evalapi/client.go`: add experiment API types and methods; extend start-evaluation request with experiment attachment fields.
- Modify `go/eval-mcp-service/internal/evalapi/client_test.go`: HTTP contract coverage.
- Modify `go/eval-mcp-service/internal/evalworkflow/service.go`: replace local store dependency with eval API experiment methods and string experiment IDs.
- Modify `go/eval-mcp-service/internal/evalworkflow/service_test.go`: workflow tests with fake API.
- Modify `go/eval-mcp-service/internal/mcpserver/server.go`: switch experiment ID schemas to strings and pass decision/evidence on conclusion.
- Modify `go/eval-mcp-service/internal/mcpserver/server_test.go`: MCP handler tests.
- Modify `go/eval-mcp-service/cmd/eval-mcp/main.go`: stop opening/migrating local experiment store.
- Modify `go/eval-mcp-service/internal/config/config.go` and `config_test.go`: remove `EVAL_MCP_DB_PATH`.
- Delete `go/eval-mcp-service/internal/store/sqlite.go` and `sqlite_test.go` after workflow and server tests no longer import them.
- Modify `go/eval-mcp-service/README.md`: document eval API source of truth and remove DB path configuration.

## Task 1: Python Models For Experiment Evidence

**Files:**
- Modify: `services/eval/app/models.py`
- Modify: `services/eval/tests/test_models.py`

- [ ] **Step 1: Write failing model tests**

Append these tests to `services/eval/tests/test_models.py`:

```python
def test_create_experiment_request_accepts_focus_metric():
    req = CreateExperimentRequest(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        focus_metric="context_precision",
    )

    assert req.focus_metric == "context_precision"


def test_create_experiment_request_rejects_unknown_focus_metric():
    with pytest.raises(ValidationError):
        CreateExperimentRequest(
            name="precision tuning",
            hypothesis="Reranking improves context precision",
            dataset_id="ds-1",
            collection="documents",
            focus_metric="accuracy",
        )


def test_update_experiment_request_accepts_conclusion_and_evidence():
    evidence = {
        "baseline_eval_id": "eval-base",
        "candidate_eval_ids": ["eval-candidate"],
        "focus_metric": "context_precision",
        "metric_deltas": {
            "candidate": {
                "faithfulness": 0.0,
                "answer_relevancy": 0.01,
                "context_precision": 0.08,
                "context_recall": -0.02,
            }
        },
        "worst_cases": [
            {
                "label": "candidate",
                "eval_id": "eval-candidate",
                "query": "What is chunking?",
                "metric": "context_precision",
                "score": 0.25,
                "reason": "retrieved context missed expected source",
            }
        ],
        "config_diffs": [{"label": "candidate", "summary": "rerank enabled"}],
        "caveats": ["small dataset size"],
    }

    req = UpdateExperimentRequest(
        status="completed",
        decision="keep",
        conclusion="Keep reranking because context precision improved.",
        evidence=evidence,
    )

    assert req.conclusion == "Keep reranking because context precision improved."
    assert req.evidence == evidence


def test_experiment_detail_includes_focus_conclusion_and_evidence():
    detail = ExperimentDetail(
        id="exp-1",
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-1",
        collection="documents",
        baseline_eval_id="eval-base",
        focus_metric="context_precision",
        status="completed",
        decision="keep",
        conclusion="Keep reranking.",
        evidence={"baseline_eval_id": "eval-base", "candidate_eval_ids": ["eval-candidate"]},
        notes="final",
        created_at="2026-05-13T10:00:00+00:00",
        updated_at="2026-05-13T10:00:00+00:00",
        runs=[],
    )

    assert detail.focus_metric == "context_precision"
    assert detail.conclusion == "Keep reranking."
    assert detail.evidence["candidate_eval_ids"] == ["eval-candidate"]
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd services/eval
pytest tests/test_models.py -q
```

Expected: failures mentioning missing `focus_metric`, `conclusion`, or `evidence` fields.

- [ ] **Step 3: Implement model fields**

In `services/eval/app/models.py`, add these aliases near the existing experiment literals:

```python
FocusMetric = Literal[
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
]
ExperimentEvidence = dict[str, Any]
```

Update experiment models:

```python
class CreateExperimentRequest(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    hypothesis: str = Field(min_length=1, max_length=1000)
    dataset_id: str
    collection: str = Field(pattern=r"^[a-zA-Z0-9_-]{1,100}$")
    baseline_eval_id: str | None = None
    focus_metric: FocusMetric = "context_precision"
    status: InitialExperimentStatus = "planned"
    notes: str | None = Field(default=None, max_length=2000)
```

```python
class UpdateExperimentRequest(BaseModel):
    hypothesis: str | None = Field(default=None, min_length=1, max_length=1000)
    baseline_eval_id: str | None = None
    focus_metric: FocusMetric | None = None
    status: ExperimentStatus | None = None
    decision: ExperimentDecision | None = None
    conclusion: str | None = Field(default=None, max_length=5000)
    evidence: ExperimentEvidence | None = None
    notes: str | None = Field(default=None, max_length=2000)
```

```python
class ExperimentSummary(BaseModel):
    id: str
    name: str
    hypothesis: str
    dataset_id: str
    collection: str
    baseline_eval_id: str | None = None
    focus_metric: FocusMetric
    status: ExperimentStatus
    decision: ExperimentDecision | None = None
    conclusion: str | None = None
    evidence: ExperimentEvidence | None = None
    notes: str | None = None
    created_at: str
    updated_at: str
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
cd services/eval
pytest tests/test_models.py -q
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/models.py services/eval/tests/test_models.py
git commit -m "feat(eval): add experiment evidence models"
```

## Task 2: Python DB Persistence For Evidence Fields

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/tests/test_db.py`

- [ ] **Step 1: Write failing DB tests**

Append these tests to `services/eval/tests/test_db.py`:

```python
@pytest.mark.asyncio
async def test_experiment_persists_focus_conclusion_and_evidence(db):
    ds_id = await db.create_dataset(name="ds-evidence", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        focus_metric="context_precision",
        status="running",
        notes="first pass",
    )

    evidence = {
        "baseline_eval_id": "eval-base",
        "candidate_eval_ids": ["eval-candidate"],
        "focus_metric": "context_precision",
        "metric_deltas": {"candidate": {"context_precision": 0.08}},
        "worst_cases": [{"label": "candidate", "query": "q", "score": 0.25}],
        "config_diffs": [{"label": "candidate", "summary": "rerank enabled"}],
        "caveats": ["small dataset size"],
    }
    await db.update_experiment(
        exp_id,
        focus_metric="context_precision",
        status="completed",
        decision="keep",
        conclusion="Keep reranking.",
        evidence=evidence,
        notes="final",
    )

    detail = await db.get_experiment(exp_id)
    assert detail["focus_metric"] == "context_precision"
    assert detail["decision"] == "keep"
    assert detail["conclusion"] == "Keep reranking."
    assert detail["evidence"] == evidence
    assert detail["notes"] == "final"


@pytest.mark.asyncio
async def test_init_is_idempotent_after_experiment_evidence_columns_exist(tmp_path):
    db_path = str(tmp_path / "experiment-evidence.db")

    db1 = EvalDB(db_path)
    await db1.init()
    await db1.close()

    db2 = EvalDB(db_path)
    await db2.init()
    await db2.close()
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd services/eval
pytest tests/test_db.py -q
```

Expected: failures because `create_experiment` and `update_experiment` do not accept the new keyword arguments.

- [ ] **Step 3: Implement DB migrations and JSON round trip**

In `services/eval/app/db.py`, add these columns to the `CREATE TABLE IF NOT EXISTS experiments` statement:

```sql
                focus_metric TEXT NOT NULL DEFAULT 'context_precision',
                conclusion TEXT,
                evidence TEXT,
```

Add idempotent migration entries in the existing `for column_ddl in (...)` loop:

```python
            "ALTER TABLE experiments "
            "ADD COLUMN focus_metric TEXT NOT NULL DEFAULT 'context_precision'",
            "ALTER TABLE experiments ADD COLUMN conclusion TEXT",
            "ALTER TABLE experiments ADD COLUMN evidence TEXT",
```

Update `_experiment_row_to_dict`:

```python
            "focus_metric": row["focus_metric"],
            "conclusion": row["conclusion"],
            "evidence": json.loads(row["evidence"]) if row["evidence"] else None,
```

Update `create_experiment` signature and insert:

```python
        focus_metric: str = "context_precision",
```

Include `focus_metric` in the insert column list and values tuple:

```python
            "(id, name, hypothesis, dataset_id, collection, baseline_eval_id, "
            "focus_metric, status, decision, notes, created_at, updated_at) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)",
```

```python
                baseline_eval_id,
                focus_metric,
                status,
```

Update `update_experiment` signature:

```python
        focus_metric: str | None = None,
        conclusion: str | None = None,
        evidence: dict | None = None,
```

Update the SQL and parameters:

```python
            "focus_metric = COALESCE(?, focus_metric), "
            "status = COALESCE(?, status), "
            "decision = ?, "
            "conclusion = ?, "
            "evidence = ?, "
```

```python
                focus_metric,
                status,
                decision,
                conclusion,
                json.dumps(evidence) if evidence is not None else None,
```

- [ ] **Step 4: Run DB tests**

Run:

```bash
cd services/eval
pytest tests/test_db.py -q
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): persist experiment evidence"
```

## Task 3: Python API Contract And Validation

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Write failing API tests**

Append these tests to `services/eval/tests/test_main.py`:

```python
@patch("app.main.get_db")
def test_create_experiment_persists_focus_metric(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_dataset.return_value = {
        "id": "ds-123",
        "name": "test",
        "items": [{"query": "q", "expected_answer": "a", "expected_sources": []}],
        "created_at": "2026-04-16T00:00:00Z",
    }
    mock_db.create_experiment.return_value = "exp-1"
    mock_db.get_experiment.return_value = {
        "id": "exp-1",
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-123",
        "collection": "documents",
        "baseline_eval_id": None,
        "focus_metric": "context_precision",
        "status": "running",
        "decision": None,
        "conclusion": None,
        "evidence": None,
        "notes": None,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z",
        "runs": [],
    }
    mock_get_db.return_value = mock_db

    response = client.post(
        "/experiments",
        json={
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "focus_metric": "context_precision",
            "status": "running",
        },
    )

    assert response.status_code == 201
    mock_db.create_experiment.assert_awaited_once_with(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id="ds-123",
        collection="documents",
        baseline_eval_id=None,
        focus_metric="context_precision",
        status="running",
        notes=None,
    )
    assert response.json()["focus_metric"] == "context_precision"


@patch("app.main.get_db")
def test_update_experiment_can_complete_with_evidence(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.side_effect = [
        {
            "id": "exp-1",
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "baseline_eval_id": None,
            "focus_metric": "context_precision",
            "status": "running",
            "decision": None,
            "conclusion": None,
            "evidence": None,
            "notes": None,
            "created_at": "2026-05-15T00:00:00Z",
            "updated_at": "2026-05-15T00:00:00Z",
            "runs": [],
        },
        {
            "id": "exp-1",
            "name": "precision tuning",
            "hypothesis": "Reranking improves context precision",
            "dataset_id": "ds-123",
            "collection": "documents",
            "baseline_eval_id": None,
            "focus_metric": "context_precision",
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
            "evidence": {"baseline_eval_id": "eval-base", "candidate_eval_ids": ["eval-candidate"]},
            "notes": None,
            "created_at": "2026-05-15T00:00:00Z",
            "updated_at": "2026-05-15T00:05:00Z",
            "runs": [],
        },
    ]
    mock_get_db.return_value = mock_db

    evidence = {"baseline_eval_id": "eval-base", "candidate_eval_ids": ["eval-candidate"]}
    response = client.patch(
        "/experiments/exp-1",
        json={
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
            "evidence": evidence,
        },
    )

    assert response.status_code == 200
    mock_db.update_experiment.assert_awaited_once_with(
        "exp-1",
        hypothesis=None,
        baseline_eval_id=None,
        focus_metric=None,
        status="completed",
        decision="keep",
        conclusion="Keep reranking.",
        evidence=evidence,
        notes=None,
    )
    assert response.json()["evidence"] == evidence


@patch("app.main.get_db")
def test_update_experiment_rejects_completed_without_decision_conclusion_or_evidence(mock_get_db):
    mock_db = AsyncMock()
    mock_db.get_experiment.return_value = {
        "id": "exp-1",
        "name": "precision tuning",
        "hypothesis": "Reranking improves context precision",
        "dataset_id": "ds-123",
        "collection": "documents",
        "baseline_eval_id": None,
        "focus_metric": "context_precision",
        "status": "running",
        "decision": None,
        "conclusion": None,
        "evidence": None,
        "notes": None,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z",
        "runs": [],
    }
    mock_get_db.return_value = mock_db

    response = client.patch("/experiments/exp-1", json={"status": "completed"})

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require a decision"
    mock_db.update_experiment.assert_not_awaited()

    response = client.patch(
        "/experiments/exp-1",
        json={"status": "completed", "decision": "keep"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require a conclusion"
    mock_db.update_experiment.assert_not_awaited()

    response = client.patch(
        "/experiments/exp-1",
        json={
            "status": "completed",
            "decision": "keep",
            "conclusion": "Keep reranking.",
        },
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "completed experiments require evidence"
    mock_db.update_experiment.assert_not_awaited()
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd services/eval
pytest tests/test_main.py -q
```

Expected: failures because the endpoint does not pass new fields to the DB and does not enforce completion decision.

- [ ] **Step 3: Wire fields and completion validation**

In `services/eval/app/main.py`, update `create_experiment` call:

```python
        focus_metric=body.focus_metric,
```

In `update_experiment`, add completion validation after `final_status` is computed:

```python
    final_decision = body.decision if body.decision is not None else experiment["decision"]
    final_conclusion = (
        body.conclusion if body.conclusion is not None else experiment["conclusion"]
    )
    final_evidence = body.evidence if body.evidence is not None else experiment["evidence"]
    if final_status == "completed" and final_decision is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require a decision"
        )
    if final_status == "completed" and final_conclusion is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require a conclusion"
        )
    if final_status == "completed" and final_evidence is None:
        raise HTTPException(
            status_code=400, detail="completed experiments require evidence"
        )
```

Keep the existing decision/status check, then pass new fields:

```python
        focus_metric=body.focus_metric,
        conclusion=body.conclusion,
        evidence=body.evidence,
```

- [ ] **Step 4: Run API tests**

Run:

```bash
cd services/eval
pytest tests/test_main.py -q
```

Expected: all tests pass.

- [ ] **Step 5: Run focused Python eval tests**

Run:

```bash
cd services/eval
pytest tests/test_models.py tests/test_db.py tests/test_main.py -q
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): expose experiment evidence API"
```

## Task 4: Go Eval API Client For Backend Experiments

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalapi/client_test.go`

- [ ] **Step 1: Write failing client tests**

Append this test to `go/eval-mcp-service/internal/evalapi/client_test.go`:

```go
func TestExperimentAPIMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/experiments":
			switch r.Method {
			case http.MethodPost:
				var body CreateExperimentRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode create experiment: %v", err)
				}
				if body.Name != "precision tuning" || body.FocusMetric != "context_precision" {
					t.Fatalf("create body = %#v", body)
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: body.Name, DatasetID: body.DatasetID, Collection: body.Collection, FocusMetric: body.FocusMetric, Status: "running"})
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{"experiments": []Experiment{{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"}}})
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		case "/experiments/exp-1":
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
			case http.MethodPatch:
				var body UpdateExperimentRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode update experiment: %v", err)
				}
				if body.Status != "completed" || body.Decision != "keep" || body.Conclusion != "Keep reranking." {
					t.Fatalf("update body = %#v", body)
				}
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "completed", Decision: "keep", Conclusion: "Keep reranking.", Evidence: body.Evidence})
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		case "/experiments/exp-1/runs":
			var body AttachExperimentRunRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode attach run: %v", err)
			}
			if body.EvaluationID != "eval-candidate" || body.Label != "candidate" {
				t.Fatalf("attach body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	ctx := context.Background()
	exp, err := client.CreateExperiment(ctx, CreateExperimentRequest{Name: "precision tuning", Hypothesis: "rerank improves precision", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	if exp.ID != "exp-1" {
		t.Fatalf("created experiment = %#v", exp)
	}
	experiments, err := client.ListExperiments(ctx)
	if err != nil || len(experiments) != 1 {
		t.Fatalf("ListExperiments = %#v, %v", experiments, err)
	}
	got, err := client.GetExperiment(ctx, "exp-1")
	if err != nil || got.FocusMetric != "context_precision" {
		t.Fatalf("GetExperiment = %#v, %v", got, err)
	}
	if _, err := client.AttachExperimentRun(ctx, "exp-1", AttachExperimentRunRequest{EvaluationID: "eval-candidate", Label: "candidate"}); err != nil {
		t.Fatalf("AttachExperimentRun error: %v", err)
	}
	updated, err := client.UpdateExperiment(ctx, "exp-1", UpdateExperimentRequest{Status: "completed", Decision: "keep", Conclusion: "Keep reranking.", Evidence: map[string]any{"baseline_eval_id": "eval-base"}})
	if err != nil {
		t.Fatalf("UpdateExperiment error: %v", err)
	}
	if updated.Status != "completed" || updated.Evidence["baseline_eval_id"] != "eval-base" {
		t.Fatalf("updated experiment = %#v", updated)
	}
}
```

Update `TestStartEvaluationSendsOptionalFields` to set and assert:

```go
ExperimentID:    "exp-1",
ExperimentLabel: "candidate",
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi
```

Expected: compile failures for missing experiment API types and fields.

- [ ] **Step 3: Implement eval API types and methods**

In `go/eval-mcp-service/internal/evalapi/client.go`, extend `StartEvaluationRequest`:

```go
	ExperimentID    string `json:"experiment_id,omitempty"`
	ExperimentLabel string `json:"experiment_label,omitempty"`
```

Add types:

```go
type Experiment struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Hypothesis     string         `json:"hypothesis"`
	DatasetID      string         `json:"dataset_id"`
	Collection     string         `json:"collection"`
	BaselineEvalID *string        `json:"baseline_eval_id"`
	FocusMetric    string         `json:"focus_metric"`
	Status         string         `json:"status"`
	Decision       *string        `json:"decision"`
	Conclusion     *string        `json:"conclusion"`
	Evidence       map[string]any `json:"evidence"`
	Notes          *string        `json:"notes"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	Runs           []ExperimentRun `json:"runs,omitempty"`
}

type ExperimentRun struct {
	EvaluationID string           `json:"evaluation_id"`
	Label        string           `json:"label"`
	Notes        *string          `json:"notes"`
	AttachedAt   string           `json:"attached_at"`
	Evaluation   EvaluationDetail `json:"evaluation"`
}

type CreateExperimentRequest struct {
	Name           string `json:"name"`
	Hypothesis     string `json:"hypothesis"`
	DatasetID      string `json:"dataset_id"`
	Collection     string `json:"collection"`
	BaselineEvalID string `json:"baseline_eval_id,omitempty"`
	FocusMetric    string `json:"focus_metric,omitempty"`
	Status         string `json:"status,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type UpdateExperimentRequest struct {
	Hypothesis     string         `json:"hypothesis,omitempty"`
	BaselineEvalID string         `json:"baseline_eval_id,omitempty"`
	FocusMetric    string         `json:"focus_metric,omitempty"`
	Status         string         `json:"status,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	Conclusion     string         `json:"conclusion,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

type AttachExperimentRunRequest struct {
	EvaluationID string `json:"evaluation_id"`
	Label        string `json:"label"`
	Notes        string `json:"notes,omitempty"`
}
```

Add methods:

```go
func (c *Client) CreateExperiment(ctx context.Context, body CreateExperimentRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPost, "/experiments", body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) ListExperiments(ctx context.Context) ([]Experiment, error) {
	var response struct {
		Experiments []Experiment `json:"experiments"`
	}
	if err := c.do(ctx, http.MethodGet, "/experiments", nil, &response); err != nil {
		return nil, err
	}
	return response.Experiments, nil
}

func (c *Client) GetExperiment(ctx context.Context, id string) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodGet, "/experiments/"+url.PathEscape(id), nil, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) UpdateExperiment(ctx context.Context, id string, body UpdateExperimentRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPatch, "/experiments/"+url.PathEscape(id), body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}

func (c *Client) AttachExperimentRun(ctx context.Context, id string, body AttachExperimentRunRequest) (Experiment, error) {
	var response Experiment
	if err := c.do(ctx, http.MethodPost, "/experiments/"+url.PathEscape(id)+"/runs", body, &response); err != nil {
		return Experiment{}, err
	}
	return response, nil
}
```

- [ ] **Step 4: Run evalapi tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalapi/client_test.go
git commit -m "feat(eval-mcp): add experiment API client"
```

## Task 5: Go Workflow Uses Eval API As Experiment Store

**Files:**
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service_test.go`

- [ ] **Step 1: Write failing workflow tests**

Change workflow tests to expect string experiment IDs and API-backed calls. Replace `TestStartRunAttachesReturnedEvalIDWhenExperimentProvided` with:

```go
func TestStartRunSendsExperimentAttachmentToEvalAPI(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-123", Status: "queued"}}
	svc := New(api, time.Millisecond, time.Second)

	got, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:      "dataset-1",
		Collection:     "kb",
		Notes:          "candidate notes",
		ExperimentID:   "exp-7",
		Label:          "candidate",
		BaselineEvalID: "eval-base",
		Rerank:         true,
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if got.EvalID != "eval-123" || got.Status != "queued" {
		t.Fatalf("StartRunResult = %#v", got)
	}
	if len(api.startRequests) != 1 {
		t.Fatalf("StartEvaluation calls = %d, want 1", len(api.startRequests))
	}
	req := api.startRequests[0]
	if req.ExperimentID != "exp-7" || req.ExperimentLabel != "candidate" {
		t.Fatalf("StartEvaluation request = %#v", req)
	}
}
```

Add this conclusion test:

```go
func TestRecordConclusionCompletesExperimentWithEvidence(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	svc := New(api, time.Millisecond, time.Second)
	evidence := map[string]any{"baseline_eval_id": "eval-base"}

	if err := svc.RecordConclusion(ctx, RecordConclusionInput{
		ExperimentID: "exp-1",
		Decision:     "keep",
		Conclusion:   "Keep reranking.",
		Evidence:     evidence,
	}); err != nil {
		t.Fatalf("RecordConclusion error: %v", err)
	}
	if api.updateExperimentID != "exp-1" {
		t.Fatalf("updateExperimentID = %q", api.updateExperimentID)
	}
	if api.updateExperimentRequest.Status != "completed" || api.updateExperimentRequest.Decision != "keep" || api.updateExperimentRequest.Conclusion != "Keep reranking." {
		t.Fatalf("update request = %#v", api.updateExperimentRequest)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow
```

Expected: compile failures because `New`, `StartRunInput`, and `RecordConclusion` still use the local store model.

- [ ] **Step 3: Refactor workflow interfaces and types**

In `service.go`, replace the `API` and `Store` split with this `API` interface:

```go
type API interface {
	ListDatasets(context.Context) ([]evalapi.Dataset, error)
	StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error)
	GetEvaluation(context.Context, string) (evalapi.EvaluationDetail, error)
	CompareEvaluations(context.Context, []string) (evalapi.Comparison, error)
	CreateExperiment(context.Context, evalapi.CreateExperimentRequest) (evalapi.Experiment, error)
	ListExperiments(context.Context) ([]evalapi.Experiment, error)
	GetExperiment(context.Context, string) (evalapi.Experiment, error)
	AttachExperimentRun(context.Context, string, evalapi.AttachExperimentRunRequest) (evalapi.Experiment, error)
	UpdateExperiment(context.Context, string, evalapi.UpdateExperimentRequest) (evalapi.Experiment, error)
}
```

Remove `Store` and change `Service`/constructor:

```go
type Service struct {
	api          API
	pollInterval time.Duration
	waitTimeout  time.Duration
}

func New(api API, pollInterval, waitTimeout time.Duration) *Service {
	return &Service{api: api, pollInterval: pollInterval, waitTimeout: waitTimeout}
}
```

Change experiment IDs to strings:

```go
ExperimentID string
```

Use `evalapi.Experiment` in outputs. Add:

```go
type RecordConclusionInput struct {
	ExperimentID string
	Decision     string
	Conclusion   string
	Evidence     map[string]any
}
```

Implement API-backed methods:

```go
func (s *Service) StartExperiment(ctx context.Context, in StartExperimentInput) (evalapi.Experiment, error) {
	collection := in.Collection
	if collection == "" {
		collection = DefaultCollection
	}
	focusMetric := in.FocusMetric
	if focusMetric == "" {
		focusMetric = DefaultFocusMetric
	}
	if err := validateMetric(focusMetric); err != nil {
		return evalapi.Experiment{}, err
	}
	return s.api.CreateExperiment(ctx, evalapi.CreateExperimentRequest{
		Name:           in.Name,
		Hypothesis:     in.Hypothesis,
		DatasetID:      in.DatasetID,
		Collection:     collection,
		BaselineEvalID: in.BaselineEvalID,
		FocusMetric:    focusMetric,
		Status:         "running",
		Notes:          in.Notes,
	})
}
```

```go
func (s *Service) StartRun(ctx context.Context, in StartRunInput) (StartRunResult, error) {
	resp, err := s.api.StartEvaluation(ctx, evalapi.StartEvaluationRequest{
		DatasetID:        in.DatasetID,
		Collection:       in.Collection,
		Notes:            in.Notes,
		BaselineEvalID:   in.BaselineEvalID,
		Rerank:           in.Rerank,
		ExperimentID:     in.ExperimentID,
		ExperimentLabel:  in.Label,
	})
	if err != nil {
		return StartRunResult{}, err
	}
	return StartRunResult{EvalID: resp.ID, Status: resp.Status}, nil
}
```

```go
func (s *Service) AttachRun(ctx context.Context, experimentID, label, evalID, notes string) error {
	_, err := s.api.AttachExperimentRun(ctx, experimentID, evalapi.AttachExperimentRunRequest{
		EvaluationID: evalID,
		Label:        label,
		Notes:        notes,
	})
	return err
}
```

```go
func (s *Service) RecordConclusion(ctx context.Context, in RecordConclusionInput) error {
	_, err := s.api.UpdateExperiment(ctx, in.ExperimentID, evalapi.UpdateExperimentRequest{
		Status:     "completed",
		Decision:   in.Decision,
		Conclusion: in.Conclusion,
		Evidence:   in.Evidence,
	})
	return err
}
```

Update label resolution to iterate `evalapi.Experiment.Runs` and use `EvaluationID`.

- [ ] **Step 4: Update fake API and run workflow tests**

Update `fakeAPI` in `service_test.go` with experiment fields:

```go
experiments map[string]evalapi.Experiment
createExperimentRequests []evalapi.CreateExperimentRequest
updateExperimentID string
updateExperimentRequest evalapi.UpdateExperimentRequest
```

Add fake methods matching the new `API` interface.

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalworkflow
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/evalworkflow/service_test.go
git commit -m "feat(eval-mcp): use eval API experiment records"
```

## Task 6: MCP Server Uses String Experiment IDs And Evidence

**Files:**
- Modify: `go/eval-mcp-service/internal/mcpserver/server.go`
- Modify: `go/eval-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing MCP tests**

Update fake service signatures in `server_test.go` to use `evalapi.Experiment` and string experiment IDs. Add this test:

```go
func TestRecordConclusionHandlerSendsDecisionAndEvidence(t *testing.T) {
	fake := &fakeEvalService{}
	result, err := recordConclusionHandler(fake)(context.Background(), callReq(map[string]any{
		"experiment_id": "exp-1",
		"decision":      "keep",
		"conclusion":    "Keep reranking.",
		"evidence": map[string]any{
			"baseline_eval_id": "eval-base",
		},
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.recordConclusionInput.ExperimentID != "exp-1" || fake.recordConclusionInput.Decision != "keep" {
		t.Fatalf("record input = %#v", fake.recordConclusionInput)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: compile failures because handler/service interfaces still use integer experiment IDs and conclusion-only payloads.

- [ ] **Step 3: Update MCP service interface and handlers**

In `server.go`, update the service interface:

```go
StartExperiment(context.Context, evalworkflow.StartExperimentInput) (evalapi.Experiment, error)
ListExperiments(context.Context) ([]evalapi.Experiment, error)
GetExperiment(context.Context, string) (evalapi.Experiment, error)
AttachRun(context.Context, string, string, string, string) error
SummarizeExperiment(context.Context, string) (evalworkflow.ExperimentSummary, error)
RecordConclusion(context.Context, evalworkflow.RecordConclusionInput) error
```

Change all request structs from `ExperimentID int64` to:

```go
ExperimentID string `json:"experiment_id"`
```

Update schemas from integer to string:

```go
return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1}},"required":["experiment_id"],"additionalProperties":false}`)
```

Change `recordConclusionSchema`:

```go
return json.RawMessage(`{"type":"object","properties":{"experiment_id":{"type":"string","minLength":1},"decision":{"type":"string","enum":["keep","revert","needs_more_data"]},"conclusion":{"type":"string"},"evidence":{"type":"object"}},"required":["experiment_id","decision","conclusion","evidence"],"additionalProperties":false}`)
```

Update `recordConclusionHandler` to pass:

```go
evalworkflow.RecordConclusionInput{
	ExperimentID: in.ExperimentID,
	Decision:     in.Decision,
	Conclusion:   in.Conclusion,
	Evidence:     in.Evidence,
}
```

- [ ] **Step 4: Run MCP server tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/mcpserver
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add go/eval-mcp-service/internal/mcpserver/server.go go/eval-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat(eval-mcp): expose backend experiment IDs"
```

## Task 7: Remove MCP Experiment SQLite Store

**Files:**
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main.go`
- Modify: `go/eval-mcp-service/cmd/eval-mcp/main_test.go`
- Modify: `go/eval-mcp-service/internal/config/config.go`
- Modify: `go/eval-mcp-service/internal/config/config_test.go`
- Delete: `go/eval-mcp-service/internal/store/sqlite.go`
- Delete: `go/eval-mcp-service/internal/store/sqlite_test.go`
- Modify: `go/eval-mcp-service/README.md`

- [ ] **Step 1: Write failing config/main tests**

In `config_test.go`, change the expected config so `DBPath` is no longer asserted and `EVAL_MCP_DB_PATH` is ignored. In `main_test.go`, assert that `run` no longer creates or migrates a store by compiling without `internal/store`.

Use this test shape in `config_test.go`:

```go
func TestFromEnvIgnoresRemovedDBPath(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "/tmp/old.db")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv error: %v", err)
	}
	if cfg.EvalAPIURL == "" {
		t.Fatalf("EvalAPIURL is empty")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./cmd/eval-mcp ./internal/config ./internal/store
```

Expected: failures until config/main stop importing store; `./internal/store` will be removed by the end of this task.

- [ ] **Step 3: Remove DB config and main store initialization**

In `config.go`, remove:

```go
defaultDBPath
DBPath string
DBPath: getenv("EVAL_MCP_DB_PATH", defaultDBPath),
```

In `cmd/eval-mcp/main.go`, remove the `store` import and this block:

```go
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}
```

Construct the service with:

```go
service := evalworkflow.New(api, cfg.PollInterval, cfg.WaitTimeout)
```

Log without DB path:

```go
logger.Printf("eval MCP server running on stdio eval_api_url=%s", cfg.EvalAPIURL)
```

Delete the store package files after no production or test code imports them:

```bash
rm go/eval-mcp-service/internal/store/sqlite.go go/eval-mcp-service/internal/store/sqlite_test.go
```

- [ ] **Step 4: Update README**

In `go/eval-mcp-service/README.md`, remove `EVAL_MCP_DB_PATH` and replace the ownership paragraph with:

```markdown
The Python eval API is the source of truth for datasets, evaluations,
experiments, labels, decisions, conclusions, and evidence. This MCP service is
a local stdio workflow adapter over that API.
```

- [ ] **Step 5: Run Go package tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add go/eval-mcp-service
git commit -m "refactor(eval-mcp): remove local experiment store"
```

## Task 8: Final Verification

**Files:**
- Verify only; no planned file edits.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: command exits 0.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: command exits 0.

- [ ] **Step 3: Inspect final diff**

Run:

```bash
git status --short
git diff --stat HEAD
```

Expected: no unstaged changes if every task committed; diff stat is empty against `HEAD`.

- [ ] **Step 4: Confirm frontend untouched**

Run:

```bash
git log --name-only --oneline --max-count=8 | rg '^frontend/' || true
```

Expected: no frontend paths from this implementation.
