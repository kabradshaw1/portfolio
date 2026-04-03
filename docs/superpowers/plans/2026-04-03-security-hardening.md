# Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the application input layer with collection name validation, question length limits, error message sanitization, XSS protection, and CORS method restriction.

**Architecture:** Minimal targeted fixes in existing files. No new modules or shared dependencies. Validation added inline in endpoints (ingestion) and via Pydantic fields (chat). DOMPurify added as a frontend dependency for SVG sanitization.

**Tech Stack:** FastAPI, Pydantic, DOMPurify, pytest

---

## File Map

- Modify: `services/ingestion/app/main.py` — collection validation, error sanitization, CORS methods, add logging
- Modify: `services/chat/app/main.py` — ChatRequest validation, error sanitization, CORS methods, add logging
- Modify: `services/ingestion/tests/test_main.py` — collection validation tests, error message test updates
- Modify: `services/chat/tests/test_main.py` — question length test, collection validation test, error message test updates
- Modify: `frontend/src/components/MermaidDiagram.tsx` — DOMPurify sanitization
- Modify: `frontend/package.json` — add dompurify + @types/dompurify

---

### Task 0: Create feature branch

- [ ] **Step 1: Checkout staging and create feature branch**

```bash
git checkout staging && git pull origin staging
git checkout -b feat/security-hardening
```

---

### Task 1: Ingestion service — collection name validation

**Files:**
- Modify: `services/ingestion/tests/test_main.py`
- Modify: `services/ingestion/app/main.py`

- [ ] **Step 1: Write failing tests for collection name validation**

Add these tests to `services/ingestion/tests/test_main.py` after the existing `test_ingest_with_custom_collection` test:

```python
def test_ingest_rejects_invalid_collection_name():
    pdf_content = b"%PDF-1.4 fake content"
    response = client.post(
        "/ingest?collection=DROP%20TABLE%20users",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]


def test_ingest_rejects_too_long_collection_name():
    pdf_content = b"%PDF-1.4 fake content"
    long_name = "a" * 101
    response = client.post(
        f"/ingest?collection={long_name}",
        files={"file": ("test.pdf", io.BytesIO(pdf_content), "application/pdf")},
    )
    assert response.status_code == 422
    assert "Invalid collection name" in response.json()["detail"]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/ingestion && python -m pytest tests/test_main.py::test_ingest_rejects_invalid_collection_name tests/test_main.py::test_ingest_rejects_too_long_collection_name -v`

Expected: FAIL — no validation exists yet, requests proceed without 422.

- [ ] **Step 3: Add collection name validation to ingestion service**

In `services/ingestion/app/main.py`, add `import re` at the top (after `import uuid`), then add validation at the start of the `ingest` function, right after the function signature and before the PDF extension check.

Add to imports:
```python
import re
```

Add this block as the first validation in the `ingest` function, before the PDF extension check:
```python
    if collection and not re.match(r"^[a-zA-Z0-9_-]{1,100}$", collection):
        raise HTTPException(status_code=422, detail="Invalid collection name")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v`

Expected: ALL tests pass, including the two new ones.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py
git commit -m "feat: add collection name validation to ingestion service"
```

---

### Task 2: Chat service — collection validation + question length limit

**Files:**
- Modify: `services/chat/tests/test_main.py`
- Modify: `services/chat/app/main.py`

- [ ] **Step 1: Write failing tests for ChatRequest validation**

Add these tests to `services/chat/tests/test_main.py` after the existing `test_chat_requires_question` test:

```python
def test_chat_rejects_too_long_question():
    response = client.post(
        "/chat",
        json={"question": "x" * 2001},
    )
    assert response.status_code == 422


def test_chat_rejects_invalid_collection_name():
    response = client.post(
        "/chat",
        json={"question": "What is this?", "collection": "DROP TABLE users"},
    )
    assert response.status_code == 422


