# RAG Tracking Dashboard — Phase 1 Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the eval-service comparison and history endpoints, capture per-run RAG configuration from chat + ingestion services, and surface tunable knobs (`top_k`, prompt version, per-collection chunk metadata) so every evaluation is self-describing. Closes [#84](https://github.com/kabradshaw1/gen_ai_engineer/issues/84).

**Architecture:** Three Python FastAPI services touched additively. Eval gains 3 columns (`notes`, `config`, `baseline_eval_id`) and 2 endpoints (`/compare`, `/history`); chat gains `/config` plus configurable `top_k` and a prompt registry; ingestion gains per-collection metadata storage in a small SQLite file plus `/collections/{name}/config`. Eval calls chat + ingestion in parallel at run start to snapshot the config. No existing endpoint shape changes.

**Tech Stack:** Python 3.11, FastAPI, Pydantic v2, aiosqlite, httpx, pytest + pytest-asyncio, ruff. Spec: `docs/superpowers/specs/2026-04-28-rag-tracking-dashboard-design.md`.

---

## File Structure

**Eval service (`services/eval/`):**
- Modify: `app/db.py`, `app/models.py`, `app/main.py`, `app/config.py`
- Create: `app/config_capture.py`
- Modify/create tests: `tests/test_db.py`, `tests/test_models.py`, `tests/test_main.py`, `tests/test_config_capture.py`

**Chat service (`services/chat/`):**
- Modify: `app/config.py`, `app/main.py`, `app/chain.py`, `app/prompt.py`
- Modify/create tests: `tests/test_main.py`, `tests/test_chain.py`, `tests/test_prompt.py`

**Ingestion service (`services/ingestion/`):**
- Modify: `app/config.py`, `app/main.py`, `app/store.py`
- Create: `app/collection_meta.py`
- Create tests: `tests/test_collection_meta.py`; modify `tests/test_main.py`

**K8s manifests (`k8s/ai-services/configmaps/`):**
- Modify: `chat-config.yml`, `eval-config.yml`, `ingestion-config.yml`

**Compose-smoke parity (`docker-compose.yml`):** services use `env_file: .env`; defaults make new keys non-blocking, so no compose changes are strictly required. Update `.env.example` if it exists.

---

## Conventions Used Throughout

- All file paths in this plan are repo-relative (pwd is the worktree root).
- Test runner pattern: `cd services/<svc> && python -m pytest tests/<file>::<name> -v` so PYTHONPATH resolves the `app` package the way it's set up in each service.
- Commit format: `<type>(<scope>): <subject>` matching repo style (`feat(eval):`, `feat(chat):`, `feat(ingestion):`, `chore(k8s):`).
- After every TDD pair (test fails → impl → test passes), commit. Small commits are intentional.
- After completing the last task in a service, run that service's full test suite before final commit: `cd services/<svc> && python -m pytest tests/ -v`.
- After all backend tasks, run `make preflight-python` and `make preflight-security` from the repo root before opening the PR.

---

## Task 1: SQLite columns and idempotent migration

**Why first:** Every other task in the eval service depends on these columns existing.

**Files:**
- Modify: `services/eval/app/db.py`
- Modify: `services/eval/tests/test_db.py`

- [ ] **Step 1.1: Write the failing test for column round-trip**

Append three tests to `services/eval/tests/test_db.py`:

1. `test_create_run_with_notes_and_baseline` — calls `create_evaluation(..., notes="bumped overlap to 300", baseline_eval_id=None)`, asserts `get_evaluation` returns those values plus `config is None`.
2. `test_set_config_persists_json` — calls a new `set_evaluation_config(eval_id, {"chat": {"llm_model": "qwen"}})`, asserts the JSON round-trips intact.
3. `test_init_is_idempotent_after_columns_exist` — initialises a DB twice against the same path; the second `init()` must not raise.

Use existing fixtures from `services/eval/tests/conftest.py`. If no `tmp_path`-based DB fixture exists, instantiate `EvalDB(str(tmp_path / "x.db"))` directly.

- [ ] **Step 1.2: Run tests to verify they fail**

```
cd services/eval && python -m pytest tests/test_db.py -v -k "notes or baseline or config or idempotent"
```

Expected: FAIL with `TypeError: create_evaluation() got an unexpected keyword argument 'notes'` and `AttributeError: 'EvalDB' object has no attribute 'set_evaluation_config'`.

- [ ] **Step 1.3: Add the migrations and methods**

