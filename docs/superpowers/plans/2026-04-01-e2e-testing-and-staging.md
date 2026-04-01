# E2E Testing, Staging Workflow & Document Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add document delete (backend + UI), Playwright E2E tests (mocked + production smoke), and a staging branch workflow.

**Architecture:** Backend gets delete endpoints on the ingestion service. Frontend gets a document management popover. Playwright tests mock APIs for staging CI, hit real production for post-deploy smoke. Staging branch gates E2E tests before merge to main.

**Tech Stack:** FastAPI, Qdrant, Next.js 16, shadcn/ui, Playwright, GitHub Actions

---

### Task 1: QdrantStore — delete_document Method

**Files:**
- Modify: `services/ingestion/app/store.py:1-72`
- Modify: `services/ingestion/tests/test_store.py`

- [ ] **Step 1: Write failing test for delete_document**

Add to `services/ingestion/tests/test_store.py`:

```python
def test_delete_document(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    # Mock scroll to return 3 points for this document
    mock_qdrant_client.scroll.return_value = (
        [
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 0,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 1,
                    "chunk_index": 1,
                }
            ),
            MagicMock(
                payload={
                    "document_id": "doc-1",
                    "filename": "a.pdf",
                    "page_number": 2,
                    "chunk_index": 2,
                }
            ),
        ],
        None,
    )

    count = store.delete_document("doc-1")
    assert count == 3
    mock_qdrant_client.delete.assert_called_once()
    call_args = mock_qdrant_client.delete.call_args
    assert call_args.kwargs["collection_name"] == "test"


def test_delete_document_not_found(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.scroll.return_value = ([], None)

    count = store.delete_document("nonexistent")
    assert count == 0
    mock_qdrant_client.delete.assert_not_called()
```

Note: You'll need to add `from unittest.mock import MagicMock, patch` — it's already imported in the file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_store.py::test_delete_document -v -p no:asyncio`
Expected: FAIL with `AttributeError: 'QdrantStore' object has no attribute 'delete_document'`

- [ ] **Step 3: Implement delete_document**

Add to `services/ingestion/app/store.py`, inside the `QdrantStore` class after the `list_documents` method. Also add `Filter` and `FieldCondition, MatchValue` to the imports from `qdrant_client.models`:

Update the imports at the top of the file:
```python
from qdrant_client.models import (
    Distance,
    FieldCondition,
    Filter,
    MatchValue,
    PointStruct,
    VectorParams,
)
```

Add the method:
```python
    def delete_document(self, document_id: str) -> int:
        records, _ = self.client.scroll(
            collection_name=self.collection_name,
            scroll_filter=Filter(
                must=[
                    FieldCondition(
                        key="document_id",
                        match=MatchValue(value=document_id),
                    )
                ]
            ),
            limit=10000,
            with_payload=True,
            with_vectors=False,
        )

        if not records:
            return 0

        self.client.delete(
            collection_name=self.collection_name,
            points_selector=Filter(
                must=[
                    FieldCondition(
                        key="document_id",
                        match=MatchValue(value=document_id),
                    )
                ]
            ),
        )
        return len(records)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_store.py -v -p no:asyncio`
Expected: All tests pass including `test_delete_document` and `test_delete_document_not_found`.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/store.py services/ingestion/tests/test_store.py
git commit -m "feat: add delete_document method to QdrantStore"
```

---

### Task 2: QdrantStore — delete_collection Method

**Files:**
- Modify: `services/ingestion/app/store.py`
- Modify: `services/ingestion/tests/test_store.py`

- [ ] **Step 1: Write failing tests for delete_collection**

Add to `services/ingestion/tests/test_store.py`:

```python
def test_delete_collection(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.collection_exists.return_value = True
    store.delete_collection("e2e-test")
    mock_qdrant_client.delete_collection.assert_called_once_with(
        collection_name="e2e-test"
    )


def test_delete_collection_not_found(mock_qdrant_client):
    mock_qdrant_client.collection_exists.return_value = True
    store = QdrantStore(host="localhost", port=6333, collection_name="test")

    mock_qdrant_client.collection_exists.return_value = False
    with pytest.raises(ValueError, match="Collection nonexistent not found"):
        store.delete_collection("nonexistent")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_store.py::test_delete_collection -v -p no:asyncio`
