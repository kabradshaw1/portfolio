# E2E Testing, Staging Workflow & Document Delete — Design Spec

## Goal

Add Playwright E2E tests, a staging branch workflow, post-deploy production smoke tests, and document delete functionality (backend + frontend) to catch regressions before and after production deploys.

## Scope

- Document delete endpoints on the ingestion service (document-level + collection-level)
- Document management UI (popover in header with delete buttons)
- Playwright mocked E2E tests (staging CI)
- Playwright production smoke tests (post-deploy CI)
- Staging branch workflow
- No changes to existing backend logic, unit tests, or lesson notebooks

## 1. Backend — Delete Endpoints

### 1.1 Document-Level Delete

**Endpoint:** `DELETE /documents/{document_id}`

Deletes all Qdrant points whose payload `document_id` matches the path parameter.

**Response (200):**
```json
{"status": "deleted", "document_id": "abc-123", "chunks_deleted": 5}
```

**Response (404):**
```json
{"detail": "No document found with id abc-123"}
```

**Implementation:** Add `delete_document(document_id: str) -> int` method to `QdrantStore` in `services/ingestion/app/store.py`. Uses Qdrant's `delete` with a `Filter` matching `document_id` in payload. Returns count of deleted points.

### 1.2 Collection-Level Delete

**Endpoint:** `DELETE /collections/{collection_name}`

Deletes an entire Qdrant collection. Intended for E2E test cleanup.

**Response (200):**
```json
{"status": "deleted", "collection": "e2e-test"}
```

**Response (404):**
```json
{"detail": "Collection e2e-test not found"}
```

**Implementation:** Add `delete_collection(collection_name: str)` method to `QdrantStore`. Uses Qdrant's `delete_collection`. Checks `collection_exists` first for the 404 case. This method operates on an arbitrary collection name (not just `self.collection_name`) because E2E tests use a dedicated test collection.

### 1.3 Backend Tests

