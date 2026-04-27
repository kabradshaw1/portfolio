# Scaffold Go Service — Smoke Test Checklist Update

## Problem

The `scaffold-go-service` skill's "Frontend + Smoke Tests" section is vague — it says "update smoke.spec.ts if endpoints moved" without specifying what actually needs to happen. After building out the smoke test infrastructure (Go compose-smoke CI, prod health checks, compose config validation), there are 7 specific files that need updates when adding a new Go service. Missing any of them causes CI failures that are only discoverable after pushing.

## Approach

Update the existing checklist in `.claude/skills/scaffold-go-service/SKILL.md`. Keep the `disable-model-invocation: true` format — the skill stays a static checklist, not an interactive wizard.

## Changes

### Replace "Frontend + Smoke Tests" section (lines 231-235)

Replace the existing 3-line section with a detailed "Smoke Tests & Compose CI" section covering:

1. **`go/ci-init.sql`** — add `CREATE DATABASE <dbname>;` and `GRANT ALL PRIVILEGES ON DATABASE <dbname> TO taskuser;` if the service uses its own database (not `ecommercedb`)

2. **`go/docker-compose.ci.yml`** — add a service block with:
   - `build: { context: ., dockerfile: <service>/Dockerfile }`
   - Ports (REST + gRPC if applicable)
   - Environment overrides: `DATABASE_URL` (with `?sslmode=disable`), `JWT_SECRET: ci-test-secret`, `ALLOWED_ORIGINS: "*"`, any gRPC addresses to other services (e.g., `AUTH_GRPC_URL: auth-service:9091`)
   - `depends_on` with health conditions for infra services

3. **CI workflow migration step** (`.github/workflows/ci.yml`, compose-smoke-go job) — add:
   ```
   $DC run --rm --no-deps --entrypoint migrate <service> \
     -path /migrations \
     -database "postgres://taskuser:taskpass@postgres:5432/<dbname>?sslmode=disable" up
   ```
   Use `&x-migrations-table=<svc>_schema_migrations` if sharing a database with another service.

4. **CI workflow seed step** — if the service has a `seed.sql`, add:
   ```
   $DC run --rm --no-deps --entrypoint sh <service> \
     -c 'PGPASSWORD=taskpass psql -h postgres -U taskuser -d <dbname> -f /seed.sql'
   ```

5. **`frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`** — add to the health check services array:
   ```typescript
   { name: "<service>", url: process.env.SMOKE_<SERVICE>_URL || "http://localhost:<port>" }
   ```

6. **`frontend/e2e/smoke-prod/smoke-health.spec.ts`** — add the service's ingress path to the prod health check endpoints array:
   ```typescript
   "/go-<service>/health"
   ```

7. **`make preflight-compose-config`** — no action needed, runs automatically and validates compose overlay merges

### Add to "Verification Gate" checklist

Add these items to the existing verification gate at the bottom of the skill:

- `[ ] Service added to go/docker-compose.ci.yml with correct env vars`
- `[ ] Service DB added to go/ci-init.sql (if separate DB)`
- `[ ] Migration step added to CI compose-smoke-go job`
- `[ ] Health check added to smoke-go-ci.spec.ts`
- `[ ] Health check added to smoke-health.spec.ts (prod)`
- `[ ] make preflight-compose-config passes`

### Keep existing "Frontend + Smoke Tests" items

The Vercel env var item stays — move it to the existing "Frontend" section or keep inline. The `make preflight-go` item is already covered elsewhere.

## What This Does NOT Change

- The skill stays `disable-model-invocation: true` (static checklist)
- No changes to service code, Dockerfile, K8s, or proto sections
- No changes to the observability checklist
- No new skill files — this is an edit to the existing SKILL.md