In `services/eval/app/db.py`, inside `init()`, after the `executescript(...)` block and before `await self._db.commit()`, add idempotent ALTER TABLE statements wrapped in `try/except aiosqlite.OperationalError` for the three columns: `notes TEXT`, `config TEXT`, `baseline_eval_id TEXT REFERENCES evaluations(id)`. Re-raise if the error message is anything other than "duplicate column name".

Update `create_evaluation` to accept `notes: str | None = None` and `baseline_eval_id: str | None = None`, persisting both in the INSERT.

Add `set_evaluation_config(self, eval_id: str, config: dict) -> None` that does `UPDATE evaluations SET config = ? WHERE id = ?` with `json.dumps(config)`.

Refactor the row→dict conversion into a private `_row_to_dict(self, row) -> dict` method that includes the new fields (`notes`, `config` JSON-decoded, `baseline_eval_id`). Update `get_evaluation` and `list_evaluations` to call `_row_to_dict`.

- [ ] **Step 1.4: Run the tests to verify they pass**

```
cd services/eval && python -m pytest tests/test_db.py -v
```

Expected: all tests pass (existing + new three).

- [ ] **Step 1.5: Commit**

```
git add services/eval/app/db.py services/eval/tests/test_db.py
git commit -m "feat(eval): add notes, config, baseline columns with idempotent migration"
```

---

## Task 2: Pydantic models — extend request and response shapes

**Files:**
- Modify: `services/eval/app/models.py`
- Create: `services/eval/tests/test_models.py`

- [ ] **Step 2.1: Write failing model tests**

Create `services/eval/tests/test_models.py` with:
- `test_start_request_accepts_notes_and_baseline` — `StartEvaluationRequest(dataset_id="x", notes="...", baseline_eval_id="y")` succeeds and round-trips.
- `test_start_request_notes_max_length` — `notes="x" * 501` raises `pydantic.ValidationError`.
- `test_evaluation_detail_includes_new_fields` — building `EvaluationDetail(...)` with `notes`, `config={"chat": {}}`, `baseline_eval_id` succeeds and the values are accessible.
- `test_run_comparison_shape` — `RunComparison(runs=[], deltas={"faithfulness": [0.0], ...})` validates.
- `test_run_history_shape` — `RunHistory(runs=[])` validates.

- [ ] **Step 2.2: Run tests to verify they fail**

```
cd services/eval && python -m pytest tests/test_models.py -v
```

Expected: FAIL with `ImportError: cannot import name 'RunComparison'` (or similar).

- [ ] **Step 2.3: Update models**

In `services/eval/app/models.py`:

- Extend `StartEvaluationRequest` with `notes: str | None = Field(default=None, max_length=500)` and `baseline_eval_id: str | None = None`.
- Extend `EvaluationSummary` and `EvaluationDetail` with `notes: str | None = None`, `config: dict[str, Any] | None = None`, `baseline_eval_id: str | None = None` (import `Any` from `typing`).
- Add `RunComparison(BaseModel)`: `runs: list[EvaluationDetail]`, `deltas: dict[str, list[float]]`.
- Add `RunHistory(BaseModel)`: `runs: list[EvaluationDetail]`.

- [ ] **Step 2.4: Run model tests**

```
cd services/eval && python -m pytest tests/test_models.py -v
```

Expected: all 5 PASS.

- [ ] **Step 2.5: Commit**

```
git add services/eval/app/models.py services/eval/tests/test_models.py
git commit -m "feat(eval): extend models with notes, config, baseline plus comparison/history shapes"
```

---

## Task 3: POST /evaluations accepts notes + baseline_eval_id

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 3.1: Write the failing test**

Append `test_start_run_accepts_notes_and_baseline` to `services/eval/tests/test_main.py`. Use the existing `client` and `auth_headers` fixtures (inspect `tests/conftest.py` first to mirror the pattern). The test should:

1. POST a dataset.
2. POST `/evaluations` with `{"dataset_id": ds_id, "notes": "bumped overlap", "baseline_eval_id": "eval-prev"}`.
3. Assert response 202.
4. GET the evaluation detail and assert `notes == "bumped overlap"` and `baseline_eval_id == "eval-prev"`.

Wrap the test with whatever monkeypatch is needed to no-op `_run_evaluation_task` so the request returns immediately — copy the pattern from any existing test in the file that hits POST `/evaluations`.

- [ ] **Step 3.2: Run test to verify it fails**