def test_chat_accepts_valid_collection_name():
    """Verify valid collection names pass Pydantic validation."""
    with patch("app.main.rag_query") as mock_rag:
        async def fake(**kwargs):
            yield {"done": True, "sources": []}
        mock_rag.return_value = fake()

        response = client.post(
            "/chat",
            json={"question": "Hello", "collection": "my-collection_123"},
        )
        assert response.status_code == 200
```

The `patch` import is already present at the top of the file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/chat && python -m pytest tests/test_main.py::test_chat_rejects_too_long_question tests/test_main.py::test_chat_rejects_invalid_collection_name -v`

Expected: FAIL — no validation exists yet.

- [ ] **Step 3: Add Pydantic validation to ChatRequest**

In `services/chat/app/main.py`, update the import:

Change:
```python
from pydantic import BaseModel
```
To:
```python
from pydantic import BaseModel, Field
```

Change the model:
```python
class ChatRequest(BaseModel):
    question: str
    collection: str | None = None
```
To:
```python
class ChatRequest(BaseModel):
    question: str = Field(max_length=2000)
    collection: str | None = Field(
        default=None, pattern=r"^[a-zA-Z0-9_-]{1,100}$"
    )
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/chat && python -m pytest tests/test_main.py -v`

Expected: ALL tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/chat/app/main.py services/chat/tests/test_main.py
git commit -m "feat: add question length limit and collection name validation to chat service"
```

---

### Task 3: Error message sanitization — both services

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/chat/app/main.py`
- Modify: `services/ingestion/tests/test_main.py`
- Modify: `services/chat/tests/test_main.py`

- [ ] **Step 1: Write test that error messages don't leak details (ingestion)**

Update the existing `test_ingest_returns_503_when_ollama_unreachable` in `services/ingestion/tests/test_main.py` to also verify the error message is generic. Add these assertions at the end of the test:

```python
    assert "Connection refused" not in response.json()["detail"]
    assert response.json()["detail"] == "Embedding service unavailable"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/ingestion && python -m pytest tests/test_main.py::test_ingest_returns_503_when_ollama_unreachable -v`

Expected: FAIL — current message includes exception details.

- [ ] **Step 3: Sanitize error messages in ingestion service**

In `services/ingestion/app/main.py`, add `import logging` at the top (after `import re`) and add logger after imports:

```python
import logging

logger = logging.getLogger(__name__)
```

Change the embedding error handler from:
```python
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        raise HTTPException(
            status_code=503, detail=f"Embedding service unavailable: {e}"
        )
```
To:
```python
    except (httpx.ConnectError, httpx.TimeoutException) as e:
        logger.error("Embedding service error: %s", e)
        raise HTTPException(
            status_code=503, detail="Embedding service unavailable"
        )
```

Change the vector store error handler from:
```python
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Vector store unavailable: {e}")
```
To:
```python
    except Exception as e:
        logger.error("Vector store error: %s", e)
        raise HTTPException(status_code=503, detail="Vector store unavailable")
```

- [ ] **Step 4: Run ingestion tests to verify they pass**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v`

Expected: ALL tests pass.

- [ ] **Step 5: Write test that SSE error messages don't leak details (chat)**

Update the existing `test_chat_returns_error_when_backend_unreachable` in `services/chat/tests/test_main.py`. Add this assertion at the end:

```python
    assert "Connection refused" not in response.text
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd services/chat && python -m pytest tests/test_main.py::test_chat_returns_error_when_backend_unreachable -v`

Expected: FAIL — current SSE message includes exception details.

- [ ] **Step 7: Sanitize error messages in chat service**

In `services/chat/app/main.py`, add `import logging` at the top (after `import json`) and add logger after imports:

```python
import logging

logger = logging.getLogger(__name__)
```

Change the SSE error handlers from:
```python
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            yield {"data": json.dumps({"error": f"Backend service unavailable: {e}"})}
        except Exception as e:
            yield {"data": json.dumps({"error": f"Internal error: {e}"})}
