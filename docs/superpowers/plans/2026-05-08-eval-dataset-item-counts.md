# Eval Dataset Item Counts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `item_count` to each dataset summary returned by the eval service `GET /datasets` API.

**Architecture:** Keep the existing dataset table and response envelope. Compute `item_count` inside `EvalDB.list_datasets()` by decoding the existing `datasets.items` JSON column and counting the persisted golden items. The FastAPI endpoint continues returning DB summaries unchanged inside `{ "datasets": [...] }`.

**Tech Stack:** Python 3, FastAPI, Pydantic, aiosqlite, pytest, pytest-asyncio.

---

## File Structure

- Modify `services/eval/app/db.py`
  - Update `EvalDB.list_datasets()` to select `items`, decode it with `json.loads()`, and return `item_count`.
- Modify `services/eval/app/models.py`
  - Add `item_count: int` to `DatasetSummary`.
- Modify `services/eval/tests/test_db.py`
  - Update `test_list_datasets` so it creates datasets with different item counts and asserts the returned counts.
- Modify `services/eval/tests/test_main.py`
  - Update `test_list_datasets` so the mocked API contract includes and preserves `item_count`.

## Task 1: DB Summary Includes Item Count

**Files:**
- Modify: `services/eval/tests/test_db.py`
- Modify: `services/eval/app/db.py`

- [ ] **Step 1: Write the failing DB test**

Replace `test_list_datasets` in `services/eval/tests/test_db.py` with:

```python
@pytest.mark.asyncio
async def test_list_datasets(db):
    await db.create_dataset(name="ds1", items=SIMPLE_ITEM)
    await db.create_dataset(
        name="ds2",
        items=[
            {"query": "q1", "expected_answer": "a1", "expected_sources": []},
            {"query": "q2", "expected_answer": "a2", "expected_sources": []},
        ],
    )

    datasets = await db.list_datasets()
    assert len(datasets) == 2
    by_name = {d["name"]: d for d in datasets}
    assert set(by_name) == {"ds1", "ds2"}
    assert by_name["ds1"]["item_count"] == 1
    assert by_name["ds2"]["item_count"] == 2
```

- [ ] **Step 2: Run the DB test to verify it fails**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_db.py::test_list_datasets -v
```

Expected: FAIL with `KeyError: 'item_count'`.

- [ ] **Step 3: Implement item counting in `EvalDB.list_datasets()`**

Replace `list_datasets()` in `services/eval/app/db.py` with:

```python
    async def list_datasets(self) -> list[dict]:
        cursor = await self._db.execute(
            "SELECT id, name, items, created_at FROM datasets ORDER BY created_at DESC"
        )
        rows = await cursor.fetchall()
        return [
            {
                "id": r["id"],
                "name": r["name"],
                "created_at": r["created_at"],
                "item_count": len(json.loads(r["items"])),
            }
            for r in rows
        ]
```

- [ ] **Step 4: Run the DB test to verify it passes**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_db.py::test_list_datasets -v
```

Expected: PASS.

## Task 2: API Contract Preserves Item Count

**Files:**
- Modify: `services/eval/tests/test_main.py`
- Modify: `services/eval/app/models.py`

- [ ] **Step 1: Write the failing API contract test**

Replace `test_list_datasets` in `services/eval/tests/test_main.py` with:

```python
@patch("app.main.get_db")
def test_list_datasets(mock_get_db):
    mock_db = AsyncMock()
    mock_db.list_datasets.return_value = [
        {
            "id": "ds-1",
            "name": "ds1",
            "created_at": "2026-04-16T00:00:00Z",
            "item_count": 1,
        },
        {
            "id": "ds-2",
            "name": "ds2",
            "created_at": "2026-04-16T01:00:00Z",
            "item_count": 2,
        },
    ]
    mock_get_db.return_value = mock_db

    response = client.get("/datasets")

    assert response.status_code == 200
    assert response.json() == {
        "datasets": [
            {
                "id": "ds-1",
                "name": "ds1",
                "created_at": "2026-04-16T00:00:00Z",
                "item_count": 1,
            },
            {
                "id": "ds-2",
                "name": "ds2",
                "created_at": "2026-04-16T01:00:00Z",
                "item_count": 2,
            },
        ]
    }
```

- [ ] **Step 2: Run the API test**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_main.py::test_list_datasets -v
```

Expected: PASS. The endpoint currently returns the DB payload unchanged, so this test documents the additive API contract.

- [ ] **Step 3: Update the Pydantic summary model**

In `services/eval/app/models.py`, update `DatasetSummary` to:

```python
class DatasetSummary(BaseModel):
    id: str
    name: str
    created_at: str
    item_count: int
```

- [ ] **Step 4: Run the API test again**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_main.py::test_list_datasets -v
```

Expected: PASS.

## Task 3: Focused Regression Sweep

**Files:**
- Verify: `services/eval/tests/test_db.py`
- Verify: `services/eval/tests/test_main.py`

- [ ] **Step 1: Run focused eval DB/API tests**

Run:

```bash
PYTHONPATH=services/eval:services pytest services/eval/tests/test_db.py services/eval/tests/test_main.py -v
```

Expected: 38 tests pass. Existing FastAPI shutdown deprecation warnings may appear.

- [ ] **Step 2: Inspect the diff**

Run:

```bash
git diff -- services/eval/app/db.py services/eval/app/models.py services/eval/tests/test_db.py services/eval/tests/test_main.py
```

Expected: Diff is limited to `item_count` behavior and tests.

## Task 4: Required Preflight And Delivery

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run Python preflight**

Run:

```bash
make preflight-python
```

Expected: ruff lint, ruff format check, and configured Python service tests pass.

- [ ] **Step 2: Run security preflight**

Run:

```bash
make preflight-security
```

Expected: configured security checks pass.

- [ ] **Step 3: Commit the implementation**

Run:

```bash
git status --short
git add services/eval/app/db.py services/eval/app/models.py services/eval/tests/test_db.py services/eval/tests/test_main.py docs/superpowers/plans/2026-05-08-eval-dataset-item-counts.md
git commit -m "feat: include eval dataset item counts"
```

Expected: Commit succeeds on branch `issue-238-eval-dataset-item-counts`.

- [ ] **Step 4: Push the feature branch**

Run:

```bash
git push -u origin issue-238-eval-dataset-item-counts
```

Expected: Branch is pushed to remote.

- [ ] **Step 5: Open a pull request to `qa`**

Run:

```bash
gh pr create --base qa --head issue-238-eval-dataset-item-counts --title "Eval: include dataset item counts" --body "## Summary
- add item_count to eval dataset summaries
- compute item_count from persisted dataset items
- cover DB and API response behavior with tests

## Verification
- PYTHONPATH=services/eval:services pytest services/eval/tests/test_db.py services/eval/tests/test_main.py -v
- make preflight-python
- make preflight-security

Closes #238"
```

Expected: GitHub creates a PR targeting `qa`.