```
cd services/eval && python -m pytest tests/test_main.py::test_start_run_accepts_notes_and_baseline -v
```

Expected: FAIL — request body fields not yet read.

- [ ] **Step 3.3: Update the endpoint**

In `services/eval/app/main.py`, modify `start_evaluation` to pass `notes=body.notes` and `baseline_eval_id=body.baseline_eval_id` into `db.create_evaluation(...)`. The function signature gains nothing new because `body: StartEvaluationRequest` already includes the new optional fields after Task 2.

- [ ] **Step 3.4: Run test to verify it passes**

```
cd services/eval && python -m pytest tests/test_main.py::test_start_run_accepts_notes_and_baseline -v
```

Expected: PASS.

- [ ] **Step 3.5: Commit**

```
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): persist notes and baseline_eval_id on POST /evaluations"
```

---

## Task 4: Chat /config endpoint + new settings

**Files:**
- Modify: `services/chat/app/main.py`
- Modify: `services/chat/app/config.py`
- Modify: `services/chat/tests/test_main.py`

- [ ] **Step 4.1: Write the failing test**

Append `test_config_endpoint_returns_active_settings` to `services/chat/tests/test_main.py`. The test should GET `/config`, assert 200, and assert the response body contains `llm_model`, `embedding_model`, `top_k`, `prompt_version` keys with values matching `app.config.settings` accessors.

(If `client` fixture doesn't exist in chat tests yet, copy the pattern from an existing chat test or from the eval service's conftest.)

- [ ] **Step 4.2: Run test to verify it fails**

```
cd services/chat && python -m pytest tests/test_main.py -k config_endpoint -v
```

Expected: FAIL — 404 on `/config` plus `AttributeError` on `settings.top_k` / `settings.prompt_version`.

- [ ] **Step 4.3: Add the settings and endpoint**

In `services/chat/app/config.py`, add to `Settings`:
```
top_k: int = 5
prompt_version: str = "v1-baseline"
```

In `services/chat/app/main.py`, add a `GET /config` route returning the four fields from `settings`.

- [ ] **Step 4.4: Run test to verify it passes**

```
cd services/chat && python -m pytest tests/test_main.py -k config_endpoint -v
```

Expected: PASS.

- [ ] **Step 4.5: Commit**

```
git add services/chat/app/main.py services/chat/app/config.py services/chat/tests/test_main.py
git commit -m "feat(chat): expose /config with top_k and prompt_version"
```

---

## Task 5: Honor settings.top_k at retrieval call sites

**Why separate task:** Task 4 added the *setting*; this task threads it through `chain.py` so changing `TOP_K` actually changes retrieval behavior.

**Files:**
- Modify: `services/chat/app/chain.py`
- Modify: `services/chat/app/main.py` (HTTP entry points)
- Modify: `services/chat/tests/test_chain.py` (or create)

- [ ] **Step 5.1: Inspect call sites**

Read `services/chat/app/chain.py` to find every callsite that invokes `retrieve_chunks(...)` or `chat_with_rag(...)` without an explicit `top_k`. Read `services/chat/app/main.py` for HTTP-layer call sites.

- [ ] **Step 5.2: Write failing test**

Pick the lower-friction test of two:

Option A (HTTP-level, preferred when chain.py is hard to mock): use the FastAPI `client` fixture. Monkeypatch `chat_with_rag` (or `retrieve_chunks`) to capture its `top_k` kwarg. Set `settings.top_k = 9`. POST a chat request. Assert the captured value is 9.

Option B (function-level): import `chain`, monkeypatch the retriever's `search` method to record the `top_k` it received, and call `chain.retrieve_chunks(question="q")` directly with `settings.top_k` mocked.

- [ ] **Step 5.3: Run test to verify it fails**

```
cd services/chat && python -m pytest tests/test_chain.py -v
```

Expected: FAIL — current code uses the function-default 5 instead of `settings.top_k`.

- [ ] **Step 5.4: Thread settings.top_k through call sites**

In `services/chat/app/chain.py`, change call sites that invoke `retrieve_chunks` or `chat_with_rag` from inside this module to pass `top_k=settings.top_k`. In `services/chat/app/main.py`, do the same at the HTTP entry points. Keep the function-default of 5 intact so library callers without settings still work.

- [ ] **Step 5.5: Run test to verify it passes**

```
cd services/chat && python -m pytest tests/test_chain.py tests/test_main.py -v
```

Expected: PASS.

- [ ] **Step 5.6: Commit**

