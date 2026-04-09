# Grafana Hardening & Refinement Design

**Date:** 2026-04-09
**Status:** Approved, ready for implementation plan

## Background

Grafana in production (`https://grafana.kylebradshaw.dev`) was broken because the `grafana-secrets` Kubernetes Secret referenced by the Deployment did not exist on the cluster. The Secret file (`k8s/monitoring/secrets/grafana-secrets.yml`) is gitignored, so nothing in the repo or CI guarantees it exists after a fresh deploy. The new ReplicaSet from commit `e7d64eb` (which moved Grafana from a subpath to the `grafana.kylebradshaw.dev` subdomain) had been stuck in `CreateContainerConfigError` for 4+ days with no visible signal, while the old ReplicaSet kept serving with a stale `GF_SERVER_ROOT_URL` that redirected visitors to a 404.

It was manually unblocked by generating a password and creating the Secret. This spec prevents the class of issue from recurring and refines the dashboard for its actual audience (recruiters reviewing a portfolio).

## Goals

1. Make it impossible for Grafana to break again due to the missing-Secret failure mode.
2. Detect Grafana breakage automatically at deploy time.
3. Make the landing experience for a first-time visitor useful with zero clicks.
4. Make the dashboard tell a clearer story about what the portfolio stack actually is.

## Non-Goals (Out of Scope)

- Instrumenting Python / Java / Go services with real Prometheus metrics (currently scraped via `/health`, which returns JSON not Prometheus format). Worth doing later as its own project.
- Prometheus alerting rules.
- Log aggregation (Loki, etc.).
- Any authenticated editing workflow. All dashboard edits happen via code (ConfigMap) and redeploy.
- Scheduled uptime checks outside of deploy time.

## Changes

### 1. Remove the admin Secret entirely

**Files:** `k8s/monitoring/deployments/grafana.yml`, `k8s/monitoring/secrets/grafana-secrets.yml.template`, `.gitignore`, cluster state.

- Delete the `GF_SECURITY_ADMIN_PASSWORD` env var and its `secretKeyRef` from the Deployment.
- Add `GF_AUTH_DISABLE_LOGIN_FORM=true` to remove the login UI.
- Add `GF_USERS_ALLOW_SIGN_UP=false` (defensive).
- Delete `k8s/monitoring/secrets/grafana-secrets.yml.template`.
- Remove any `.gitignore` entry that existed solely for this Secret (leave broader `k8s/**/secrets/*.yml` patterns alone if they cover other services).
- On deploy, delete the existing Secret from the cluster: `kubectl -n monitoring delete secret grafana-secrets`.

**Rationale:** Anonymous Viewer access is the entire point for this portfolio. The provisioned dashboard is owned by a checked-in ConfigMap — there is nothing to edit through a logged-in UI that wouldn't be wiped on the next deploy. Removing the Secret permanently removes the `CreateContainerConfigError` failure mode.

### 2. Default home dashboard

**File:** `k8s/monitoring/deployments/grafana.yml`

Add env var:

```yaml
- name: GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH
  value: /var/lib/grafana/dashboards/system-overview.json
```

**Rationale:** A first-time visitor hitting `https://grafana.kylebradshaw.dev/` currently lands on Grafana's empty default Home dashboard and has to click **Dashboards → system-overview** to see anything. Recruiters won't. This wires the dashboard as the landing page so the URL goes straight to the content.

### 3. "Scrape Targets" panel on the dashboard

**Files:** `k8s/monitoring/configmaps/grafana-dashboards.yml` and `monitoring/grafana/dashboards/system-overview.json` (keep both in sync — the second is used by the Docker Compose path).

Add a new row at the bottom of `system-overview.json` titled **"Scrape Targets"** with a single panel:

- **Type:** Table (or Stat in "All values" mode — Table preferred for readability).
- **Query:** `up` (no aggregation — all series).
- **Legend/column:** `{{job}}`.
- **Value mapping:** `1 → UP (green)`, `0 → DOWN (red)`.
- **Sort:** alphabetical by job name.
- **Description:** "Live health of every service and exporter Prometheus scrapes."

This surfaces all nine existing scrape jobs (`prometheus`, `windows`, `nvidia-gpu`, `qdrant`, `ingestion`, `chat`, `debug`, `gateway-service`, `grafana`) so the dashboard answers "what is this person actually running?" at a glance instead of showing an abstract `9/9` stat tile.

No changes to the existing System, GPU, or Services stat rows.

### 4. Post-deploy smoke test for Grafana

**File:** wherever the existing post-deploy smoke tests live (to be located during implementation — likely `.github/workflows/ci.yml` or a deploy script invoked by it; commit `e7d64eb` added the initial Grafana smoke step).

- Ensure the post-deploy smoke step curls `https://grafana.kylebradshaw.dev/api/health`.
- Assert HTTP 200 and response body contains `"database":"ok"`.
- Fail the deploy / workflow on non-match.

If the step already exists from `e7d64eb`, verify it does both assertions (status and body) and strengthen it if not.

### 5. Rollout

Single feature branch off `main`:

1. Apply all file changes above.
2. `make preflight` for anything that runs over YAML / workflow files.
3. Commit locally. Kyle handles push and merge through the normal feature → staging → main flow.
4. On deploy to the Windows PC:
   - New manifests apply.
   - `kubectl -n monitoring delete secret grafana-secrets` (one-time cleanup).
   - New ReplicaSet rolls, old one terminates.
   - Post-deploy smoke test runs and must pass.
5. Manually verify: hitting `https://grafana.kylebradshaw.dev/` lands directly on the dashboard with all four rows (System, GPU, Services, Scrape Targets), no login prompt.

## Risks

- **Grafana version support for `GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH`:** The Deployment uses `grafana/grafana:latest`. This env var has been supported for many versions, but `:latest` is itself a separate fragility — out of scope here, flagging for a future spec.
- **Dashboard JSON drift between K8s and Docker Compose copies:** Two copies of `system-overview.json` must stay in sync. This is existing repo structure, not introduced by this change. Implementation must touch both.
- **Smoke test requires tunnel to be up:** If the Cloudflare Tunnel or DNS for `grafana.kylebradshaw.dev` is broken, the smoke test will fail. That's correct behavior — it's what we want to catch.
