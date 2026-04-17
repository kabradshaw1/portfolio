# Eval Service UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an interactive frontend at `/ai/eval` for managing golden datasets, running RAGAS evaluations, and viewing scorecards — with cross-service JWT auth (Go auth → Python eval).

**Architecture:** Tabbed client-side UI (Datasets / Evaluate / Results) wrapped in HealthGate + GoAuthProvider. The eval service already has JWT auth middleware via `shared.auth`; we modify it to also read from the `access_token` httpOnly cookie (set by the Go auth service). Frontend uses `credentials: "include"` with retry-on-401 pattern from `go-api.ts`.

**Tech Stack:** Next.js, TypeScript, FastAPI, PyJWT (already installed), shadcn/ui, SVG radial gauges

---

## File Structure

### Backend (Python)
| File | Action | Responsibility |
|------|--------|---------------|
| `services/shared/auth.py` | Modify | Add cookie-based JWT extraction alongside Bearer header |
| `services/shared/tests/test_auth.py` | Create | Tests for shared auth (header + cookie + empty secret) |
| `services/eval/app/main.py` | Modify | Add `allow_credentials=True` to CORS config |
| `services/eval/tests/test_main.py` | Modify | Add test for CORS credentials header |

### Frontend
| File | Action | Responsibility |
|------|--------|---------------|
| `frontend/src/lib/eval-api.ts` | Create | API client — all eval service endpoints with auth |
| `frontend/src/app/ai/eval/page.tsx` | Create | Page — HealthGate + GoAuthProvider + tab state + auth gate |
| `frontend/src/components/eval/RadialGauge.tsx` | Create | Reusable SVG radial gauge component |
| `frontend/src/components/eval/DatasetTab.tsx` | Create | Dataset creation form + list |
| `frontend/src/components/eval/EvaluateTab.tsx` | Create | Dataset selector + run button + polling |
| `frontend/src/components/eval/ResultsTab.tsx` | Create | Eval selector + scorecard + per-query breakdown |

### Infrastructure
| File | Action | Responsibility |
|------|--------|---------------|
| `k8s/ai-services/configmaps/eval-config.yml` | Modify | Add `ALLOWED_ORIGINS` for QA domain |
| `docker-compose.yml` | Modify | Add `JWT_SECRET` env var to eval service |

---

## Task 1: Shared Auth — Cookie Support

**Files:**
- Modify: `services/shared/auth.py`
- Create: `services/shared/tests/test_auth.py`

- [ ] **Step 1: Write tests for shared auth**

Create `services/shared/tests/__init__.py` (empty) and `services/shared/tests/test_auth.py`:

```python
import jwt
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from shared.auth import create_auth_dependency

SECRET = "test-secret-key"


def _make_app(secret: str):
    """Create a minimal FastAPI app with auth dependency."""
    app = FastAPI()
    require_auth = create_auth_dependency(secret)

    @app.get("/protected")
    async def protected(user_id: str = Depends(require_auth)):
        return {"user_id": user_id}

    return app


def _make_token(sub: str = "user-123", secret: str = SECRET, exp_offset: int = 3600):
    import time

    return jwt.encode(
        {"sub": sub, "exp": int(time.time()) + exp_offset},
        secret,
        algorithm="HS256",
    )


# --- Empty secret (auth disabled) ---


def test_empty_secret_allows_anonymous():
    app = _make_app("")
    client = TestClient(app)
    res = client.get("/protected")
    assert res.status_code == 200
    assert res.json()["user_id"] == "anonymous"


# --- Bearer header ---


def test_bearer_header_valid():
    app = _make_app(SECRET)
    client = TestClient(app)
    token = _make_token()
    res = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert res.status_code == 200
    assert res.json()["user_id"] == "user-123"


def test_bearer_header_expired():
    app = _make_app(SECRET)
    client = TestClient(app)
    token = _make_token(exp_offset=-10)
    res = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert res.status_code == 401


def test_bearer_header_wrong_secret():
    app = _make_app(SECRET)
    client = TestClient(app)
    token = _make_token(secret="wrong-secret")
    res = client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert res.status_code == 401


# --- Cookie ---


def test_cookie_valid():
    app = _make_app(SECRET)
    client = TestClient(app)
    token = _make_token()
    client.cookies.set("access_token", token)
    res = client.get("/protected")
    assert res.status_code == 200
    assert res.json()["user_id"] == "user-123"


def test_cookie_expired():
    app = _make_app(SECRET)
    client = TestClient(app)
    token = _make_token(exp_offset=-10)
    client.cookies.set("access_token", token)
    res = client.get("/protected")
    assert res.status_code == 401


# --- No auth at all ---


def test_no_auth_returns_401():
    app = _make_app(SECRET)
    client = TestClient(app)
    res = client.get("/protected")
    assert res.status_code == 401


# --- Bearer takes precedence over cookie ---


def test_bearer_takes_precedence_over_cookie():
    app = _make_app(SECRET)
    client = TestClient(app)
    bearer_token = _make_token(sub="bearer-user")
    cookie_token = _make_token(sub="cookie-user")
    client.cookies.set("access_token", cookie_token)
    res = client.get(
        "/protected", headers={"Authorization": f"Bearer {bearer_token}"}
    )
    assert res.status_code == 200
    assert res.json()["user_id"] == "bearer-user"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services && python -m pytest shared/tests/test_auth.py -v`