```
git add services/chat/app/chain.py services/chat/app/main.py services/chat/tests/test_chain.py
git commit -m "feat(chat): honor settings.top_k at retrieval call sites"
```

---

## Task 6: Prompt registry with PROMPT_VERSION env var

**Files:**
- Modify: `services/chat/app/prompt.py`
- Modify: `services/chat/app/config.py`
- Create: `services/chat/tests/test_prompt.py`

- [ ] **Step 6.1: Write the failing tests**

Create `services/chat/tests/test_prompt.py` with:

- `test_v1_baseline_is_registered` — assert `"v1-baseline" in PROMPTS`.
- `test_build_prompt_uses_active_version` — monkeypatch `app.prompt.settings.prompt_version = "v1-baseline"`, call `build_rag_prompt(question="What is X?", chunks=[{"text": "X is a thing.", "filename": "f.pdf", "page_number": 1}])`, assert the rendered string contains `"X"`.
- `test_build_prompt_raises_for_unknown_version` — monkeypatch `prompt_version = "v999-missing"`, expect `KeyError` from `build_rag_prompt(question="q", chunks=[])`.
- `test_settings_validate_rejects_unknown_prompt_version` — instantiate `Settings(prompt_version="v999-missing")`, expect `validate()` to raise `ValueError` mentioning `prompt_version`.

- [ ] **Step 6.2: Run tests to verify they fail**

```
cd services/chat && python -m pytest tests/test_prompt.py -v
```

Expected: FAIL — `ImportError: cannot import name 'PROMPTS'`.

- [ ] **Step 6.3: Add the registry**

In `services/chat/app/prompt.py`:

1. Read the file first to see the current template variable.
2. Move the existing template body into `PROMPTS: dict[str, str] = {"v1-baseline": "...existing template..."}`.
3. Change `build_rag_prompt` to look up `PROMPTS[settings.prompt_version]` (let `KeyError` propagate; the validate hook prevents the bad-config case in production).

In `services/chat/app/config.py`, extend `validate()` to do a lazy import of `PROMPTS` and raise `ValueError(f"prompt_version '{self.prompt_version}' is not in the registry (known: {sorted(PROMPTS)})")` when not present. Lazy import avoids circular dependencies between `config` and `prompt`.

- [ ] **Step 6.4: Run prompt tests + chat full suite**

```
cd services/chat && python -m pytest tests/test_prompt.py -v && cd services/chat && python -m pytest tests/ -v
```

Expected: all PASS, including pre-existing tests that exercise the prompt indirectly.

- [ ] **Step 6.5: Commit**

```
git add services/chat/app/prompt.py services/chat/app/config.py services/chat/tests/test_prompt.py
git commit -m "feat(chat): introduce prompt registry with PROMPT_VERSION selection"
```

---

## Task 7: Ingestion collection-metadata SQLite store

**Why:** Qdrant has no first-class per-collection metadata; we keep our own. SQLite mirrors the eval service pattern.

**Files:**
- Create: `services/ingestion/app/collection_meta.py`
- Modify: `services/ingestion/app/config.py`
- Create: `services/ingestion/tests/test_collection_meta.py`

- [ ] **Step 7.1: Write the failing tests**

Create `services/ingestion/tests/test_collection_meta.py` with three async tests using `tmp_path`:

- `test_round_trip` — `upsert(collection="documents", chunk_size=1000, chunk_overlap=200, embedding_model="nomic-embed-text")` then `get("documents")` returns the same triple.
- `test_get_missing_returns_none` — `get("nope")` returns `None`.
- `test_init_idempotent` — instantiate two `CollectionMetaDB` objects against the same path, both `init()` succeeds.

- [ ] **Step 7.2: Run tests to verify they fail**

```
cd services/ingestion && python -m pytest tests/test_collection_meta.py -v
```

Expected: FAIL — module does not exist.

- [ ] **Step 7.3: Implement the metadata store**

Create `services/ingestion/app/collection_meta.py` with a `CollectionMetaDB` class:

- `__init__(self, db_path: str)` — store path; `_db` starts None.
- `async def init(self)` — open `aiosqlite` connection; row factory; `CREATE TABLE IF NOT EXISTS collection_meta (collection TEXT PRIMARY KEY, chunk_size INTEGER NOT NULL, chunk_overlap INTEGER NOT NULL, embedding_model TEXT NOT NULL)`; commit.
- `async def close(self)` — close the connection if open.
- `async def upsert(self, collection, chunk_size, chunk_overlap, embedding_model)` — `INSERT ... ON CONFLICT(collection) DO UPDATE SET ...`.
- `async def get(self, collection)` — return dict `{chunk_size, chunk_overlap, embedding_model}` or `None`.