```
To:
```python
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Backend service error: %s", e)
            yield {"data": json.dumps({"error": "Service unavailable"})}
        except Exception as e:
            logger.error("Internal error: %s", e)
            yield {"data": json.dumps({"error": "Internal error"})}
```

- [ ] **Step 8: Run chat tests to verify they pass**

Run: `cd services/chat && python -m pytest tests/test_main.py -v`

Expected: ALL tests pass.

- [ ] **Step 9: Commit**

```bash
git add services/ingestion/app/main.py services/ingestion/tests/test_main.py services/chat/app/main.py services/chat/tests/test_main.py
git commit -m "fix: sanitize error messages to prevent information disclosure"
```

---

### Task 4: DOMPurify for MermaidDiagram

**Files:**
- Modify: `frontend/package.json` (via npm install)
- Modify: `frontend/src/components/MermaidDiagram.tsx`

- [ ] **Step 1: Install DOMPurify**

```bash
cd frontend && npm install dompurify && npm install --save-dev @types/dompurify
```

- [ ] **Step 2: Add DOMPurify sanitization to MermaidDiagram**

Update `frontend/src/components/MermaidDiagram.tsx`. Add the DOMPurify import after the mermaid import:

```typescript
import DOMPurify from "dompurify";
```

Change the line that sets DOM content from the mermaid render output to wrap the svg in `DOMPurify.sanitize(svg)`. The full updated component:

```tsx
"use client";

import { useEffect, useRef } from "react";
import mermaid from "mermaid";
import DOMPurify from "dompurify";

mermaid.initialize({
  startOnLoad: false,
  theme: "dark",
  themeVariables: {
    darkMode: true,
  },
});

export function MermaidDiagram({ chart }: { chart: string }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!ref.current) return;

    const id = `mermaid-${Date.now()}`;
    mermaid.render(id, chart).then(({ svg }) => {
      if (ref.current) {
        ref.current.innerHTML = DOMPurify.sanitize(svg);
      }
    });
  }, [chart]);

  return <div ref={ref} className="flex justify-center overflow-x-auto" />;
}
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/src/components/MermaidDiagram.tsx
git commit -m "fix: sanitize mermaid SVG output with DOMPurify to prevent XSS"
```

---

### Task 5: Restrict CORS allow_methods

**Files:**
- Modify: `services/ingestion/app/main.py`
- Modify: `services/chat/app/main.py`

- [ ] **Step 1: Restrict CORS methods in ingestion service**

In `services/ingestion/app/main.py`, change the CORS middleware `allow_methods` from:
```python
    allow_methods=["*"],
```
To:
```python
    allow_methods=["GET", "POST", "DELETE"],
```

- [ ] **Step 2: Restrict CORS methods in chat service**

In `services/chat/app/main.py`, change the CORS middleware `allow_methods` from:
```python
    allow_methods=["*"],
```
To:
```python
    allow_methods=["GET", "POST"],
```

- [ ] **Step 3: Run all tests for both services**

Run: `cd services/ingestion && python -m pytest tests/test_main.py -v`
Run: `cd services/chat && python -m pytest tests/test_main.py -v`

Expected: ALL tests pass in both services.

- [ ] **Step 4: Commit**

```bash
git add services/ingestion/app/main.py services/chat/app/main.py
git commit -m "fix: restrict CORS allow_methods to explicit method lists"
```

---

### Task 6: Final CI checks

- [ ] **Step 1: Run ruff on all services**

```bash
cd services/ingestion && ruff check . && ruff format --check .
cd services/chat && ruff check . && ruff format --check .
```

Expected: No errors, no formatting issues.

- [ ] **Step 2: Run tsc on frontend**

```bash
cd frontend && npx tsc --noEmit
```

Expected: No errors.

- [ ] **Step 3: Run full test suites**

```bash
cd services/ingestion && python -m pytest tests/ -v
cd services/chat && python -m pytest tests/ -v
```

Expected: ALL tests pass.