Expected: Cookie tests fail (cookie reading not implemented yet), header tests should pass.

- [ ] **Step 3: Modify shared auth to support cookies**

Edit `services/shared/auth.py`:

```python
"""JWT authentication dependency for FastAPI services."""

import jwt
from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

_bearer_scheme = HTTPBearer(auto_error=False)


def create_auth_dependency(secret: str):
    """Create a FastAPI dependency that validates JWT Bearer tokens or cookies.

    Checks Authorization header first, then access_token cookie.
    When secret is empty, auth is disabled (all requests pass as anonymous).
    """
    if not secret:

        async def no_auth(
            credentials: HTTPAuthorizationCredentials | None = Depends(
                _bearer_scheme
            ),
        ) -> str:
            return "anonymous"

        return no_auth

    async def require_auth(
        request: Request,
        credentials: HTTPAuthorizationCredentials | None = Depends(
            _bearer_scheme
        ),
    ) -> str:
        """Validate JWT from Bearer header or access_token cookie."""
        token: str | None = None

        # Prefer Bearer header
        if credentials is not None:
            token = credentials.credentials
        else:
            # Fall back to cookie
            token = request.cookies.get("access_token")

        if token is None:
            raise HTTPException(status_code=401, detail="Missing authorization")

        try:
            payload = jwt.decode(
                token,
                secret,
                algorithms=["HS256"],
                options={"require": ["sub", "exp"]},
            )
        except jwt.ExpiredSignatureError:
            raise HTTPException(status_code=401, detail="Token expired")
        except jwt.InvalidTokenError:
            raise HTTPException(status_code=401, detail="Invalid token")

        user_id = payload.get("sub")
        if not user_id:
            raise HTTPException(status_code=401, detail="Invalid token")
        return user_id

    return require_auth
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services && python -m pytest shared/tests/test_auth.py -v`
Expected: All 8 tests pass.

- [ ] **Step 5: Run existing eval tests to verify no regression**

Run: `cd services && python -m pytest eval/tests/ -v`
Expected: All existing tests pass (they use empty secret → anonymous path).

- [ ] **Step 6: Commit**

```bash
git add services/shared/auth.py services/shared/tests/
git commit -m "feat(auth): add cookie-based JWT support to shared auth

Read access_token cookie as fallback when no Bearer header is present.
Bearer header takes precedence. Existing behavior unchanged."
```

---

## Task 2: Eval Service CORS + Infrastructure Config

**Files:**
- Modify: `services/eval/app/main.py:28-34`
- Modify: `docker-compose.yml` (eval service section)
- Modify: `k8s/ai-services/configmaps/eval-config.yml`

- [ ] **Step 1: Add `allow_credentials=True` to CORS**

In `services/eval/app/main.py`, update the CORS middleware block:

```python
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.allowed_origins.split(","),
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)
```

- [ ] **Step 2: Add JWT_SECRET to docker-compose.yml**

In `docker-compose.yml`, add the `JWT_SECRET` env var to the eval service. Find the eval service section and add an `environment` block:

```yaml
  eval:
    image: ghcr.io/kabradshaw1/portfolio/eval:latest
    build:
      context: ./services
      dockerfile: eval/Dockerfile
    env_file: .env
    environment:
      - JWT_SECRET=${JWT_SECRET}
    volumes:
      - eval_data:/app/data
    depends_on:
      chat:
        condition: service_started
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

- [ ] **Step 3: Add QA origin to eval ConfigMap**

In `k8s/ai-services/configmaps/eval-config.yml`, add the QA domain to `ALLOWED_ORIGINS`:

```yaml
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev,https://qa.kylebradshaw.dev
```

- [ ] **Step 4: Run eval tests to confirm nothing broke**

Run: `cd services && python -m pytest eval/tests/ -v`
Expected: All tests pass.

- [ ] **Step 5: Run preflight-python**

Run: `make preflight-python`
Expected: All lint/format/test checks pass.

- [ ] **Step 6: Commit**

```bash
git add services/eval/app/main.py docker-compose.yml k8s/ai-services/configmaps/eval-config.yml
git commit -m "feat(eval): enable CORS credentials and configure JWT_SECRET

Add allow_credentials=True so browser sends httpOnly cookies.
Wire JWT_SECRET in docker-compose and add QA origin to ConfigMap."
```

---

## Task 3: API Client

**Files:**
- Create: `frontend/src/lib/eval-api.ts`

- [ ] **Step 1: Create the eval API client**

Create `frontend/src/lib/eval-api.ts`:

```typescript
import { refreshGoAccessToken } from "@/lib/go-auth";

const EVAL_API_URL =
  process.env.NEXT_PUBLIC_EVAL_API_URL || "http://localhost:8000/eval";

async function evalFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${EVAL_API_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  if (res.status === 401 || res.status === 403) {
    const success = await refreshGoAccessToken();
    if (success) {
      return fetch(`${EVAL_API_URL}${path}`, {
        ...options,
        headers,
        credentials: "include",
      });
    }
  }

  return res;
}

// --- Types ---

export interface GoldenItem {
  query: string;
  expected_answer: string;
  expected_sources: string[];
}

export interface DatasetSummary {
  id: string;
  name: string;
  item_count: number;
  created_at: string;
}

export interface QueryScore {
  faithfulness: number | null;
  answer_relevancy: number | null;
  context_precision: number | null;
  context_recall: number | null;
}

export interface QueryResult {
  query: string;
  answer: string;
  contexts: string[];
  scores: QueryScore;
}

export interface EvaluationSummary {
  id: string;
  dataset_id: string;
  status: "running" | "completed" | "failed";
  collection: string | null;
  aggregate_scores: QueryScore | null;
  created_at: string;
  completed_at: string | null;
}

export interface EvaluationDetail extends EvaluationSummary {
  results: QueryResult[] | null;
  error: string | null;
}

// --- API Functions ---

export async function getHealth(): Promise<boolean> {
  try {
    const res = await fetch(`${EVAL_API_URL}/health`, { signal: AbortSignal.timeout(3000) });
    return res.ok;
  } catch {
    return false;
  }
}

export async function createDataset(
  name: string,
  items: GoldenItem[],
): Promise<{ id: string }> {
  const res = await evalFetch("/datasets", {
    method: "POST",
    body: JSON.stringify({ name, items }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ detail: "Request failed" }));
    throw new Error(err.detail || `HTTP ${res.status}`);
  }
  return res.json();
}

export async function listDatasets(): Promise<DatasetSummary[]> {
  const res = await evalFetch("/datasets");
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  return data.datasets ?? data;
}

export async function startEvaluation(
  datasetId: string,
  collection?: string,
): Promise<{ id: string; status: string }> {
  const body: Record<string, string> = { dataset_id: datasetId };
  if (collection) body.collection = collection;
  const res = await evalFetch("/evaluations", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ detail: "Request failed" }));
    throw new Error(err.detail || `HTTP ${res.status}`);
  }
  return res.json();
}