In `services/ingestion/app/config.py` add `collection_meta_db_path: str = "data/collection_meta.db"`.

- [ ] **Step 7.4: Run tests to verify they pass**

```
cd services/ingestion && python -m pytest tests/test_collection_meta.py -v
```

Expected: all 3 PASS.

- [ ] **Step 7.5: Commit**

```
git add services/ingestion/app/collection_meta.py services/ingestion/app/config.py services/ingestion/tests/test_collection_meta.py
git commit -m "feat(ingestion): add per-collection metadata SQLite store"
```

---

## Task 8: Ingestion /collections/{name}/config endpoint + write-on-create

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/ingestion/app/store.py` (only if it owns collection creation)
- Modify: `services/ingestion/tests/test_main.py`

- [ ] **Step 8.1: Write the failing tests**

Append to `services/ingestion/tests/test_main.py`:

- `test_get_collection_config_404_when_unknown` — GET `/collections/nonexistent/config` returns 404.
- `test_get_collection_config_returns_metadata_after_upload` — arrange a metadata row directly via `CollectionMetaDB.upsert(...)` (override settings to point at a tmp path), then GET `/collections/test-coll/config` returns the same triple.

The simplest arrange path is to bypass the upload pipeline and seed the metadata DB directly — that keeps the test fast and isolates the endpoint surface from the upload pipeline.

- [ ] **Step 8.2: Run tests to verify they fail**

```
cd services/ingestion && python -m pytest tests/test_main.py -k collection_config -v
```

Expected: FAIL — endpoint does not exist.

- [ ] **Step 8.3: Add the endpoint and write-path**

In `services/ingestion/app/main.py`:

1. Wire `CollectionMetaDB` as a lazy global the same way eval wires `EvalDB` (see `services/eval/app/main.py:get_db`).
2. Add `GET /collections/{name}/config` returning the metadata or 404. Match the auth posture of the existing `/collections` endpoint (look it up; if it requires `require_auth`, do the same).
3. In the upload/ingestion code path (search for where `QdrantStore` creates the collection or where chunks are written), call `await meta_db.upsert(collection=..., chunk_size=settings.chunk_size, chunk_overlap=settings.chunk_overlap, embedding_model=settings.embedding_model)`. Idempotent due to `ON CONFLICT`.

- [ ] **Step 8.4: Run tests to verify they pass**

```
cd services/ingestion && python -m pytest tests/test_main.py -k collection_config -v
```

Expected: PASS.

- [ ] **Step 8.5: Commit**

```
git add services/ingestion/app/main.py services/ingestion/app/store.py services/ingestion/tests/test_main.py
git commit -m "feat(ingestion): expose /collections/{name}/config and persist metadata at upload"
```

---

## Task 9: Eval config-snapshot helper

**Why isolated:** Pure helper that takes URLs and returns a merged dict; easy to unit test without spinning HTTP servers. Orchestration that calls it from the background task is Task 10.

**Files:**
- Modify: `services/eval/app/config.py` — add `ingestion_service_url`
- Create: `services/eval/app/config_capture.py`
- Create: `services/eval/tests/test_config_capture.py`
- Modify: `services/eval/requirements.txt` (add `respx`)

- [ ] **Step 9.1: Write the failing tests**

Create `services/eval/tests/test_config_capture.py` with three tests using `respx` to mock httpx:

- `test_capture_merges_chat_and_collection` — both endpoints return 200; result has `chat`, `collection`, `captured_at`, no `_capture_error`.
- `test_capture_records_error_when_chat_fails` — chat endpoint raises `httpx.ConnectError`; result has `_capture_error` substring "chat" and still has `collection`.
- `test_capture_records_error_when_collection_unknown` — ingestion endpoint returns 404; result has `_capture_error` substring "collection" and still has `chat`.

The function under test: `await capture_run_config(chat_url=..., ingestion_url=..., collection=...) -> dict`.

- [ ] **Step 9.2: Run tests to verify they fail**

```
cd services/eval && python -m pytest tests/test_config_capture.py -v
```

Expected: FAIL — `ImportError: cannot import name 'capture_run_config'`.

- [ ] **Step 9.3: Implement the helper**

In `services/eval/app/config.py`, add `ingestion_service_url: str = "http://ingestion:8000"`.

Create `services/eval/app/config_capture.py` exporting `async def capture_run_config(chat_url: str, ingestion_url: str, collection: str) -> dict`. Implementation:

1. Open one `httpx.AsyncClient` with a 5s timeout.
2. `asyncio.gather(_fetch(chat), _fetch(coll), return_exceptions=True)`.
3. Build output dict starting with `{"captured_at": iso_now}`.
4. For each leg: if `Exception` → append a string to an `errors` list; else attach to output under `"chat"` or `"collection"`.
5. If `errors`, set `out["_capture_error"] = "; ".join(errors)`.
6. Return `out`.

Add `respx` to `services/eval/requirements.txt`.

- [ ] **Step 9.4: Run tests to verify they pass**

```
cd services/eval && python -m pytest tests/test_config_capture.py -v
```

Expected: all 3 PASS.

- [ ] **Step 9.5: Commit**

```
git add services/eval/app/config_capture.py services/eval/app/config.py services/eval/tests/test_config_capture.py services/eval/requirements.txt
git commit -m "feat(eval): config_capture helper snapshots chat+ingestion in parallel"
```

---

## Task 10: Background task snapshots config at run start

**Files:**
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 10.1: Write the failing test**

Append `test_run_persists_config_snapshot` to `services/eval/tests/test_main.py`:

1. Monkeypatch `app.main.capture_run_config` to return a known dict like `{"chat": {"llm_model": "qwen"}, "collection": {"chunk_size": 1000}, "captured_at": "2026-04-28T00:00:00+00:00"}`.
2. Monkeypatch `app.main.run_evaluation` to return `({"faithfulness": 0.9}, [])` so the test is fast.
3. POST a dataset, POST `/evaluations`.
4. GET the run detail and assert `detail["config"]["chat"]["llm_model"] == "qwen"` and `detail["config"]["collection"]["chunk_size"] == 1000`.

- [ ] **Step 10.2: Run test to verify it fails**

```
cd services/eval && python -m pytest tests/test_main.py::test_run_persists_config_snapshot -v
```

Expected: FAIL — background task doesn't call `capture_run_config` yet, so `config` stays None.

- [ ] **Step 10.3: Wire the capture into the background task**

In `services/eval/app/main.py`, import `capture_run_config` from `app.config_capture`. Modify `_run_evaluation_task` so that *before* the existing RAGAS call, it does:

```
config = await capture_run_config(
    chat_url=settings.chat_service_url,
    ingestion_url=settings.ingestion_service_url,
    collection=collection or "documents",
)
await db.set_evaluation_config(eval_id, config)
```

Keep the rest of the function unchanged. Failures in `capture_run_config` are already swallowed inside the helper (returns `_capture_error` instead of raising), so the run still completes.

- [ ] **Step 10.4: Run test to verify it passes**

```
cd services/eval && python -m pytest tests/test_main.py::test_run_persists_config_snapshot -v
```

Expected: PASS.

- [ ] **Step 10.5: Commit**

```
git add services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): snapshot RAG config at run start"
```

---

## Task 11: GET /evaluations/compare

**Files:**
- Modify: `services/eval/app/db.py` — add `get_evaluations_by_ids`
- Modify: `services/eval/app/main.py` — add the endpoint
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 11.1: Write the failing tests**

Append to `services/eval/tests/test_main.py` four `compare_*` tests:

- `test_compare_happy_path` — seed a dataset and two completed runs (use direct `db.complete_evaluation(...)` in the arrange block to set known aggregate scores, e.g., `{"faithfulness": 0.8, "answer_relevancy": 0.7, "context_precision": 0.6, "context_recall": 0.5}` for run A and `{...0.85, 0.75, 0.65, 0.55}` for run B). GET `/evaluations/compare?ids=A,B`. Assert 200, `len(runs)==2`, `deltas["faithfulness"][0]==0.0` and `deltas["faithfulness"][1]==0.05`.
- `test_compare_400_on_mixed_datasets` — two runs with different `dataset_id`. GET. Assert 400 + detail contains "same dataset".
- `test_compare_400_on_too_few_or_too_many` — 1 id and 6 ids both return 400.
- `test_compare_404_on_unknown_id` — GET with synthetic UUIDs returns 404.

- [ ] **Step 11.2: Run tests to verify they fail**

```
cd services/eval && python -m pytest tests/test_main.py -k compare -v
```

Expected: FAIL — endpoint missing.

- [ ] **Step 11.3: Implement the endpoint**

In `services/eval/app/db.py`, add `async def get_evaluations_by_ids(self, ids: list[str]) -> list[dict]`:

- Build SQL with `IN (?, ?, ...)` placeholders sized to len(ids).
- Fetch rows.
- Return them in **input order** (build a dict by id, then iterate `ids`).
- Use `_row_to_dict` from Task 1.

In `services/eval/app/main.py`, add `GET /evaluations/compare`:

1. Parse `ids` query param as comma-separated; validate `2 <= len <= 5` (else 400 "compare requires 2-5 ids").
2. Fetch via `db.get_evaluations_by_ids(id_list)`.
3. If `len(rows) != len(id_list)`, raise 404 with the missing ids in the detail.
4. Validate all `dataset_id` are equal (else 400 "all runs must belong to the same dataset").
5. For each of the four metric names, compute `deltas[m] = [round(score - baseline, 6) if both not None else 0.0 for each run]` where baseline = first run's score for that metric.
6. Return `{"runs": rows, "deltas": deltas}`.

Apply `@limiter.limit("30/minute")` matching other GET endpoints.

- [ ] **Step 11.4: Run tests**

```
cd services/eval && python -m pytest tests/test_main.py -k compare -v
```

Expected: all 4 PASS.

- [ ] **Step 11.5: Commit**

```
git add services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): GET /evaluations/compare with delta computation"
```

---

## Task 12: GET /evaluations/history

**Files:**
- Modify: `services/eval/app/db.py` — add `get_history`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/tests/test_main.py`

