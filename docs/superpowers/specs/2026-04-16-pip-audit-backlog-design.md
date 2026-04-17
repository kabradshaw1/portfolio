# Spec: Address Ignored Python pip-audit Advisories

**Issue:** #62
**Date:** 2026-04-16

## Context

The CI pipeline ignores 9 pip-audit advisories to stay green. Most stem from `langchain-community==0.2.19`, which is listed as a dependency in all three Python services but **only used for `langchain-text-splitters`** (via `RecursiveCharacterTextSplitter`). The chat service doesn't import langchain at all. Rather than upgrading the full langchain stack to 0.3.x, we can drop the unused `langchain-community` package entirely and upgrade only `langchain-text-splitters`.

## Approach

1. **Drop `langchain-community`** from all three services — it's unused at the import level.
2. **Upgrade `langchain-text-splitters`** from 0.2.4 to the latest 0.3.x release.
3. **Upgrade pytest** from 8.4.2 to 9.0.3+ (fixes CVE-2025-71176).
4. **Verify the dependency tree** — confirm `langchain-core` is no longer pulled in transitively (resolves CVE-2026-34070).
5. **Remove resolved `--ignore-vuln` flags** from CI. Add dated notes for any that remain.
6. **Quick-audit ADR notebooks** for stale langchain version references.

## Files to Modify

### requirements.txt (all three services)

| File | Remove | Upgrade |
|------|--------|---------|
| `services/chat/requirements.txt` | `langchain-community==0.2.19` | `pytest` → 9.0.3+ |
| `services/ingestion/requirements.txt` | `langchain-community==0.2.19` | `langchain-text-splitters` → 0.3.x, `pytest` → 9.0.3+ |
| `services/debug/requirements.txt` | `langchain-community==0.2.19` | `langchain-text-splitters` → 0.3.x, `pytest` → 9.0.3+ |

Also check `pytest-asyncio` and `pytest-cov` compatibility with pytest 9.x and bump if needed.

### Service source code

- `services/ingestion/app/chunker.py` — No changes expected. Import path `from langchain_text_splitters import RecursiveCharacterTextSplitter` is identical in 0.3.x.
- `services/debug/app/indexer.py` — No changes expected. `Language.PYTHON` and `RecursiveCharacterTextSplitter.from_language()` are unchanged in 0.3.x.

### CI workflow

**`.github/workflows/ci.yml`** (~line 487) — remove these `--ignore-vuln` flags:

| Advisory | Resolved by |
|----------|-------------|
| CVE-2025-6984 | Dropping langchain-community |
| CVE-2025-6985 | Dropping langchain-community |
| CVE-2025-65106 | Dropping langchain-community |
| CVE-2025-68664 | Dropping langchain-community |
| CVE-2026-26013 | Dropping langchain-community |
| GHSA-926x-3r5x-gfhw | Dropping langchain-core (transitive) |
| GHSA-rr7j-v2q5-chgv | Dropping langsmith (transitive via langchain) |
| CVE-2025-71176 | Upgrading pytest to 9.0.3+ |
| CVE-2026-34070 | Verify — should be gone if langchain-core is no longer in dep tree |

If CVE-2026-34070 persists (langchain-text-splitters still pulls langchain-core), either pin langchain-core to a clean patched version or add a dated residual-risk note.

### Documentation updates

- **`services/CLAUDE.md`** line 28 — update to reflect langchain-community removed, text-splitters upgraded.
- **ADR notebooks** (`docs/adr/document-qa/`, `docs/adr/document-debugger/`) — scan for langchain version mentions or stale package names; update text where needed. No code cell rewrites unless an import path actually changed.

## What's NOT in scope

- Full langchain 0.3.x migration (not needed — we're dropping langchain-community instead)
- Changes to the RAG pipeline logic or debug agent loop
- New features or refactoring beyond what's needed to clear CVEs

## Verification

1. Install updated deps in each service's virtualenv; run `pip-audit` with **no** `--ignore-vuln` flags.
2. `make preflight-python` — all unit tests pass across chat, ingestion, debug.
3. `make preflight-security` — pip-audit step is clean (zero ignores or only dated residual notes).
4. Confirm `RecursiveCharacterTextSplitter` and `Language.PYTHON` behave identically with the upgraded `langchain-text-splitters`.
5. `make preflight-e2e` if any notebook cells were updated.

## Acceptance Criteria

Per issue #62: every `--ignore-vuln` in the pip-audit step is either removed (package upgraded/dropped) or has a dated note explaining why it's a conscious residual risk.