export async function getEvaluation(id: string): Promise<EvaluationDetail> {
  const res = await evalFetch(`/evaluations/${id}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function listEvaluations(): Promise<EvaluationSummary[]> {
  const res = await evalFetch("/evaluations");
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  return data.evaluations ?? data;
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/eval-api.ts
git commit -m "feat(frontend): add eval service API client

Wraps all eval endpoints with credentials:include for cookie auth
and retry-on-401 pattern matching go-api.ts."
```

---

## Task 4: RadialGauge Component

**Files:**
- Create: `frontend/src/components/eval/RadialGauge.tsx`

- [ ] **Step 1: Create the RadialGauge component**

Create `frontend/src/components/eval/RadialGauge.tsx`:

```tsx
interface RadialGaugeProps {
  value: number | null;
  label: string;
  size?: number;
}

function scoreColor(value: number): string {
  if (value >= 0.7) return "#22c55e"; // green
  if (value >= 0.4) return "#eab308"; // yellow
  return "#ef4444"; // red
}

export function RadialGauge({ value, label, size = 80 }: RadialGaugeProps) {
  const radius = size * 0.4;
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;
  const strokeWidth = size * 0.08;

  const displayValue = value !== null ? value : 0;
  const offset = circumference - displayValue * circumference;
  const color = value !== null ? scoreColor(value) : "#64748b";

  return (
    <div className="flex flex-col items-center gap-1">
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {/* Background track */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke="#1e293b"
          strokeWidth={strokeWidth}
        />
        {/* Score arc */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${center} ${center})`}
        />
        {/* Score text */}
        <text
          x={center}
          y={center + size * 0.05}
          textAnchor="middle"
          fill="white"
          fontSize={size * 0.18}
          fontWeight="bold"
        >
          {value !== null ? value.toFixed(2) : "N/A"}
        </text>
      </svg>
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/eval/RadialGauge.tsx
git commit -m "feat(frontend): add RadialGauge SVG component

Reusable gauge with color thresholds (green/yellow/red) for RAGAS metrics."
```

---

## Task 5: DatasetTab Component

**Files:**
- Create: `frontend/src/components/eval/DatasetTab.tsx`

- [ ] **Step 1: Create the DatasetTab component**

Create `frontend/src/components/eval/DatasetTab.tsx`:

```tsx
"use client";

import { useState, useEffect, useCallback } from "react";
import {
  createDataset,
  listDatasets,
  type DatasetSummary,
  type GoldenItem,
} from "@/lib/eval-api";

const EXAMPLE_ITEMS: GoldenItem[] = [
  {
    query: "What is chunking in document processing?",
    expected_answer:
      "Chunking is the process of splitting documents into smaller pieces for embedding and retrieval.",
    expected_sources: ["ingestion.pdf"],
  },
];

export function DatasetTab() {
  const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
  const [name, setName] = useState("");
  const [itemsJson, setItemsJson] = useState(
    JSON.stringify(EXAMPLE_ITEMS, null, 2),
  );
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const data = await listDatasets();
      setDatasets(data);
    } catch {
      // silently fail on list
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  async function handleCreate() {
    setError(null);
    let items: GoldenItem[];
    try {
      items = JSON.parse(itemsJson);
      if (!Array.isArray(items) || items.length === 0) {
        setError("Items must be a non-empty JSON array.");
        return;
      }
    } catch {
      setError("Invalid JSON. Check syntax and try again.");
      return;
    }

    setCreating(true);
    try {
      await createDataset(name.trim(), items);
      setName("");
      setItemsJson(JSON.stringify(EXAMPLE_ITEMS, null, 2));
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create dataset");
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="space-y-8">
      {/* Create form */}
      <div className="rounded-lg border border-border p-4 space-y-4">
        <h3 className="text-sm font-medium">Create a Golden Dataset</h3>
        <p className="text-xs text-muted-foreground">
          A golden dataset is a set of test queries with expected answers, used
          to measure RAG pipeline quality. Each item has a query, expected
          answer, and optional expected source documents.
        </p>
        <input
          type="text"
          placeholder="Dataset name (e.g., rag-baseline-v1)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        />
        <textarea
          value={itemsJson}
          onChange={(e) => setItemsJson(e.target.value)}
          rows={10}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono"
        />
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          onClick={handleCreate}
          disabled={creating || !name.trim()}
          className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
        >
          {creating ? "Creating..." : "Create Dataset"}
        </button>
      </div>

      {/* Dataset list */}
      <div>
        <h3 className="text-sm font-medium mb-3">Existing Datasets</h3>
        {datasets.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No datasets yet. Create one above to get started.
          </p>
        ) : (
          <div className="space-y-2">
            {datasets.map((ds) => (
              <div key={ds.id} className="rounded-lg border border-border">
                <button
                  onClick={() =>
                    setExpandedId(expandedId === ds.id ? null : ds.id)
                  }
                  className="w-full flex items-center justify-between p-3 text-sm hover:bg-muted/50 transition-colors"
                >
                  <span className="font-medium">{ds.name}</span>
                  <span className="text-muted-foreground text-xs">
                    {new Date(ds.created_at).toLocaleDateString()}
                  </span>
                </button>
                {expandedId === ds.id && (
                  <div className="border-t border-border p-3 text-xs text-muted-foreground">
                    <p>ID: {ds.id}</p>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/eval/DatasetTab.tsx
git commit -m "feat(frontend): add DatasetTab component

Form for creating golden datasets with JSON input and expandable list."
```

---

## Task 6: EvaluateTab Component

**Files:**
- Create: `frontend/src/components/eval/EvaluateTab.tsx`

- [ ] **Step 1: Create the EvaluateTab component**

Create `frontend/src/components/eval/EvaluateTab.tsx`:

```tsx
"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import {
  listDatasets,
  startEvaluation,
  getEvaluation,
  type DatasetSummary,
  type EvaluationDetail,
} from "@/lib/eval-api";

interface EvaluateTabProps {
  onComplete: (evaluation: EvaluationDetail) => void;
}

export function EvaluateTab({ onComplete }: EvaluateTabProps) {
  const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
  const [selectedDatasetId, setSelectedDatasetId] = useState("");
  const [collection, setCollection] = useState("documents");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    listDatasets()
      .then((data) => {
        setDatasets(data);
        if (data.length > 0) setSelectedDatasetId(data[0].id);
      })
      .catch(() => {});
  }, []);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  const pollForCompletion = useCallback(
    (evalId: string) => {
      pollRef.current = setInterval(async () => {
        try {
          const detail = await getEvaluation(evalId);
          if (detail.status === "completed") {
            if (pollRef.current) clearInterval(pollRef.current);
            setRunning(false);
            setStatus(null);
            onComplete(detail);
          } else if (detail.status === "failed") {
            if (pollRef.current) clearInterval(pollRef.current);
            setRunning(false);
            setStatus(null);
            setError(detail.error || "Evaluation failed");
          }
        } catch {
          // keep polling on transient errors
        }
      }, 5000);
    },
    [onComplete],
  );

  async function handleRun() {
    setError(null);
    setRunning(true);
    setStatus("Starting evaluation...");

    try {
      const { id } = await startEvaluation(
        selectedDatasetId,
        collection || undefined,
      );
      setStatus("Evaluating... this may take a few minutes.");
      pollForCompletion(id);
    } catch (e) {
      setRunning(false);
      setStatus(null);
      setError(e instanceof Error ? e.message : "Failed to start evaluation");
    }
  }

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-border p-4 space-y-4">
        <h3 className="text-sm font-medium">Run Evaluation</h3>
        <p className="text-xs text-muted-foreground">
          Select a golden dataset and run RAGAS metrics against the RAG
          pipeline. The evaluation runs in the background — results appear when
          complete.
        </p>

        <div className="space-y-3">
          <div>
            <label className="text-xs text-muted-foreground block mb-1">
              Dataset
            </label>
            <select
              value={selectedDatasetId}
              onChange={(e) => setSelectedDatasetId(e.target.value)}
              disabled={running || datasets.length === 0}
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
            >
              {datasets.length === 0 && (
                <option value="">No datasets available</option>
              )}
              {datasets.map((ds) => (
                <option key={ds.id} value={ds.id}>
                  {ds.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-xs text-muted-foreground block mb-1">
              Collection (optional)
            </label>
            <input
              type="text"
              value={collection}
              onChange={(e) => setCollection(e.target.value)}
              placeholder="documents"
              disabled={running}
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
            />
          </div>
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        {status && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <svg
              className="h-4 w-4 animate-spin"
              viewBox="0 0 24 24"
              fill="none"
            >
              <circle
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
                className="opacity-25"
              />
              <path
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                className="opacity-75"
              />
            </svg>
            {status}
          </div>
        )}

        <button
          onClick={handleRun}
          disabled={running || !selectedDatasetId}
          className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
        >
          {running ? "Running..." : "Run Evaluation"}
        </button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/eval/EvaluateTab.tsx
git commit -m "feat(frontend): add EvaluateTab component

Dataset selector, run button, and polling for async evaluation completion."
```

---

## Task 7: ResultsTab Component

**Files:**
- Create: `frontend/src/components/eval/ResultsTab.tsx`

- [ ] **Step 1: Create the ResultsTab component**

Create `frontend/src/components/eval/ResultsTab.tsx`:

```tsx
"use client";

import { useState, useEffect } from "react";
import {
  listEvaluations,
  getEvaluation,
  type EvaluationSummary,
  type EvaluationDetail,
  type QueryScore,
} from "@/lib/eval-api";
import { RadialGauge } from "@/components/eval/RadialGauge";

interface ResultsTabProps {
  selectedEvaluation: EvaluationDetail | null;
}

function averageScore(scores: QueryScore): number | null {
  const values = [
    scores.faithfulness,
    scores.answer_relevancy,
    scores.context_precision,
    scores.context_recall,
  ].filter((v): v is number => v !== null);
  if (values.length === 0) return null;
  return values.reduce((a, b) => a + b, 0) / values.length;
}

export function ResultsTab({ selectedEvaluation }: ResultsTabProps) {
  const [evaluations, setEvaluations] = useState<EvaluationSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string>("");
  const [detail, setDetail] = useState<EvaluationDetail | null>(
    selectedEvaluation,
  );
  const [expandedQuery, setExpandedQuery] = useState<number | null>(null);

  useEffect(() => {
    listEvaluations()
      .then((data) => {
        setEvaluations(data);
        if (selectedEvaluation) {
          setSelectedId(selectedEvaluation.id);
        } else if (data.length > 0) {
          setSelectedId(data[0].id);
        }
      })
      .catch(() => {});
  }, [selectedEvaluation]);

  useEffect(() => {
    if (selectedEvaluation && selectedId === selectedEvaluation.id) {
      setDetail(selectedEvaluation);
      return;
    }
    if (!selectedId) return;
    getEvaluation(selectedId)
      .then(setDetail)
      .catch(() => setDetail(null));
  }, [selectedId, selectedEvaluation]);

  return (
    <div className="space-y-6">
      {/* Evaluation selector */}
      <div>
        <label className="text-xs text-muted-foreground block mb-1">
          Select Evaluation
        </label>
        <select
          value={selectedId}
          onChange={(e) => setSelectedId(e.target.value)}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        >
          {evaluations.length === 0 && (
            <option value="">No evaluations yet</option>
          )}
          {evaluations.map((ev) => (
            <option key={ev.id} value={ev.id}>
              {new Date(ev.created_at).toLocaleString()} — {ev.status}
            </option>
          ))}
        </select>
      </div>

      {!detail && evaluations.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No evaluation results yet. Go to the Evaluate tab to run one.
        </p>
      )}

      {detail && detail.status === "failed" && (
        <div className="rounded-lg border border-red-800 bg-red-950/50 p-4">
          <p className="text-sm text-red-400">
            Evaluation failed: {detail.error || "Unknown error"}
          </p>
        </div>
      )}

      {detail && detail.status === "running" && (
        <p className="text-sm text-muted-foreground">
          This evaluation is still running. Results will appear when complete.
        </p>
      )}

      {/* Aggregate Scorecard */}
      {detail?.aggregate_scores && detail.status === "completed" && (
        <div className="rounded-lg border border-border p-6">
          <h3 className="text-sm font-medium mb-4">Aggregate Scores</h3>
          <div className="flex justify-center gap-8 flex-wrap">
            <RadialGauge
              value={detail.aggregate_scores.faithfulness}
              label="Faithfulness"
            />
            <RadialGauge
              value={detail.aggregate_scores.answer_relevancy}
              label="Relevancy"
            />
            <RadialGauge
              value={detail.aggregate_scores.context_precision}
              label="Precision"
            />
            <RadialGauge
              value={detail.aggregate_scores.context_recall}
              label="Recall"
            />
          </div>
        </div>
      )}

      {/* Per-query breakdown */}
      {detail?.results && detail.results.length > 0 && (
        <div>
          <h3 className="text-sm font-medium mb-3">Per-Query Breakdown</h3>
          <div className="space-y-2">
            {detail.results.map((result, idx) => {
              const avg = averageScore(result.scores);
              return (
                <div key={idx} className="rounded-lg border border-border">
                  <button
                    onClick={() =>
                      setExpandedQuery(expandedQuery === idx ? null : idx)
                    }
                    className="w-full flex items-center justify-between p-3 text-sm hover:bg-muted/50 transition-colors text-left"
                  >
                    <span className="truncate flex-1 mr-4">
                      {result.query}
                    </span>
                    <span
                      className={`shrink-0 font-mono text-xs ${
                        avg !== null && avg >= 0.7
                          ? "text-green-400"
                          : avg !== null && avg >= 0.4
                            ? "text-yellow-400"
                            : "text-red-400"
                      }`}
                    >
                      {avg !== null ? avg.toFixed(2) : "N/A"}
                    </span>
                  </button>
                  {expandedQuery === idx && (
                    <div className="border-t border-border p-4 space-y-4 text-sm">
                      <div>
                        <span className="text-xs text-muted-foreground uppercase tracking-wide">
                          Answer
                        </span>
                        <p className="mt-1 text-muted-foreground">
                          {result.answer}
                        </p>
                      </div>
                      <div>
                        <span className="text-xs text-muted-foreground uppercase tracking-wide">
                          Retrieved Contexts
                        </span>
                        <ul className="mt-1 space-y-1">
                          {result.contexts.map((ctx, i) => (
                            <li
                              key={i}
                              className="text-xs text-muted-foreground bg-muted/30 rounded p-2"
                            >
                              {ctx}
                            </li>
                          ))}
                        </ul>
                      </div>
                      <div>
                        <span className="text-xs text-muted-foreground uppercase tracking-wide">
                          Scores
                        </span>
                        <div className="mt-1 grid grid-cols-2 gap-2 text-xs">
                          <div>
                            Faithfulness:{" "}
                            <span className="font-mono">
                              {result.scores.faithfulness?.toFixed(2) ?? "N/A"}
                            </span>
                          </div>
                          <div>
                            Relevancy:{" "}
                            <span className="font-mono">
                              {result.scores.answer_relevancy?.toFixed(2) ??
                                "N/A"}
                            </span>
                          </div>
                          <div>
                            Precision:{" "}
                            <span className="font-mono">
                              {result.scores.context_precision?.toFixed(2) ??
                                "N/A"}
                            </span>
                          </div>
                          <div>
                            Recall:{" "}
                            <span className="font-mono">
                              {result.scores.context_recall?.toFixed(2) ??
                                "N/A"}
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/eval/ResultsTab.tsx
git commit -m "feat(frontend): add ResultsTab component

Evaluation selector, radial gauge scorecard, and expandable per-query breakdown."
```

---

## Task 8: Eval Page

**Files:**
- Create: `frontend/src/app/ai/eval/page.tsx`

- [ ] **Step 1: Create the eval page**

Create `frontend/src/app/ai/eval/page.tsx`:

```tsx
"use client";

import { useState } from "react";
import Link from "next/link";
import { HealthGate } from "@/components/HealthGate";
import { GoAuthProvider, useGoAuth } from "@/components/go/GoAuthProvider";
import { DatasetTab } from "@/components/eval/DatasetTab";
import { EvaluateTab } from "@/components/eval/EvaluateTab";
import { ResultsTab } from "@/components/eval/ResultsTab";
import type { EvaluationDetail } from "@/lib/eval-api";

const evalHealthUrl =
  process.env.NEXT_PUBLIC_EVAL_API_URL || "http://localhost:8000/eval";

type Tab = "datasets" | "evaluate" | "results";

function EvalPageInner() {
  const { isLoggedIn } = useGoAuth();
  const [activeTab, setActiveTab] = useState<Tab>("datasets");
  const [completedEval, setCompletedEval] = useState<EvaluationDetail | null>(
    null,
  );

  function handleEvalComplete(evaluation: EvaluationDetail) {
    setCompletedEval(evaluation);
    setActiveTab("results");
  }

  if (!isLoggedIn) {
    return (
      <div className="min-h-screen bg-background text-foreground">
        <div className="mx-auto max-w-3xl px-6 py-12">
          <h1 className="mt-8 text-3xl font-bold">RAG Evaluation</h1>
          <div className="mt-8 rounded-lg border border-border p-8 text-center">
            <p className="text-muted-foreground">
              Log in to use the evaluation tool.
            </p>
            <Link
              href="/go/login?next=/ai/eval"
              className="mt-4 inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 transition-colors"
            >
              Log in
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: "datasets", label: "Datasets" },
    { key: "evaluate", label: "Evaluate" },
    { key: "results", label: "Results" },
  ];

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-12">
        <h1 className="mt-8 text-3xl font-bold">RAG Evaluation</h1>
        <p className="mt-2 text-muted-foreground">
          Measure RAG pipeline quality with golden datasets and RAGAS metrics.
        </p>

        {/* Tabs */}
        <div className="mt-8 flex gap-1 border-b border-border">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.key
                  ? "border-b-2 border-indigo-500 text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div className="mt-6">
          {activeTab === "datasets" && <DatasetTab />}
          {activeTab === "evaluate" && (
            <EvaluateTab onComplete={handleEvalComplete} />
          )}
          {activeTab === "results" && (
            <ResultsTab selectedEvaluation={completedEval} />
          )}
        </div>
      </div>
    </div>
  );
}

export default function EvalPage() {
  return (
    <GoAuthProvider>
      <HealthGate
        endpoint={`${evalHealthUrl}/health`}
        stack="Eval Service"
        docsHref="/ai"
      >
        <EvalPageInner />
      </HealthGate>
    </GoAuthProvider>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Run frontend preflight**

Run: `make preflight-frontend`
Expected: Lint, type check, and build all pass. The `/ai/eval` route should appear in the build output.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/ai/eval/page.tsx
git commit -m "feat(frontend): add /ai/eval page with tabbed eval UI

HealthGate + GoAuthProvider wrapper, three tabs for datasets,
evaluation runner, and results with radial gauge scorecards."
```

---

## Task 9: Add Eval Link to AI Hub Page

**Files:**
- Modify: `frontend/src/app/ai/page.tsx`

- [ ] **Step 1: Read the AI hub page**

Read `frontend/src/app/ai/page.tsx` to find where the existing links to RAG and Debug are.

- [ ] **Step 2: Add eval link following existing pattern**

Add a link to `/ai/eval` in the same section as the RAG and Debug links. Follow the exact same card/link pattern used for the existing entries. The eval link should have:
- Title: "RAG Evaluation"
- Description: "Measure pipeline quality with golden datasets and RAGAS metrics"
- Href: `/ai/eval`

- [ ] **Step 3: Verify frontend builds**

Run: `cd frontend && npx tsc --noEmit && npx next build`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/ai/page.tsx
git commit -m "feat(frontend): add eval link to AI hub page"
```

---

## Task 10: Vercel Env Var + Final Preflight

**Files:**
- No code changes — configuration only

- [ ] **Step 1: Add NEXT_PUBLIC_EVAL_API_URL to Vercel**

```bash
cd frontend
printf 'https://api.kylebradshaw.dev/eval' | vercel env add NEXT_PUBLIC_EVAL_API_URL production
printf 'https://qa-api.kylebradshaw.dev/eval' | vercel env add NEXT_PUBLIC_EVAL_API_URL preview
```

- [ ] **Step 2: Run full frontend preflight**

Run: `make preflight-frontend`
Expected: All checks pass.

- [ ] **Step 3: Run full Python preflight**

Run: `make preflight-python`
Expected: All checks pass.

- [ ] **Step 4: Run security preflight**

Run: `make preflight-security`
Expected: All checks pass.

- [ ] **Step 5: Commit any remaining changes and push**

```bash
git push
```