- [ ] **Step 12.1: Write failing tests**

Append three tests to `services/eval/tests/test_main.py`:

- `test_history_returns_completed_runs_for_pair` — seed: one dataset with three completed runs on `collection=documents`, one completed run on `collection=other`, and one *failed* run on `collection=documents`. GET `/evaluations/history?dataset_id=ds&collection=documents`. Assert 200, `len(runs)==3` (failed and other-collection runs excluded), and timestamps are sorted ASC.
- `test_history_400_when_filters_missing` — one call with only `dataset_id`, another with only `collection`; both return 400.
- `test_history_empty_returns_200` — GET with a dataset id that has no runs returns 200 with `{"runs": []}`.

- [ ] **Step 12.2: Run tests**

```
cd services/eval && python -m pytest tests/test_main.py -k history -v
```

Expected: FAIL.

- [ ] **Step 12.3: Implement**

In `services/eval/app/db.py`, add `async def get_history(self, dataset_id: str, collection: str) -> list[dict]`:

- SELECT all rows WHERE `dataset_id = ?` AND `collection = ?` AND `status = 'completed'` ORDER BY `created_at` ASC.
- Return `[_row_to_dict(r) for r in rows]`.

In `services/eval/app/main.py`, add `GET /evaluations/history`:

- Accept `dataset_id: str | None = None`, `collection: str | None = None`.
- If either is missing, raise 400 with detail "dataset_id and collection are both required".
- Otherwise return `{"runs": await db.get_history(...)}`.
- Apply `@limiter.limit("30/minute")`.

