# Eval Service UI — Interactive Frontend for RAG Evaluation

## What

New frontend route at `/ai/eval` with a tabbed interface for managing golden datasets, running RAGAS evaluations, and viewing scorecards with per-query breakdowns. Uses the Go auth system for JWT-based authentication, demonstrating cross-service auth propagation (Browser → Python eval service, validated with the same HS256 secret as the Go auth service).

## Why

Makes the eval service usable and demonstrable in the portfolio. Shows interviewers that Kyle builds measurement tooling for RAG pipelines, not just the pipelines themselves. The cross-service auth adds a real-world integration pattern.

## Architecture

- `/ai/eval` page wrapped in `HealthGate` (checks `GET /eval/health`)
- Three client-side tabs: **Datasets**, **Evaluate**, **Results**
- Auth: Go auth service sets an httpOnly `access_token` cookie with `Path: "/"`. Since the eval service is behind the same reverse proxy (nginx in dev, ingress in prod), the browser automatically sends the cookie with `credentials: "include"`. The eval service reads the JWT from the cookie and validates it using the shared `JWT_SECRET`.
- If not logged in (no `go_user` in localStorage), page shows login prompt linking to `/go/login` with `returnTo` param
- On 401 from eval API, attempt a cookie refresh via `refreshGoAccessToken()` (same pattern as `goApiFetch` in `go-api.ts`)

### Service Chain

```
Browser (cookie: access_token=<jwt>)
    → nginx/ingress → eval service (Python/FastAPI)
                          ↓ reads JWT from cookie
                    Go auth service signed it (same HS256 JWT_SECRET)
```

## API Client

New file: `frontend/src/lib/eval-api.ts`

| Function | Method | Path | Auth |
|----------|--------|------|------|
| `getHealth()` | GET | `/health` | No |
| `createDataset(data)` | POST | `/datasets` | Yes |
| `listDatasets()` | GET | `/datasets` | Yes |
| `startEvaluation(datasetId, collection?)` | POST | `/evaluations` | Yes |
| `getEvaluation(id)` | GET | `/evaluations/{id}` | Yes |
| `listEvaluations()` | GET | `/evaluations` | Yes |

Base URL from `NEXT_PUBLIC_EVAL_API_URL` with localhost fallback for dev. All authenticated calls use `credentials: "include"` to send the httpOnly `access_token` cookie. On 401, retry once after calling `refreshGoAccessToken()`.

## Tab Details

### Datasets Tab

- **Create form:** name input + JSON textarea for golden items array (`query`, `expected_answer`, `expected_sources`). "Create Dataset" button.
- **Dataset list:** table showing name, item count, created date. Clicking a row expands to show items preview.
- **Empty state:** guidance text explaining what a golden dataset is and how to format one.

### Evaluate Tab

- **Dataset selector:** dropdown populated from `listDatasets()`
- **Collection input:** optional text input, defaults to "documents"
- **Run button:** "Run Evaluation" triggers `startEvaluation()`, receives 202 with eval ID
- **Polling:** while status is `running`, poll `getEvaluation(id)` every 5 seconds, show spinner with "Evaluating..." badge
- **On completion:** auto-switch to Results tab with the new evaluation selected
- **On failure:** inline error message showing the error string from the API

### Results Tab

- **Evaluation selector:** dropdown of past evaluations (dataset name + date) from `listEvaluations()`
- **Aggregate scorecard:** four radial gauges in a row — faithfulness, answer relevancy, context precision, context recall. Color-coded: green (≥0.7), yellow (0.4–0.7), red (<0.4). Score displayed in center of each gauge.
- **Per-query breakdown:** expandable row table. Default view shows query text + average score per row. Expanding a row shows:
  - Generated answer
  - Retrieved contexts (as a list)
  - Individual metric scores (four values)

## Components

### New Frontend Files

| File | Purpose |
|------|---------|
| `frontend/src/lib/eval-api.ts` | API client for eval service |
| `frontend/src/app/ai/eval/page.tsx` | Page: HealthGate + auth check + tab state |
| `frontend/src/components/eval/DatasetTab.tsx` | Dataset creation form + list |
| `frontend/src/components/eval/EvaluateTab.tsx` | Run form + polling status |
| `frontend/src/components/eval/ResultsTab.tsx` | Eval selector + scorecard + breakdown |
| `frontend/src/components/eval/RadialGauge.tsx` | Reusable SVG radial gauge |

### Backend Changes

| File | Purpose |
|------|---------|
| `services/eval/app/auth.py` | JWT validation middleware (HS256, shared secret) |
| `services/eval/app/main.py` | Wire auth dependency into protected routes (currently has placeholder auth) |

### Infrastructure Changes

- Add `JWT_SECRET` env var to eval service in:
  - `docker-compose.yml`
  - `k8s/ai-services/` manifests
  - K8s QA overlay

## Auth Integration

### Backend (eval service)

New `services/eval/app/auth.py`:
- Read `JWT_SECRET` from environment
- Extract JWT from the `access_token` cookie (matching the Go auth service's cookie name)
- Decode and validate HS256 signature
- Extract user identity (for logging/audit, not for user lookup)
- Return 401 on missing/invalid/expired token
- Wire as a FastAPI dependency on all routes except `/health`

### Frontend

- Check `go_user` in `localStorage` to determine login state (same as Go ecommerce pages)
- If not logged in: show "Log in to use the evaluation tool" with link to `/go/login?returnTo=/ai/eval`
- All eval API calls use `credentials: "include"` so the browser sends the httpOnly `access_token` cookie automatically
- On 401 response: attempt `refreshGoAccessToken()`, retry once. If still 401, dispatch `go-auth-cleared` event and show login prompt.

## Scorecard Visualization

Radial gauges using inline SVG (no charting library needed):
- Circle background track + colored arc proportional to score
- Score value centered inside
- Color thresholds: green ≥0.7, yellow 0.4–0.7, red <0.4
- Metric name below each gauge
- Four gauges in a responsive row

## Dependencies

- No new npm packages (SVG gauges, native fetch, existing shadcn components)
- `PyJWT` package added to eval service requirements (for HS256 token decoding)
- Eval service must be deployed (already done)
- Go auth service must be running (already deployed)
- `JWT_SECRET` must match between Go auth and eval service configs
- Eval service must be behind the same reverse proxy as the Go auth service so the `access_token` cookie (set with `Path: "/"`) is sent on eval API requests

## Out of Scope

- Dataset editing/deletion (datasets are immutable in the current API)
- Evaluation comparison or history trends (Specs 3 and 4)
- File upload for datasets (JSON textarea is sufficient for now)
- SSE/WebSocket for real-time eval progress (polling is adequate)
