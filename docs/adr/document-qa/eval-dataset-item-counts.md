# Eval Dataset Item Counts

- **Date:** 2026-05-08
- **Status:** Accepted
- **Related issue:** #238
- **Related PR:** #242
- **Builds on:** [rag-evaluation-service.md](rag-evaluation-service.md), [rag-tracking-foundation.md](rag-tracking-foundation.md)

## Context

The eval service stores golden datasets in SQLite and exposes them through
`GET /datasets`. Before this change, the list endpoint returned only `id`,
`name`, and `created_at`. Consumers could not tell how much evaluation coverage
each dataset represented without loading full dataset contents.

That gap mattered for the RAG dashboard work because dataset size is part of
how a human should interpret evaluation results. A run against 3 golden items
and a run against 80 golden items should not carry the same confidence.

The existing dataset table stores `items` as JSON text. Dataset creation already
validates item shape through Pydantic and caps each dataset at 100 items, so the
current data volume is small. The API change needed to be additive because
frontend work can already tolerate a missing `item_count`.

Opening the PR also caused the eval service pip-audit job to run. That surfaced
a newly reported `langchain-core` CVE in the existing `ragas==0.2.15`
transitive dependency chain. The finding was unrelated to dataset counts but
had to be handled for the PR to pass CI.

## Decision

Add `item_count` to dataset summaries returned by `GET /datasets`.

`EvalDB.list_datasets()` now selects the persisted `items` JSON column, decodes
it with `json.loads()`, and returns `item_count` as `len(items)`. The endpoint
keeps the existing response envelope:

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

`DatasetSummary` includes `item_count: int` so the typed API model reflects the
response contract.

We explicitly did **not** add a stored `item_count` column. A persisted count
would require a migration and backfill for data that is already present in each
row. Given the 100-item cap and read-only dataset contents, read-time counting
is simpler and accurate.

We also did **not** use SQLite JSON functions such as `json_array_length()`.
Python-side decoding avoids relying on SQLite JSON extension availability across
local, CI, and container environments.

For the new `CVE-2026-44843` pip-audit finding, CI now documents and ignores
that advisory under the same model as
[ragas-cve-risk-assessment.md](ragas-cve-risk-assessment.md). The vulnerability
concerns older LangChain deserialization/runtime APIs such as
`RunnableWithMessageHistory`, `astream_log()`, `astream_events(version="v1")`,
and broad `load()`/`loads()` object revival. The eval service validates incoming
requests into fixed schemas and does not use those APIs.

## Consequences

**Positive:**

- Dataset list consumers can show evaluation coverage without fetching full
  dataset details.
- The change is additive and keeps existing clients compatible.
- No database migration or backfill is required.
- Counts are derived from the source of truth, so they cannot drift from stored
  dataset contents.
- The new pip-audit exception is tied to a specific advisory and usage
  assessment rather than a broad suppression.

**Trade-offs:**

- `GET /datasets` now decodes each dataset's `items` JSON. This is acceptable
  under the current 100-item cap, but should be revisited if datasets become
  large, mutable, or paginated at high volume.
- Corrupted `items` JSON will cause listing to fail rather than returning a
  fallback count. That is intentional: silent `0` counts would hide data
  integrity problems and make coverage misleading.
- The eval service still carries known RAGAS 0.2.x transitive CVEs that are not
  exploitable under current usage. If eval starts accepting untrusted templates,
  multimodal inputs, LangChain serialized objects, or older LangChain runtime
  event APIs, the RAGAS CVE assessment and CI ignore list must be revisited.

**Future work:**

- If dataset size grows beyond the current bounded JSON model, add a persisted
  `item_count` column with an idempotent backfill migration.
- Revisit the RAGAS dependency chain when a stable RAGAS release can move eval
  off LangChain 0.2.x without breaking metrics.