- [ ] **Step 12.4: Run tests**

```
cd services/eval && python -m pytest tests/test_main.py -k history -v
```

Expected: PASS.

- [ ] **Step 12.5: Commit**

```
git add services/eval/app/db.py services/eval/app/main.py services/eval/tests/test_main.py
git commit -m "feat(eval): GET /evaluations/history filtered by dataset+collection"
```

---

## Task 13: K8s ConfigMaps + compose env keys

**Files:**
- Modify: `k8s/ai-services/configmaps/chat-config.yml`
- Modify: `k8s/ai-services/configmaps/eval-config.yml`
- Modify: `k8s/ai-services/configmaps/ingestion-config.yml`
- Modify: `.env.example` (only if it exists in repo root)

- [ ] **Step 13.1: Update chat ConfigMap**

Add to `data:`:
```
TOP_K: "5"
PROMPT_VERSION: v1-baseline
```

(Quote `"5"` because ConfigMap values must be strings.)

- [ ] **Step 13.2: Update eval ConfigMap**

Add to `data:`:
```
INGESTION_SERVICE_URL: http://ingestion:8000
```

- [ ] **Step 13.3: Update ingestion ConfigMap**

Add to `data:`:
```
COLLECTION_META_DB_PATH: /app/data/collection_meta.db
```

