# Eval Result Persistence Design

## Issue

The eval service already stores datasets and evaluation runs in SQLite, and the
frontend reloads saved runs through `GET /evaluations`. In Kubernetes, however,
the eval deployment mounts `/app/data` with `emptyDir`, so `/app/data/eval.db`
is lost whenever the pod restarts, is redeployed, or is rescheduled.

The Results tab also hides reload failures in a few paths. If a refresh cannot
load saved evaluations, the UI can look like there are no saved runs even when
the real problem is an API, auth, or service error.

## Goals

- Preserve eval datasets and completed evaluation results across eval pod
  restarts in QA and production Kubernetes.
- Keep the current SQLite persistence model and `DB_PATH=/app/data/eval.db`.
- Add visible, actionable frontend errors when saved evaluations or selected
  evaluation details fail to load.
- Add a non-blocking warning when Evaluate tab baseline history cannot load.
- Keep Docker Compose behavior unchanged because it already uses a named volume.
- Add focused tests for the frontend error states and Kubernetes manifest
  change.

## Non-Goals

- Do not migrate eval persistence to Postgres.
- Do not add browser `localStorage`, IndexedDB, or other client-side result
  caches.
- Do not redesign the eval dashboard or result cards.
- Do not add retention, backup, export, or database administration workflows.
- Do not change the eval database schema or API response contracts.

## Recommended Approach

Use a Kubernetes PersistentVolumeClaim for eval SQLite data and make frontend
load failures visible instead of silently swallowing them.

This fixes the actual durability gap with the smallest operational change:
SQLite remains the source of truth, the container keeps writing to the same
path, and Kubernetes supplies durable storage for that path. The frontend
changes address the separate refresh ambiguity by distinguishing "no saved
runs" from "could not load saved runs."

## Alternatives Considered

### Move Eval Persistence To Postgres

Postgres would improve concurrency and operational tooling, but it would add a
larger migration surface: schemas, migrations, connection management, backup
expectations, and another shared dependency for a small eval service. This is
not justified for the immediate durability problem.

### Client-Side Result Cache

Caching completed runs in the browser could make a refresh appear to work in
some cases, but it would not survive pod restarts for other users or devices.
It also risks showing stale results when the backend is unavailable. The
backend must remain the source of truth.

### Keep `emptyDir` And Document The Limitation

Documenting the limitation would explain the behavior but leave the eval tool
unreliable during normal deploys. That does not meet the project quality bar.

## Storage And Deployment

Add a new Kubernetes PVC for eval data:

- Create `k8s/ai-services/pvc/eval-data.yml`.
- Include the PVC in `k8s/ai-services/kustomization.yaml`.
- Update `k8s/ai-services/deployments/eval.yml` so the `eval-data` volume uses:

```yaml
persistentVolumeClaim:
  claimName: eval-data
```

The existing mount stays unchanged:

```yaml
mountPath: /app/data
```

The existing eval config stays unchanged:

```yaml
DB_PATH: /app/data/eval.db
```

The PVC should match the local Minikube-style storage pattern already used by
the AI services. A small request is sufficient because eval results are JSON
rows in SQLite; start with `1Gi` unless the existing PVC convention in this
directory uses another default.

Docker Compose already mounts the named `eval_data` volume at `/app/data`, so
no Compose change is required.

## Frontend Behavior

### Results Tab

`ResultsTab` should distinguish empty state from load failure:

- If `listEvaluations()` fails on mount, show a compact error message near the
  selector area: `Could not load saved evaluations. Refresh or sign in again.`
- If there are no saved runs and no error, keep the current empty state.
- If the run list loads but `getEvaluation(selectedId)` fails, keep the
  selector visible and show: `Could not load the selected evaluation. Try
  selecting it again or refresh.`
- Do not keep stale detail visible after a selected-detail load failure.
- Keep the existing behavior where a freshly completed run passed through props
  is displayed immediately.

### Evaluate Tab

The baseline selector should surface history load failures without blocking a
new evaluation:

- If `getHistory(selectedDatasetId, collection)` fails, clear the baseline list
  and show a warning near the baseline selector:
  `Could not load baseline runs for this dataset and collection.`
- Keep the Run button usable with no baseline selected.
- Clear the warning after a later successful history load.

### Dashboard Tab

`DashboardTab` already has an error path for history loading. The implementation
should verify that the error is visible and specific enough for users to
understand that saved history failed to load. If it is already visible and
clear, no redesign is required.

## Data Flow

1. A user creates a dataset or starts an evaluation through the frontend.
2. The eval API writes the dataset or evaluation row to SQLite at
   `/app/data/eval.db`.
3. Kubernetes stores `/app/data` on the `eval-data` PVC.
4. On browser refresh, the Results tab calls `GET /evaluations` and then
   `GET /evaluations/{id}` for the selected run.
5. On eval pod restart, the replacement pod mounts the same PVC and opens the
   existing SQLite database.

## Error Handling

Backend persistence errors should continue to surface through existing API
failure behavior. This design does not add a new backend health mode.

Frontend load errors should be visible and scoped to the failing operation.
They should not replace the whole page, and they should not imply data loss
unless the API actually returns an empty list successfully.

## Test Plan

### Frontend

Update the mocked `/ai/eval` Playwright coverage or the nearest existing
frontend tests to cover:

- Results tab shows the saved-evaluations load error when
  `GET /eval/evaluations` returns a failing response.
- Results tab shows the selected-detail error when
  `GET /eval/evaluations/{id}` fails after the list loads.
- Evaluate tab shows a non-blocking baseline-history warning when
  `GET /eval/evaluations/history` fails.
- The Run button remains enabled when baseline history fails but a dataset is
  selected.

### Kubernetes

Add or update a manifest-focused test/preflight if one already exists for
Kubernetes resources. At minimum, verify locally that:

- `kustomize build k8s/ai-services` succeeds.
- The rendered eval deployment references `persistentVolumeClaim` with
  `claimName: eval-data`.
- The rendered resources include a PVC named `eval-data`.

### Manual QA

After deployment to QA:

1. Run an eval and wait for completion.
2. Refresh `/ai/eval`, open Results, and confirm the completed run loads.
3. Restart the eval pod.
4. Refresh `/ai/eval` again and confirm the same completed run still loads.

## Verification

Before committing implementation changes, run:

```bash
make preflight-frontend
make preflight-e2e
kustomize build k8s/ai-services
```

If implementation touches Python eval service code, also run:

```bash
make preflight-python
make preflight-security
```
