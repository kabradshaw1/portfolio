# RAG Seed Job Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Seed the production and QA RAG vector collections from committed product-catalog PDFs without calling the authenticated/rate-limited ingestion HTTP API.

**Architecture:** Add an ingestion-image CLI module that reuses the existing parser, chunker, embedder, sparse encoder, Qdrant store, and collection metadata writer. Package the small product-catalog PDFs into the ingestion image and run a Kubernetes Job in each environment with the environment's `ingestion-config`.

**Tech Stack:** Python 3.11, FastAPI ingestion package internals, Qdrant client, Ollama embeddings, Kubernetes Job, Kustomize, GitHub Actions.

---

### Task 1: Seed Runner

**Files:**
- Create: `services/ingestion/app/seed_product_catalog.py`
- Test: `services/ingestion/tests/test_seed_product_catalog.py`

- [ ] **Step 1: Write failing tests**

Create tests that verify the runner skips a non-empty collection, seeds all PDFs from a directory through existing ingestion helpers, writes collection metadata, and fails when the collection is still empty after seeding.

- [ ] **Step 2: Run tests to verify red**

Run: `PYTHONPATH=services/ingestion:services/shared pytest services/ingestion/tests/test_seed_product_catalog.py -q`

Expected: import failure for `app.seed_product_catalog`.

- [ ] **Step 3: Implement runner**

Implement `seed_product_catalog.py` with `async seed_product_catalog(pdf_dir, collection_name, replace=False)`, a direct Qdrant point-count check, parser/chunker/embedder/sparse/store reuse, metadata upsert, and an argparse `main()`.

- [ ] **Step 4: Run tests to verify green**

Run: `PYTHONPATH=services/ingestion:services/shared pytest services/ingestion/tests/test_seed_product_catalog.py -q`

Expected: all new seed runner tests pass.

### Task 2: Image Packaging

**Files:**
- Modify: `services/ingestion/Dockerfile`
- Modify: `docker-compose.yml`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Adjust build context**

Change only the ingestion image build context to repo root so the Dockerfile can copy both `services/` code and `docs/product-catalog/*.pdf`.

- [ ] **Step 2: Copy fixtures into image**

Update the ingestion Dockerfile paths for root context and copy `docs/product-catalog/*.pdf` into `/app/seed/product-catalog/`.

- [ ] **Step 3: Verify Dockerfile references**

Run: `docker build -f services/ingestion/Dockerfile . --target final` if a target exists, otherwise `docker build -f services/ingestion/Dockerfile .`.

Expected: build succeeds and the image contains `/app/seed/product-catalog`.

### Task 3: Kubernetes Jobs

**Files:**
- Create: `k8s/ai-services/jobs/seed-product-docs.yml`
- Modify: `k8s/ai-services/kustomization.yaml`
- Modify: `k8s/overlays/qa/kustomization.yaml`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add prod Job**

Create a Job named `seed-product-docs` in `ai-services` using the ingestion image, `ingestion-config`, and command `python -m app.seed_product_catalog --pdf-dir /app/seed/product-catalog`.

- [ ] **Step 2: Patch QA Job**

Patch the Job namespace/config through the QA overlay so it uses `ai-services-qa` and therefore `COLLECTION_NAME=documents_qa` plus the shared Qdrant host from QA config.

- [ ] **Step 3: Wire CI execution**

In QA and prod deploy jobs, delete/apply/wait the seed Job after the ingestion rollout. The Job must fail deploy if seeding fails.

- [ ] **Step 4: Verify manifests**

Run: `kubectl kustomize k8s/ai-services >/tmp/rag-prod.yml` and `kubectl kustomize k8s/overlays/qa >/tmp/rag-qa.yml`.

Expected: both render a `seed-product-docs` Job with the correct namespace and config map reference.

### Task 4: Verification

**Files:**
- All modified files

- [ ] **Step 1: Run targeted tests**

Run: `PYTHONPATH=services/ingestion:services/shared pytest services/ingestion/tests/test_seed_product_catalog.py services/ingestion/tests/test_main.py -q`

- [ ] **Step 2: Run Python preflight**

Run: `make preflight-python`

- [ ] **Step 3: Run security preflight**

Run: `make preflight-security`

- [ ] **Step 4: Commit**

Commit the feature branch after tests pass.