If the ingestion deployment does not already mount a `/app/data` volume for SQLite persistence, add an `emptyDir` volume + `volumeMount` in `k8s/ai-services/deployments/ingestion.yml`. Inspect `k8s/ai-services/deployments/eval.yml` for the existing pattern; mirror it.

- [ ] **Step 13.4: Update `.env.example` (if present)**

If `.env.example` exists at repo root, append the four keys with their default values. If it does not exist, skip — Python settings provide defaults that make the new vars non-blocking for compose-smoke.

- [ ] **Step 13.5: Validate kustomize build**

```
kubectl kustomize k8s/ai-services > /dev/null && kubectl kustomize k8s/overlays/qa > /dev/null
```

Expected: both render with no errors.

- [ ] **Step 13.6: Commit**

```
git add k8s/ai-services/configmaps/chat-config.yml k8s/ai-services/configmaps/eval-config.yml k8s/ai-services/configmaps/ingestion-config.yml
# Plus .env.example or k8s/ai-services/deployments/ingestion.yml if changed
git commit -m "chore(k8s): wire TOP_K, PROMPT_VERSION, INGESTION_SERVICE_URL, COLLECTION_META_DB_PATH"
```

---

## Task 14: QA-environment safety check

CLAUDE.md flags: "Shared-infra services must exist in their prod namespace before QA can ExternalName-route to them." For this PR, `INGESTION_SERVICE_URL=http://ingestion:8000` resolves *within the same namespace* (prod uses `ai-services/ingestion`, QA uses `ai-services-qa/ingestion` — both have an `ingestion` Service). No cross-namespace route is added.

- [ ] **Step 14.1: Verify ingestion Service exists in QA overlay render**

```
kubectl kustomize k8s/overlays/qa | grep "name: ingestion"
```

Expected: `name: ingestion` appears (Service kind). If not, add it before merging.

(No commit needed if no change.)

---

## Task 15: Final preflight + PR

- [ ] **Step 15.1: Run full Python preflight**

```
make preflight-python
```

Expected: ruff + pytest across all services pass.

- [ ] **Step 15.2: Run security preflight**

```
make preflight-security
```

Expected: bandit + pip-audit + gitleaks pass. If pip-audit complains about `respx`, accept it — dev dependency.

- [ ] **Step 15.3: Verify all three service test suites are green**

```
cd services/eval && python -m pytest tests/ -v
cd services/chat && python -m pytest tests/ -v
cd services/ingestion && python -m pytest tests/ -v
```

Expected: all green.

- [ ] **Step 15.4: Push and open the PR**

```
git push -u origin agent/feat-eval-tracking-backend
```

Then open the PR with `gh pr create --base qa` and a body covering: the 3 schema columns, the 2 new eval endpoints, chat `/config` + prompt registry + configurable `top_k`, ingestion `/collections/{name}/config` + metadata persistence, the parallel snapshot helper, and the K8s ConfigMap additions. Closes #84.

- [ ] **Step 15.5: Notify Kyle with the PR URL**

---

## Self-Review Checklist

**Spec coverage:**
- Schema additions — Task 1
- Pydantic model updates — Task 2
- POST /evaluations notes/baseline — Task 3
- Chat /config — Task 4
- Configurable top_k — Tasks 4 + 5
- Prompt registry — Task 6
- Ingestion metadata store — Task 7
- Ingestion /collections/{name}/config + write path — Task 8
- Config snapshot helper — Task 9
- Snapshot wired into background task — Task 10
- /evaluations/compare — Task 11
- /evaluations/history — Task 12
- K8s ConfigMaps — Task 13
- QA-namespace safety check — Task 14
- Preflight + PR — Task 15

**Type consistency check:**
- `notes: str | None` — same in DB, models, request, response, tests.
- `baseline_eval_id: str | None` — same throughout.
- `config: dict[str, Any] | None` — Pydantic shape matches DB JSON round-trip.
- `RunComparison.deltas: dict[str, list[float]]` — keyed by the four RAGAS metric names that Task 11 enumerates.
- `capture_run_config(chat_url, ingestion_url, collection)` — same signature in helper, test, and caller.

**Phase 2 deferral note:** The frontend Trends-tab plan (`2026-04-28-rag-tracking-dashboard-phase2-frontend.md`) will be written *after* this PR ships, when the API surface is live and the implementer can read response shapes from QA rather than guessing from this spec. Splitting the two phases as separate plans matches the "two PRs" decision in the spec.
