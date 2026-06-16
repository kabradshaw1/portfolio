# dspm-classifier

DSPM-flavored sensitive-data classification service. Consumes object-storage
events from Kafka, classifies content via a tiered regex/NER/LLM pipeline,
persists findings to Postgres, and emits to a downstream topic.

See: `docs/superpowers/specs/2026-06-16-dspm-classifier-design.md`.

## Status

Plan 1 (in progress) — classification engine + CLI driver. Kafka and FastAPI
arrive in Plans 2 and 3.

## Local development

Requires Docker (Postgres + MinIO via testcontainers for tests):

```bash
make preflight-python  # runs from repo root
```
