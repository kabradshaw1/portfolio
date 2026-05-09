# Eval Dataset Item Counts Design

## Issue

GitHub issue: https://github.com/kabradshaw1/portfolio/issues/238

`GET /datasets` in the eval service currently returns each dataset's `id`,
`name`, and `created_at` timestamp. Consumers cannot see how many golden items a
dataset contains without loading the full dataset detail. The RAG dashboard work
can tolerate a missing count, but the list is less useful for selecting datasets
and judging evaluation coverage.

## Goals

- Add `item_count` to each dataset summary returned by `GET /datasets`.
- Keep the existing response envelope: `{ "datasets": [...] }`.
- Preserve backward compatibility by adding a response field only.
- Avoid a database migration for this enhancement.
- Cover the DB behavior and API response shape with focused backend tests.

## Non-Goals

- Do not change dataset creation validation or the maximum dataset size.
- Do not add pagination, filtering, or sorting changes to `GET /datasets`.
- Do not change `DatasetDetail`; it already returns full `items`.
- Do not introduce a stored `item_count` column.

## Recommended Approach

Compute `item_count` in `EvalDB.list_datasets()` from the existing JSON
`datasets.items` column at read time.

This is the lowest-risk production-grade option for the current model. Dataset
items are already persisted as JSON and are capped at 100 items by
`CreateDatasetRequest`, so decoding each listed dataset is inexpensive. The
approach avoids SQLite JSON extension assumptions and avoids migration/backfill
complexity before the service needs a denormalized count.

## Alternatives Considered

### SQLite JSON Function

The query could use `json_array_length(items) AS item_count`. That avoids Python
JSON decoding, but it depends on SQLite JSON1 support being available in every
local, CI, and runtime environment. The portability risk is not worth it for the
current dataset size.

### Stored Count Column

The service could add an `item_count` column and populate it during dataset
creation. That may become useful if datasets become much larger or mutable, but
it adds migration and backfill work for an additive list-field enhancement.

## API Contract

`GET /datasets` continues returning the same envelope:

```json
{
  "datasets": [
    {
      "id": "ds-1",
      "name": "rag-regression",
      "created_at": "2026-04-16T00:00:00Z",
      "item_count": 12
    }
  ]
}
```

`item_count` is always an integer. Existing clients that ignore unknown fields
remain compatible.

`DatasetSummary` should include:

```python
item_count: int
```

## Data Flow

1. `GET /datasets` calls `EvalDB.list_datasets()`.
2. `EvalDB.list_datasets()` selects `id`, `name`, `created_at`, and `items`
   from `datasets`, preserving the existing `ORDER BY created_at DESC`.
3. For each row, the method decodes `items` with `json.loads()` and returns
   `item_count` as the length of the decoded list.
4. The endpoint returns the summaries unchanged inside `{ "datasets": ... }`.

## Error Handling

The `items` column is written by `create_dataset()` from validated request data.
If a stored dataset contains invalid JSON, `list_datasets()` should fail
naturally instead of returning a misleading `item_count` such as `0`.

This treats corrupted dataset JSON as a data integrity problem rather than a
recoverable user-facing condition.

## Test Plan

Update `services/eval/tests/test_db.py`:

- Create at least two datasets with different item counts.
- Call `db.list_datasets()`.
- Assert each returned summary includes the expected `item_count`.
- Keep the existing assertion that both dataset names are returned.

Update `services/eval/tests/test_main.py`:

- Mock `list_datasets()` to return summaries that include `item_count`.
- Assert `GET /datasets` returns status `200`.
- Assert the response keeps the `{ "datasets": [...] }` envelope and preserves
  the `item_count` values.

No separate corrupted-JSON test is required for this issue because the desired
behavior is natural failure, not a formalized error response contract.

## Verification

Before committing the implementation, run:

```bash
make preflight-python
make preflight-security
```