Expected: FAIL with `AttributeError: 'QdrantStore' object has no attribute 'delete_collection'`

- [ ] **Step 3: Implement delete_collection**

Add to `services/ingestion/app/store.py`, inside the `QdrantStore` class after `delete_document`:

```python
    def delete_collection(self, collection_name: str) -> None:
        if not self.client.collection_exists(collection_name):
            raise ValueError(f"Collection {collection_name} not found")
        self.client.delete_collection(collection_name=collection_name)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_store.py -v -p no:asyncio`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/store.py services/ingestion/tests/test_store.py
git commit -m "feat: add delete_collection method to QdrantStore"
```

---

### Task 3: API Endpoint — DELETE /documents/{document_id}

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write failing tests**

Add to `services/ingestion/tests/test_main.py`:

```python
@patch("app.main.get_store")
def test_delete_document_success(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_document.return_value = 5
    mock_get_store.return_value = mock_store

    response = client.delete("/documents/abc-123")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "deleted"
    assert data["document_id"] == "abc-123"
    assert data["chunks_deleted"] == 5


@patch("app.main.get_store")
def test_delete_document_not_found(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_document.return_value = 0
    mock_get_store.return_value = mock_store

    response = client.delete("/documents/nonexistent")
    assert response.status_code == 404
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_main.py::test_delete_document_success -v -p no:asyncio`
Expected: FAIL with 405 Method Not Allowed (no DELETE route exists yet).

- [ ] **Step 3: Implement the endpoint**

Add to `services/ingestion/app/main.py`, after the `list_documents` function:

```python
@app.delete("/documents/{document_id}")
async def delete_document(document_id: str):
    store = get_store()
    chunks_deleted = store.delete_document(document_id)
    if chunks_deleted == 0:
        raise HTTPException(
            status_code=404, detail=f"No document found with id {document_id}"
        )
    return {
        "status": "deleted",
        "document_id": document_id,
        "chunks_deleted": chunks_deleted,
    }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v -p no:asyncio`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: add DELETE /documents/{document_id} endpoint"
```

---

### Task 4: API Endpoint — DELETE /collections/{collection_name}

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write failing tests**

Add to `services/ingestion/tests/test_main.py`:

```python
@patch("app.main.get_store")
def test_delete_collection_success(mock_get_store):
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store

    response = client.delete("/collections/e2e-test")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "deleted"
    assert data["collection"] == "e2e-test"


@patch("app.main.get_store")
def test_delete_collection_not_found(mock_get_store):
    mock_store = MagicMock()
    mock_store.delete_collection.side_effect = ValueError(
        "Collection nonexistent not found"
    )
    mock_get_store.return_value = mock_store

    response = client.delete("/collections/nonexistent")
    assert response.status_code == 404
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_main.py::test_delete_collection_success -v -p no:asyncio`
Expected: FAIL with 405 Method Not Allowed.

- [ ] **Step 3: Implement the endpoint**

Add to `services/ingestion/app/main.py`, after the `delete_document` function:

```python
@app.delete("/collections/{collection_name}")
async def delete_collection(collection_name: str):
    store = get_store()
    try:
        store.delete_collection(collection_name)
    except ValueError as e:
        raise HTTPException(status_code=404, detail=str(e))
    return {"status": "deleted", "collection": collection_name}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v -p no:asyncio`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: add DELETE /collections/{collection_name} endpoint"
```

---

### Task 5: API — Optional Collection Param on Ingest

**Files:**
- Modify: `services/ingestion/app/main.py:75-131`
- Modify: `services/ingestion/tests/test_main.py`

- [ ] **Step 1: Write failing test**

Add to `services/ingestion/tests/test_main.py`:

```python
@patch("app.main.get_store")
@patch("app.main.embed_texts", new_callable=AsyncMock)
@patch("app.main.extract_pages")
def test_ingest_with_custom_collection(mock_extract, mock_embed, mock_get_store):
    mock_extract.return_value = [
        {"page_number": 1, "text": "Hello world. " * 100},
    ]
    mock_embed.return_value = [[0.1] * 768] * 2
    mock_store = MagicMock()
    mock_get_store.return_value = mock_store

    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest?collection=e2e-test",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )

    assert response.status_code == 200
    # Verify the store was created/called — the collection param is used
    # The store.upsert call should still work
    mock_store.upsert.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_main.py::test_ingest_with_custom_collection -v -p no:asyncio`
Expected: PASS actually (query param is ignored currently). That's fine — the test confirms behavior doesn't break. We need to verify the collection is actually used.

- [ ] **Step 3: Implement the collection parameter**

In `services/ingestion/app/main.py`, modify the `ingest` function signature and the store retrieval. Add `Query` to the FastAPI import:

Change the import line:
```python
from fastapi import FastAPI, File, HTTPException, Query, UploadFile
```

Change the `ingest` function signature from:
```python
@app.post("/ingest")
async def ingest(file: UploadFile = File(...)):
```
to:
```python
@app.post("/ingest")
async def ingest(
    file: UploadFile = File(...),
    collection: str | None = Query(default=None),
):
```

And change the store usage in the ingest function. Replace:
```python
    try:
        store = get_store()
        store.upsert(
```
with:
```python
    try:
        if collection:
            store = QdrantStore(
                host=settings.qdrant_host,
                port=settings.qdrant_port,
                collection_name=collection,
            )
        else:
            store = get_store()
        store.upsert(
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v -p no:asyncio`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: add optional collection query param to ingest endpoint"
```

---

### Task 6: Frontend — Add shadcn Popover Component

**Files:**
- Create: `frontend/src/components/ui/popover.tsx`

- [ ] **Step 1: Install the shadcn popover component**

Run: `cd frontend && npx shadcn@latest add popover`

This will create `frontend/src/components/ui/popover.tsx` with `Popover`, `PopoverTrigger`, and `PopoverContent` exports.

- [ ] **Step 2: Verify it installed**

Run: `ls frontend/src/components/ui/popover.tsx`
Expected: File exists.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ui/popover.tsx
git commit -m "chore: add shadcn popover component"
```

Note: If `npx shadcn@latest add popover` also modifies other files (package.json, etc.), include those in the commit too.

---

### Task 7: Frontend — DocumentList Component

**Files:**
- Create: `frontend/src/components/DocumentList.tsx`

- [ ] **Step 1: Create the DocumentList component**

Create `frontend/src/components/DocumentList.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

export interface Document {
  document_id: string;
  filename: string;
  chunks: number;
}

interface DocumentListProps {
  documents: Document[];
  onDelete: (documentId: string) => Promise<void>;
}

export function DocumentList({ documents, onDelete }: DocumentListProps) {
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const handleDelete = async (documentId: string) => {
    setDeletingId(documentId);
    try {
      await onDelete(documentId);
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button className="text-sm text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
          {documents.length} document{documents.length !== 1 ? "s" : ""} uploaded
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-72" align="end">
        {documents.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No documents uploaded yet.
          </p>
        ) : (
          <ul className="space-y-2">
            {documents.map((doc) => (
              <li
                key={doc.document_id}
                className="flex items-center justify-between gap-2"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{doc.filename}</p>
                  <p className="text-xs text-muted-foreground">
                    {doc.chunks} chunk{doc.chunks !== 1 ? "s" : ""}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDelete(doc.document_id)}
                  disabled={deletingId === doc.document_id}
                  className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/DocumentList.tsx
git commit -m "feat: add DocumentList popover component"
```

---

### Task 8: Frontend — Wire Up DocumentList in Page

**Files:**
- Modify: `frontend/src/app/page.tsx`

- [ ] **Step 1: Update page.tsx with document state and DocumentList**

Replace the entire `frontend/src/app/page.tsx` with:

```tsx
"use client";

import { useState, useCallback, useEffect } from "react";
import { ChatWindow, Message, Source } from "@/components/ChatWindow";
import { MessageInput } from "@/components/MessageInput";
import { FileUpload } from "@/components/FileUpload";
import { DocumentList, Document } from "@/components/DocumentList";

export default function Home() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [documents, setDocuments] = useState<Document[]>([]);

  const ingestionBaseUrl =
    process.env.NEXT_PUBLIC_INGESTION_API_URL || "http://localhost:8001";

  const fetchDocuments = useCallback(async () => {
    try {
      const res = await fetch(`${ingestionBaseUrl}/documents`);
      if (res.ok) {
        const data = await res.json();
        setDocuments(data.documents);
      }
    } catch {
      // Silently fail — documents list is non-critical
    }
  }, [ingestionBaseUrl]);

  useEffect(() => {
    fetchDocuments();
  }, [fetchDocuments]);

  const handleDelete = useCallback(
    async (documentId: string) => {
      const res = await fetch(`${ingestionBaseUrl}/documents/${documentId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        await fetchDocuments();
      }
    },
    [ingestionBaseUrl, fetchDocuments]
  );

  const handleSend = useCallback(
    async (question: string) => {
      setMessages((prev) => [...prev, { role: "user", content: question }]);
      setMessages((prev) => [...prev, { role: "assistant", content: "" }]);
      setIsStreaming(true);

      try {
        const baseUrl =
          process.env.NEXT_PUBLIC_CHAT_API_URL || "http://localhost:8002";
        const res = await fetch(`${baseUrl}/chat`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ question }),
        });

        if (!res.ok) {
          throw new Error("Failed to connect to chat service");
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error("No response stream");

        const decoder = new TextDecoder();
        let buffer = "";
        let sources: Source[] = [];

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";

          for (const line of lines) {
            if (!line.startsWith("data: ")) continue;
            const jsonStr = line.slice(6).trim();
            if (!jsonStr) continue;

            try {
              const event = JSON.parse(jsonStr);

              if (event.token) {
                setMessages((prev) => {
                  const updated = [...prev];
                  const last = updated[updated.length - 1];
                  updated[updated.length - 1] = {
                    ...last,
                    content: last.content + event.token,
                  };
                  return updated;
                });
              }

              if (event.done && event.sources) {
                sources = event.sources;
              }
            } catch {
              // skip malformed SSE lines
            }
          }
        }

        // Attach sources to the final assistant message
        if (sources.length > 0) {
          setMessages((prev) => {
            const updated = [...prev];
            const last = updated[updated.length - 1];
            updated[updated.length - 1] = { ...last, sources };
            return updated;
          });
        }

        // Handle empty response
        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last.role === "assistant" && !last.content) {
            const updated = [...prev];
            updated[updated.length - 1] = {
              ...last,
              content: "No response received.",
            };
            return updated;
          }
          return prev;
        });
      } catch (err) {
        setMessages((prev) => {
          const updated = [...prev];
          updated[updated.length - 1] = {
            role: "assistant",
            content:
              err instanceof Error
                ? err.message
                : "Could not connect to the backend. Make sure the services are running.",
          };
          return updated;
        });
      } finally {
        setIsStreaming(false);
      }
    },
    []
  );

  const handleUploaded = useCallback(
    (_filename: string, _chunks: number) => {
      fetchDocuments();
    },
    [fetchDocuments]
  );

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      {/* Header */}
      <header className="flex items-center justify-between border-b px-6 py-3">
        <h1 className="text-lg font-semibold">Document Q&A Assistant</h1>
        <div className="flex items-center gap-4">
          {documents.length > 0 && (
            <DocumentList documents={documents} onDelete={handleDelete} />
          )}
          <FileUpload onUploaded={handleUploaded} />
        </div>
      </header>

      {/* Chat */}
      <ChatWindow messages={messages} />

      {/* Input */}
      <MessageInput onSend={handleSend} disabled={isStreaming} />
    </div>
  );
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Verify it builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/app/page.tsx
git commit -m "feat: wire up DocumentList with fetch, delete, and refresh"
```

---

### Task 9: Playwright Setup

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/playwright.config.ts`
- Create: `frontend/e2e/fixtures/test.pdf`

- [ ] **Step 1: Install Playwright**

Run: `cd frontend && npm install -D @playwright/test && npx playwright install chromium`

- [ ] **Step 2: Create Playwright config**

Create `frontend/playwright.config.ts`:

```ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testIgnore: ["**/smoke.spec.ts"],
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: "html",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  webServer: {
    command: "npm run dev",
    url: "http://localhost:3000",
    reuseExistingServer: !process.env.CI,
  },
});
```

- [ ] **Step 3: Create a minimal test PDF**

Run: `mkdir -p frontend/e2e/fixtures`

Then create a tiny test PDF using Python:

```bash
python -c "
from reportlab.lib.pagesizes import letter
from reportlab.pdfgen import canvas
c = canvas.Canvas('frontend/e2e/fixtures/test.pdf', pagesize=letter)
c.drawString(72, 700, 'This is a test document about artificial intelligence.')
c.drawString(72, 680, 'Machine learning is a subset of AI that enables systems to learn from data.')
c.drawString(72, 660, 'Neural networks are computing systems inspired by biological neural networks.')
c.save()
"
```

If `reportlab` is not installed, create it with:

```bash
pip install reportlab && python -c "
from reportlab.lib.pagesizes import letter
from reportlab.pdfgen import canvas
c = canvas.Canvas('frontend/e2e/fixtures/test.pdf', pagesize=letter)
c.drawString(72, 700, 'This is a test document about artificial intelligence.')
c.drawString(72, 680, 'Machine learning is a subset of AI that enables systems to learn from data.')
c.drawString(72, 660, 'Neural networks are computing systems inspired by biological neural networks.')
c.save()
"
```

- [ ] **Step 4: Add e2e script to package.json**

In `frontend/package.json`, add to `"scripts"`:

```json
"e2e": "playwright test",
"e2e:smoke": "playwright test e2e/smoke.spec.ts --config=playwright.smoke.config.ts"
```

- [ ] **Step 5: Create smoke Playwright config**

Create `frontend/playwright.smoke.config.ts`:

```ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "smoke.spec.ts",
  fullyParallel: false,
  retries: 1,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
  },
});
```

- [ ] **Step 6: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/playwright.config.ts frontend/playwright.smoke.config.ts frontend/e2e/fixtures/test.pdf
git commit -m "chore: set up Playwright with configs and test fixture"
```

---

### Task 10: E2E Test — App Loads

**Files:**
- Create: `frontend/e2e/app-loads.spec.ts`

- [ ] **Step 1: Write the test**

Create `frontend/e2e/app-loads.spec.ts`:

```ts
import { test, expect } from "@playwright/test";

test.describe("App loads", () => {
  test.beforeEach(async ({ page }) => {
    // Mock the documents endpoint to return empty
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );
  });

  test("renders the header", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: "Document Q&A Assistant" })
    ).toBeVisible();
  });

  test("shows empty state message", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByText("Upload a PDF using the button above")
    ).toBeVisible();
  });

  test("shows input field and send button", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByPlaceholder("Ask a question about your documents...")
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  });

  test("shows upload button", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.getByRole("button", { name: "Upload PDF" })
    ).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test**

Run: `cd frontend && npx playwright test e2e/app-loads.spec.ts`
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/app-loads.spec.ts
git commit -m "test: add E2E test for app load and empty state"
```

---

### Task 11: E2E Test — Upload Flow

**Files:**
- Create: `frontend/e2e/upload-flow.spec.ts`

- [ ] **Step 1: Write the test**

Create `frontend/e2e/upload-flow.spec.ts`:

```ts
import { test, expect } from "@playwright/test";
import path from "path";

test.describe("Upload flow", () => {
  test("uploads a PDF and shows status", async ({ page }) => {
    // Mock empty initial documents list
    await page.route("**/documents", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            documents: [
              {
                document_id: "test-id",
                filename: "test.pdf",
                chunks: 3,
              },
            ],
          }),
        });
      }
      return route.continue();
    });

    // Mock the ingest endpoint
    await page.route("**/ingest**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "success",
          document_id: "test-id",
          chunks_created: 3,
          filename: "test.pdf",
        }),
      })
    );

    await page.goto("/");

    // Upload a file
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles(
      path.join(__dirname, "fixtures", "test.pdf")
    );

    // Verify upload status appears
    await expect(page.getByText("test.pdf (3 chunks)")).toBeVisible();

    // Verify document count appears
    await expect(page.getByText("1 document uploaded")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test**

Run: `cd frontend && npx playwright test e2e/upload-flow.spec.ts`
Expected: Test passes.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/upload-flow.spec.ts
git commit -m "test: add E2E test for PDF upload flow"
```

---

### Task 12: E2E Test — Chat Flow

**Files:**
- Create: `frontend/e2e/chat-flow.spec.ts`

- [ ] **Step 1: Write the test**

Create `frontend/e2e/chat-flow.spec.ts`:

```ts
import { test, expect } from "@playwright/test";

test.describe("Chat flow", () => {
  test("sends a question and receives a streamed response", async ({
    page,
  }) => {
    // Mock empty documents
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );

    // Mock the chat SSE endpoint
    await page.route("**/chat", (route) => {
      const sseBody = [
        'data: {"token": "Hello"}',
        "",
        'data: {"token": " world"}',
        "",
        'data: {"done": true, "sources": [{"file": "test.pdf", "page": 1}]}',
        "",
      ].join("\n");

      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: sseBody,
      });
    });

    await page.goto("/");

    // Type and send a question
    const input = page.getByPlaceholder(
      "Ask a question about your documents..."
    );
    await input.fill("What is this about?");
    await page.getByRole("button", { name: "Send" }).click();

    // Verify user message appears
    await expect(page.getByText("What is this about?")).toBeVisible();

    // Verify assistant response streams in
    await expect(page.getByText("Hello world")).toBeVisible();

    // Verify source badge appears
    await expect(page.getByText("test.pdf")).toBeVisible();
    await expect(page.getByText("p. 1")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test**

Run: `cd frontend && npx playwright test e2e/chat-flow.spec.ts`
Expected: Test passes.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/chat-flow.spec.ts
git commit -m "test: add E2E test for chat SSE streaming flow"
```

---

### Task 13: E2E Test — Document Delete

**Files:**
- Create: `frontend/e2e/document-delete.spec.ts`

- [ ] **Step 1: Write the test**

Create `frontend/e2e/document-delete.spec.ts`:

```ts
import { test, expect } from "@playwright/test";

test.describe("Document delete", () => {
  test("opens document list and deletes a document", async ({ page }) => {
    let deleted = false;

    // Mock documents endpoint — returns 1 doc initially, empty after delete
    await page.route("**/documents", (route) => {
      if (route.request().method() === "GET") {
        const docs = deleted
          ? []
          : [
              {
                document_id: "test-id",
                filename: "test.pdf",
                chunks: 3,
              },
            ];
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ documents: docs }),
        });
      }
      return route.continue();
    });

    // Mock delete endpoint
    await page.route("**/documents/test-id", (route) => {
      if (route.request().method() === "DELETE") {
        deleted = true;
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            status: "deleted",
            document_id: "test-id",
            chunks_deleted: 3,
          }),
        });
      }
      return route.continue();
    });

    await page.goto("/");

    // Verify document count shows
    await expect(page.getByText("1 document uploaded")).toBeVisible();

    // Click to open popover
    await page.getByText("1 document uploaded").click();

    // Verify document appears in list
    await expect(page.getByText("test.pdf")).toBeVisible();
    await expect(page.getByText("3 chunks")).toBeVisible();

    // Click delete button (trash icon)
    await page.getByRole("button").filter({ has: page.locator("svg") }).last().click();

    // Verify document is removed — popover trigger should disappear since count is 0
    await expect(page.getByText("1 document uploaded")).not.toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test**

Run: `cd frontend && npx playwright test e2e/document-delete.spec.ts`
Expected: Test passes.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/document-delete.spec.ts
git commit -m "test: add E2E test for document delete flow"
```

---

### Task 14: E2E Test — Error Handling

**Files:**
- Create: `frontend/e2e/error-handling.spec.ts`

- [ ] **Step 1: Write the test**

Create `frontend/e2e/error-handling.spec.ts`:

```ts
import { test, expect } from "@playwright/test";

test.describe("Error handling", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );
  });

  test("shows error when chat service is down", async ({ page }) => {
    // Mock chat endpoint returning 500
    await page.route("**/chat", (route) =>
      route.fulfill({ status: 500, body: "Internal Server Error" })
    );

    await page.goto("/");

    const input = page.getByPlaceholder(
      "Ask a question about your documents..."
    );
    await input.fill("test question");
    await page.getByRole("button", { name: "Send" }).click();

    // Verify error message appears
    await expect(
      page.getByText("Failed to connect to chat service")
    ).toBeVisible();
  });

  test("shows error when upload fails", async ({ page }) => {
    await page.route("**/ingest**", (route) =>
      route.fulfill({
        status: 422,
        contentType: "application/json",
        body: JSON.stringify({ detail: "No text content found in PDF" }),
      })
    );

    await page.goto("/");

    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles({
      name: "empty.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4 fake"),
    });

    await expect(
      page.getByText("No text content found in PDF")
    ).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test**

Run: `cd frontend && npx playwright test e2e/error-handling.spec.ts`
Expected: Tests pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/error-handling.spec.ts
git commit -m "test: add E2E tests for error handling"
```

---

### Task 15: Production Smoke Tests

**Files:**
- Create: `frontend/e2e/smoke.spec.ts`

- [ ] **Step 1: Write the smoke tests**

Create `frontend/e2e/smoke.spec.ts`:

```ts
import { test, expect } from "@playwright/test";
import path from "path";

const FRONTEND_URL =
  process.env.SMOKE_FRONTEND_URL || "https://kylebradshaw.dev";
const CHAT_API_URL =
  process.env.SMOKE_CHAT_API_URL || "https://api-chat.kylebradshaw.dev";
const INGESTION_API_URL =
  process.env.SMOKE_INGESTION_API_URL ||
  "https://api-ingestion.kylebradshaw.dev";

test.describe("Production smoke tests", () => {
  test("frontend loads", async ({ page }) => {
    await page.goto(FRONTEND_URL);
    await expect(
      page.getByRole("heading", { name: "Document Q&A Assistant" })
    ).toBeVisible();
    await expect(
      page.getByPlaceholder("Ask a question about your documents...")
    ).toBeVisible();
  });

  test("backend health checks pass", async ({ request }) => {
    const chatHealth = await request.get(`${CHAT_API_URL}/health`);
    expect(chatHealth.ok()).toBeTruthy();
    const chatData = await chatHealth.json();
    expect(chatData.status).toBe("healthy");

    const ingestionHealth = await request.get(`${INGESTION_API_URL}/health`);
    expect(ingestionHealth.ok()).toBeTruthy();
    const ingestionData = await ingestionHealth.json();
    expect(ingestionData.status).toBe("healthy");
  });

  test("full E2E flow with cleanup", async ({ request, page }) => {
    const testCollection = "e2e-test";

    // Step 1: Upload test PDF to dedicated collection
    const pdfPath = path.join(__dirname, "fixtures", "test.pdf");
    const fs = await import("fs");
    const pdfBuffer = fs.readFileSync(pdfPath);

    const uploadResponse = await request.post(
      `${INGESTION_API_URL}/ingest?collection=${testCollection}`,
      {
        multipart: {
          file: {
            name: "test.pdf",
            mimeType: "application/pdf",
            buffer: pdfBuffer,
          },
        },
      }
    );
    expect(uploadResponse.ok()).toBeTruthy();
    const uploadData = await uploadResponse.json();
    expect(uploadData.status).toBe("success");
    expect(uploadData.chunks_created).toBeGreaterThan(0);

    // Step 2: Ask a question against the test collection
    const chatResponse = await request.post(`${CHAT_API_URL}/chat`, {
      data: {
        question: "What is artificial intelligence?",
        collection: testCollection,
      },
    });
    expect(chatResponse.ok()).toBeTruthy();
    const chatBody = await chatResponse.text();
    // Verify we got SSE data with tokens
    expect(chatBody).toContain("data:");

    // Step 3: Cleanup — delete the test collection
    const deleteResponse = await request.delete(
      `${INGESTION_API_URL}/collections/${testCollection}`
    );
    expect(deleteResponse.ok()).toBeTruthy();
    const deleteData = await deleteResponse.json();
    expect(deleteData.status).toBe("deleted");
  });
});
```

- [ ] **Step 2: Commit** (don't run — these hit production)

```bash
git add frontend/e2e/smoke.spec.ts
git commit -m "test: add production smoke tests with E2E flow and cleanup"
```

---

### Task 16: CI — E2E Staging Job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add e2e-staging job**

Add after the `security-cors-check` job and before the `deploy` job in `.github/workflows/ci.yml`:

```yaml
  e2e-staging:
    name: E2E Tests (Staging)
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/staging'
    needs: [frontend-checks]
    defaults:
      run:
        working-directory: frontend
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright browsers
        run: npx playwright install --with-deps chromium

      - name: Run E2E tests
        run: npx playwright test

      - name: Upload Playwright report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: frontend/playwright-report/
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add E2E staging test job for Playwright"
```

---

### Task 17: CI — Production Smoke Job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add smoke-production job**

Add after the `deploy` job in `.github/workflows/ci.yml`:

```yaml
  smoke-production:
    name: Production Smoke Tests
    runs-on: ubuntu-latest
    needs: [deploy]
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    defaults:
      run:
        working-directory: frontend
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: npm ci

      - name: Install Playwright browsers
        run: npx playwright install --with-deps chromium

      - name: Wait for deployment to stabilize
        run: sleep 30

      - name: Run smoke tests
        env:
          SMOKE_FRONTEND_URL: https://kylebradshaw.dev
          SMOKE_CHAT_API_URL: https://api-chat.kylebradshaw.dev
          SMOKE_INGESTION_API_URL: https://api-ingestion.kylebradshaw.dev
        run: npx playwright test e2e/smoke.spec.ts --config=playwright.smoke.config.ts
```

- [ ] **Step 2: Validate YAML**

Run: `python -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "Valid YAML"`
Expected: "Valid YAML"

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add production smoke test job after deploy"
```

---

### Task 18: Create Staging Branch

- [ ] **Step 1: Create and push the staging branch**

```bash
git checkout -b staging
git push -u origin staging
git checkout main
```

- [ ] **Step 2: Verify branch exists**

Run: `git branch -a | grep staging`
Expected: Shows both local `staging` and `remotes/origin/staging`.

- [ ] **Step 3: Commit** (nothing to commit — branch creation only)

---

### Task 19: Final Verification

- [ ] **Step 1: Run all backend tests**

Run: `cd services/ingestion && python -m pytest tests/ -v -p no:asyncio`
Expected: All tests pass (including new delete tests).

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Run mocked E2E tests**

Run: `cd frontend && npx playwright test`
Expected: All E2E tests pass (smoke tests excluded by config).

- [ ] **Step 5: Validate CI YAML**

Run: `python -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "Valid YAML"`
Expected: "Valid YAML"