Unit tests for both endpoints and both store methods:
- `test_delete_document_success` — mock store returns chunk count, verify 200 response
- `test_delete_document_not_found` — mock store returns 0, verify 404
- `test_delete_collection_success` — mock store succeeds, verify 200
- `test_delete_collection_not_found` — mock store raises (collection doesn't exist), verify 404

Unit tests for store methods:
- `test_store_delete_document` — verify Qdrant client `delete` called with correct filter
- `test_store_delete_collection` — verify Qdrant client `delete_collection` called

## 2. Frontend — Document Management Dropdown

### 2.1 DocumentList Component

New component: `frontend/src/components/DocumentList.tsx`

A popover triggered by clicking the document count in the header. Contains:
- List of documents (filename, chunk count, delete button with trash icon)
- Empty state: "No documents uploaded yet."
- Delete button calls `DELETE /documents/{document_id}` on the ingestion API

### 2.2 Page Integration

Modify `frontend/src/app/page.tsx`:
- Add state for documents list (`Document[]`)
- Fetch `GET /documents` from the ingestion API on mount
- Refresh document list after upload (in `handleUploaded`) and after delete
- Pass documents list and delete handler to `DocumentList`
- Replace the plain document count text with the `DocumentList` popover trigger

### 2.3 UI Behavior

- Popover opens on click, closes on click outside or after successful delete
- Delete shows a brief loading state on the button
- After delete, list refreshes and document count updates
- No confirmation dialog — the action is reversible (re-upload the PDF)

## 3. Playwright — Mocked E2E Tests (Staging)

### 3.1 Setup

- Install Playwright as a dev dependency in `frontend/`
- `playwright.config.ts` in `frontend/` — starts `npm run dev` as the web server, tests run against `localhost:3000`
- All API calls intercepted via `page.route()` with mock responses

### 3.2 Test Cases

All tests in `frontend/e2e/`:

**`app-loads.spec.ts`** — Page renders with header ("Document Q&A Assistant"), empty state message, input field, upload button visible.

**`upload-flow.spec.ts`** — Mock `POST /ingest` to return success. Click upload button, select file, verify status updates ("Uploading..."), verify document count increments, verify document appears in list.

**`chat-flow.spec.ts`** — Mock `POST /chat` to return an SSE stream with tokens + sources. Type a question, submit, verify user message appears, assistant response streams in, source badges render with filename and page.

**`document-delete.spec.ts`** — Mock `GET /documents` to return a list. Open document popover, verify documents shown. Mock `DELETE /documents/{id}` to return success. Click delete, verify document removed from list, count decrements.

**`error-handling.spec.ts`** — Mock API to return 500 or network error. Verify user sees appropriate error messages for both upload failure and chat failure.

### 3.3 Mock Strategy

Each test file sets up its own `page.route()` interceptors before navigating. Mocked responses match the real API contract:
- `POST /ingest` → `{"status": "success", "document_id": "test-id", "chunks_created": 3, "filename": "test.pdf"}`
- `POST /chat` → SSE stream: `data: {"token": "Hello"}`, `data: {"token": " world"}`, `data: {"done": true, "sources": [{"file": "test.pdf", "page": 1}]}`
- `GET /documents` → `{"documents": [{"document_id": "test-id", "filename": "test.pdf", "chunks": 3}]}`
- `DELETE /documents/{id}` → `{"status": "deleted", "document_id": "test-id", "chunks_deleted": 3}`

## 4. Playwright — Production Smoke Tests (Post-Deploy)

### 4.1 Test Cases

Separate test file: `frontend/e2e/smoke.spec.ts`

Configured via environment variables for production URLs:
- `SMOKE_FRONTEND_URL` = `https://kylebradshaw.dev`
- `SMOKE_CHAT_API_URL` = `https://api-chat.kylebradshaw.dev`
- `SMOKE_INGESTION_API_URL` = `https://api-ingestion.kylebradshaw.dev`

**Test 1: Frontend loads** — Navigate to frontend URL, verify page renders, header visible, input field present.

**Test 2: Backend health** — Fetch `/health` from both API URLs, verify both return `{"status": "healthy"}`.

**Test 3: Full E2E flow with cleanup** — Upload `frontend/e2e/fixtures/test.pdf` to a collection named `e2e-test` (passed as query param or request body field), ask a question against that collection, verify streaming response contains tokens, delete the `e2e-test` collection via `DELETE /collections/e2e-test`.

### 4.2 Test PDF

A tiny 1-page PDF committed at `frontend/e2e/fixtures/test.pdf`. Content: a short paragraph about a known topic so the LLM response is predictable enough to verify. File size < 10KB.

### 4.3 Collection Isolation

The smoke test uses a dedicated collection name (`e2e-test`) to avoid polluting production data. The chat endpoint already accepts an optional `collection` field in the request body. The ingest endpoint currently uses the default collection — the smoke test will need to pass a collection name. This requires a small addition to the ingest endpoint: an optional `collection` query parameter that overrides the default collection name.

## 5. Staging Branch Workflow

### 5.1 Branching Model

- `main` — production. Pushes trigger deploy + smoke tests.
- `staging` — integration branch. Pushes trigger mocked E2E tests.
- `feat/*`, `fix/*` — feature branches merged into `staging` by you.

### 5.2 CI Workflow Changes

**New job: `e2e-staging`**
- Trigger: push to `staging` branch only
- Runs: `npx playwright install --with-deps chromium`, then `npx playwright test` (excluding `smoke.spec.ts`)
- Requires: `frontend-checks` to pass first

**New job: `smoke-production`**
- Trigger: runs after `deploy` job succeeds (on `main` branch only)
- Runs: Playwright with `smoke.spec.ts` only, pointed at production URLs
- Uses Tailscale (already configured in deploy) if needed for API access
- Environment variables set for production URLs

**Existing jobs unchanged** — lint, unit tests, security scans, docker build all continue to run on all branches.

### 5.3 Developer Workflow

1. Create feature branch from `staging`
2. Make changes, push, CI runs lint + tests + security
3. Merge feature branch into `staging`
4. CI runs all checks + mocked E2E tests on `staging`
5. Review results — if all pass, merge `staging` into `main`
6. CI deploys to production, runs smoke tests
7. GitHub notifies if smoke tests fail

## 6. Ingest Endpoint Change

The `POST /ingest` endpoint needs an optional `collection` query parameter to support E2E test collection isolation:

```
POST /ingest?collection=e2e-test
```

When provided, chunks are stored in the specified collection instead of the default. When omitted, behavior is unchanged (uses `settings.collection_name`). This is a non-breaking addition.

## Files Changed

| File | Change |
|------|--------|
| `services/ingestion/app/store.py` | Add `delete_document()`, `delete_collection()` methods |
| `services/ingestion/app/main.py` | Add `DELETE /documents/{id}`, `DELETE /collections/{name}` endpoints, optional `collection` param on ingest |
| `services/ingestion/tests/test_main.py` | Tests for new endpoints |
| `services/ingestion/tests/test_store.py` | Tests for new store methods |
| `frontend/src/components/DocumentList.tsx` | New component — document management popover |
| `frontend/src/app/page.tsx` | Wire up document list state, fetch, delete handler |
| `frontend/package.json` | Add `@playwright/test` dev dependency |
| `frontend/playwright.config.ts` | Playwright configuration |
| `frontend/e2e/app-loads.spec.ts` | Page load test |
| `frontend/e2e/upload-flow.spec.ts` | Upload flow test |
| `frontend/e2e/chat-flow.spec.ts` | Chat flow test |
| `frontend/e2e/document-delete.spec.ts` | Document delete test |
| `frontend/e2e/error-handling.spec.ts` | Error handling test |
| `frontend/e2e/smoke.spec.ts` | Production smoke tests |
| `frontend/e2e/fixtures/test.pdf` | Small test PDF for smoke tests |
| `.github/workflows/ci.yml` | Add `e2e-staging` and `smoke-production` jobs |

## Files NOT Changed

- Existing backend unit tests (except adding new ones)
- Chat service (already supports `collection` field)
- Lesson notebooks
- Existing CI jobs
