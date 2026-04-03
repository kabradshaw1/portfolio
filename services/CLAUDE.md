# Services

All backend services are Python/FastAPI microservices.

## Package Selection

- Prefer minimal, focused packages over large frameworks (e.g., `langchain-text-splitters` not the full `langchain` framework)
- When adding or updating dependencies, use context7 to:
  - Verify the package is not deprecated or renamed
  - Check current recommended import paths
  - Confirm API usage patterns are current
  - Review version compatibility

### Known Deprecations

- PyPDF2 → `pypdf` (same API, renamed by the same authors)

## Pre-commit Requirements

Before every commit touching `services/`:
- `ruff check services/` must pass
- `ruff format --check services/` must pass
- Pre-commit hooks run automatically (ruff lint + format)
- If pre-commit rejects a commit, stage the auto-fixed files and re-commit

## Known Issues

- langchain 0.2.x has 5 CVEs that require 0.3.x migration (ignored in pip-audit). Migration tracked as future work.

## Adding a New Service

When adding a new service under `services/`, update these:
1. `ci.yml` — add to `backend-tests.strategy.matrix.service`
2. `ci.yml` — add to `docker-build.strategy.matrix.service`
3. `ci.yml` — add to `security-pip-audit.strategy.matrix.service`
4. `ci.yml` — add Dockerfile path to `security-hadolint.strategy.matrix.dockerfile`
5. `docker-compose.yml` — add service with GHCR image
6. `ci.yml` deploy step — add service name to `docker compose pull` command
7. `docs/adr/<service-name>/` — create companion ADR notebooks explaining the service's design decisions step-by-step
