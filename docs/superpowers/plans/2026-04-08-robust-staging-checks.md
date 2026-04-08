# Robust Staging Checks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three path-filtered CI jobs that catch infra/integration failures on feature/staging branches so the main deploy stays green.

**Architecture:** All three jobs run inside GitHub Actions runners (no Windows PC changes). Job 1 runs the real Go migration pipeline against a postgres service container. Job 2 validates k8s manifests with `kubeconform` + `kind` server-side dry-run + a homegrown policy script. Job 3 stands up the Python stack via `docker compose` with a mocked Ollama stub and runs a RAG happy-path Playwright smoke test.

**Tech Stack:** GitHub Actions, Docker Compose, kubeconform, kind, golang-migrate, FastAPI (mock-ollama stub), Playwright.

**Reference:** `docs/superpowers/specs/2026-04-08-robust-staging-checks-design.md`

**Branch:** `robust-staging-checks` (already checked out)

---

## Task 1: Add `go-migration-test` CI job

**Files:**
- Modify: `.github/workflows/ci.yml` (add new job after `backend-lint`, before `frontend-lint`)

**Context:** GitHub Actions supports "service containers" — sidecar Docker containers that run alongside the job's runner. We'll use postgres:17-alpine (same tag as `java/k8s/deployments/postgres.yml`). The job mirrors what the prod migration Jobs do: create `ecommercedb`, run auth migrations, run ecommerce migrations + seed.

- [ ] **Step 1: Add the job to ci.yml**

Insert the following job block after the `backend-tests` job definition and before `frontend-lint` (or wherever fits the alphabetical-ish ordering; the exact position doesn't affect behavior). Indent must match sibling jobs (2 spaces under the top-level `jobs:` key).

```yaml
  go-migration-test:
    name: Go Migration Pipeline Test
    runs-on: ubuntu-latest
    needs: [changes]
    if: needs.changes.outputs.go == 'true' || needs.changes.outputs.k8s == 'true'
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_USER: taskuser
          POSTGRES_PASSWORD: taskpass
          POSTGRES_DB: taskdb
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U taskuser -d taskdb"
          --health-interval 5s
          --health-timeout 3s
          --health-retries 10
    env:
      # Intentional: sslmode=disable matches prod exactly. Dropping it here
      # would cause the pq driver to default to sslmode=require and reproduce
      # the 2026-04-08 production deploy failure.
      DATABASE_URL: postgres://taskuser:taskpass@localhost:5432/ecommercedb?sslmode=disable
    steps:
      - uses: actions/checkout@v4

      - name: Install postgresql-client and golang-migrate
        run: |
          sudo apt-get update
          sudo apt-get install -y postgresql-client
          curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xz
          sudo mv migrate /usr/local/bin/migrate
          migrate -version

      - name: Create ecommercedb
        env:
          PGPASSWORD: taskpass
        run: |
          psql -h localhost -U taskuser -d taskdb -c "CREATE DATABASE ecommercedb;"

      - name: Run auth-service migrations
        run: |
          migrate -path go/auth-service/migrations -database "$DATABASE_URL" up

      - name: Run ecommerce-service migrations
        run: |
          migrate -path go/ecommerce-service/migrations -database "$DATABASE_URL" up

      - name: Apply ecommerce seed data
        run: |
          psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f go/ecommerce-service/seed.sql

      - name: Verify tables exist
        run: |
          psql "$DATABASE_URL" -c "\dt" | tee /tmp/tables.txt
          grep -q ' users ' /tmp/tables.txt || (echo "users table missing" && exit 1)
          grep -q ' products ' /tmp/tables.txt || (echo "products table missing" && exit 1)
          grep -q ' orders ' /tmp/tables.txt || (echo "orders table missing" && exit 1)
```

- [ ] **Step 2: Validate YAML locally**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output (valid YAML).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add Go migration pipeline test job

Runs auth-service and ecommerce-service migrations plus seed.sql
against a real postgres service container on every push that touches
go/** or k8s manifests. Reproduces the 2026-04-08 sslmode and Job
ordering failures in CI instead of waiting for a main deploy."
```

---

## Task 2: Create `k8s-policy-check.sh` with tests

**Files:**
- Create: `scripts/k8s-policy-check.sh`
- Create: `scripts/test-k8s-policy-check.sh`

**Context:** A bash script that walks all `k8s/`, `java/k8s/`, `go/k8s/` YAML files and enforces two portfolio-specific rules:
1. Any `Deployment` whose container image contains `postgres`, `mongo`, or `redis` must have a `readinessProbe`.
2. Any `ConfigMap` data key ending in `DATABASE_URL` whose value starts with `postgres://` must contain `sslmode=disable`.

Uses `yq` (v4, Go rewrite). We'll install it in CI, but Kyle also has it locally via `brew install yq` if he wants to test.

- [ ] **Step 1: Write the test script (failing fixtures first)**

Create `scripts/test-k8s-policy-check.sh`:

```bash
#!/usr/bin/env bash
# Tests for k8s-policy-check.sh. Creates temp fixtures with known-bad and
# known-good manifests and asserts the script's exit code and output.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICY="$SCRIPT_DIR/k8s-policy-check.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

# --- Fixture 1: postgres Deployment WITHOUT readinessProbe (should fail) ---
mkdir -p "$TMP/case1"
cat > "$TMP/case1/postgres.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  template:
    spec:
      containers:
        - name: postgres
          image: postgres:17-alpine
EOF
if "$POLICY" "$TMP/case1" >/dev/null 2>&1; then
  fail "case1: postgres without readinessProbe should have failed"
fi
pass "case1: missing postgres readinessProbe is detected"

# --- Fixture 2: postgres Deployment WITH readinessProbe (should pass) ---
mkdir -p "$TMP/case2"
cat > "$TMP/case2/postgres.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  template:
    spec:
      containers:
        - name: postgres
          image: postgres:17-alpine
          readinessProbe:
            exec:
              command: ["pg_isready"]
EOF
if ! "$POLICY" "$TMP/case2" >/dev/null 2>&1; then
  fail "case2: postgres with readinessProbe should have passed"
fi
pass "case2: postgres with readinessProbe passes"

# --- Fixture 3: ConfigMap postgres:// URL WITHOUT sslmode=disable (fail) ---
mkdir -p "$TMP/case3"
cat > "$TMP/case3/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: bad
data:
  DATABASE_URL: postgres://user:pass@host:5432/db
EOF
if "$POLICY" "$TMP/case3" >/dev/null 2>&1; then
  fail "case3: postgres URL without sslmode=disable should have failed"
fi
pass "case3: missing sslmode=disable is detected"

# --- Fixture 4: ConfigMap postgres:// URL WITH sslmode=disable (pass) ---
mkdir -p "$TMP/case4"
cat > "$TMP/case4/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: good
data:
  DATABASE_URL: postgres://user:pass@host:5432/db?sslmode=disable
EOF
if ! "$POLICY" "$TMP/case4" >/dev/null 2>&1; then
  fail "case4: postgres URL with sslmode=disable should have passed"
fi
pass "case4: sslmode=disable URL passes"

# --- Fixture 5: unrelated Deployment without probe (should pass — rule only
# targets postgres/mongo/redis images) ---
mkdir -p "$TMP/case5"
cat > "$TMP/case5/svc.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat
spec:
  template:
    spec:
      containers:
        - name: chat
          image: ghcr.io/example/chat:latest
EOF
if ! "$POLICY" "$TMP/case5" >/dev/null 2>&1; then
  fail "case5: unrelated Deployment without probe should have passed"
fi
pass "case5: non-stateful Deployment without probe is allowed"

echo
echo "All policy check tests passed."
```

```bash
chmod +x scripts/test-k8s-policy-check.sh
```

- [ ] **Step 2: Run the test — it should fail because the policy script doesn't exist yet**

Run: `scripts/test-k8s-policy-check.sh`
Expected: something like `scripts/k8s-policy-check.sh: No such file or directory`.

- [ ] **Step 3: Write the policy script**

Create `scripts/k8s-policy-check.sh`:

```bash
#!/usr/bin/env bash
# k8s-policy-check.sh — enforce portfolio-specific k8s manifest rules.
#
# Rules:
#   R1. Any Deployment whose container image references postgres, mongo, or
#       redis MUST define a readinessProbe on that container. Rationale:
#       without a probe, kubectl rollout status returns before the database
#       is actually accepting connections, causing startup races.
#   R2. Any ConfigMap data key ending in DATABASE_URL whose value starts with
#       postgres:// MUST include sslmode=disable. Rationale: the Go pq driver
#       defaults to sslmode=require against a non-SSL postgres.
#
# Usage: scripts/k8s-policy-check.sh [dir ...]
# Exits 0 on success, 1 on any violation. Prints each violation to stderr.
set -euo pipefail

if ! command -v yq >/dev/null 2>&1; then
  echo "k8s-policy-check.sh: yq is required (v4, Go rewrite)" >&2
  exit 2
fi

DIRS=("$@")
if [ "${#DIRS[@]}" -eq 0 ]; then
  DIRS=(k8s java/k8s go/k8s)
fi

violations=0
report() {
  echo "VIOLATION: $*" >&2
  violations=$((violations + 1))
}

check_file() {
  local file="$1"
  # Handle multi-document YAML files.
  local doc_count
  doc_count=$(yq 'di' "$file" 2>/dev/null | tail -1)
  if [ -z "$doc_count" ]; then
    return 0
  fi

  local i
  for i in $(seq 0 "$doc_count"); do
    local kind
    kind=$(yq "select(di == $i) | .kind // \"\"" "$file")

    if [ "$kind" = "Deployment" ]; then
      # Iterate containers and check images against the stateful-service regex.
      local n_containers
      n_containers=$(yq "select(di == $i) | .spec.template.spec.containers | length" "$file")
      local c
      for c in $(seq 0 $((n_containers - 1))); do
        local image probe
        image=$(yq "select(di == $i) | .spec.template.spec.containers[$c].image // \"\"" "$file")
        if echo "$image" | grep -Eq '(^|/)(postgres|mongo|redis)(:|$)'; then
          probe=$(yq "select(di == $i) | .spec.template.spec.containers[$c].readinessProbe // \"null\"" "$file")
          if [ "$probe" = "null" ]; then
            local name
            name=$(yq "select(di == $i) | .metadata.name" "$file")
            report "$file: Deployment/$name container '$image' is missing readinessProbe (R1)"
          fi
        fi
      done
    fi

    if [ "$kind" = "ConfigMap" ]; then
      # Iterate data keys and check any ending in DATABASE_URL.
      local keys
      keys=$(yq "select(di == $i) | .data // {} | keys | .[]" "$file" 2>/dev/null || true)
      local key
      while IFS= read -r key; do
        [ -z "$key" ] && continue
        case "$key" in
          *DATABASE_URL)
            local value
            value=$(yq "select(di == $i) | .data[\"$key\"]" "$file")
            if echo "$value" | grep -q '^postgres://'; then
              if ! echo "$value" | grep -q 'sslmode=disable'; then
                local name
                name=$(yq "select(di == $i) | .metadata.name" "$file")
                report "$file: ConfigMap/$name key '$key' missing sslmode=disable (R2)"
              fi
            fi
            ;;
        esac
      done <<< "$keys"
    fi
  done
}

for dir in "${DIRS[@]}"; do
  if [ ! -d "$dir" ]; then
    continue
  fi
  while IFS= read -r -d '' file; do
    check_file "$file"
  done < <(find "$dir" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0)
done

if [ "$violations" -gt 0 ]; then
  echo "" >&2
  echo "k8s-policy-check: $violations violation(s) found" >&2
  exit 1
fi

echo "k8s-policy-check: all rules passed"
```

```bash
chmod +x scripts/k8s-policy-check.sh
```

- [ ] **Step 4: Install yq locally if missing**

Run: `command -v yq || brew install yq`
Expected: yq path printed, or Homebrew installs yq.

- [ ] **Step 5: Run the test script — it should pass**

Run: `scripts/test-k8s-policy-check.sh`
Expected: `PASS: case1:` ... `PASS: case5:` ... `All policy check tests passed.`

- [ ] **Step 6: Run the policy check against the real repo — it should pass**

Run: `scripts/k8s-policy-check.sh`
Expected: `k8s-policy-check: all rules passed` (assumes earlier fixes from this session — postgres readiness probe + `sslmode=disable` in ConfigMaps — are committed on this branch's ancestry).

If it fails, the earlier fixes from `main` haven't landed on this branch yet; stop and investigate rather than "fixing" the script.

- [ ] **Step 7: Commit**

```bash
git add scripts/k8s-policy-check.sh scripts/test-k8s-policy-check.sh
git commit -m "feat(scripts): add k8s policy check for readinessProbe and sslmode

Homegrown lint for two portfolio-specific rules:
- Deployments running postgres/mongo/redis must have a readinessProbe.
- ConfigMap DATABASE_URL entries on postgres:// must include sslmode=disable.

Comes with a bash test harness covering 5 fixtures. Both rules derive
from regressions caught during the 2026-04-08 deploy incident."
```

---

## Task 3: Add `k8s-manifest-validation` CI job

**Files:**
- Modify: `.github/workflows/ci.yml`

**Context:** Three-stage job. Stage A is kubeconform (static). Stage B is kind server-side dry-run. Stage C invokes the script from Task 2.

- [ ] **Step 1: Add the job to ci.yml**

Insert after the `go-migration-test` job from Task 1:

```yaml
  k8s-manifest-validation:
    name: K8s Manifest Validation
    runs-on: ubuntu-latest
    needs: [changes]
    if: needs.changes.outputs.k8s == 'true'
    steps:
      - uses: actions/checkout@v4

      - name: Install kubeconform
        run: |
          curl -L -o kubeconform.tar.gz https://github.com/yannh/kubeconform/releases/download/v0.6.7/kubeconform-linux-amd64.tar.gz
          tar xf kubeconform.tar.gz
          sudo mv kubeconform /usr/local/bin/
          kubeconform -v

      - name: Install yq
        run: |
          sudo curl -L -o /usr/local/bin/yq https://github.com/mikefarah/yq/releases/download/v4.44.3/yq_linux_amd64
          sudo chmod +x /usr/local/bin/yq
          yq --version

      # Stage A: static schema validation (fast fail, <5s).
      - name: Stage A — kubeconform
        run: |
          kubeconform -strict -summary -schema-location default \
            -ignore-missing-schemas \
            k8s/ java/k8s/ go/k8s/

      # Stage B: spin up a throwaway kind cluster and run server-side dry-run.
      - name: Stage B — kind cluster (create)
        uses: helm/kind-action@v1
        with:
          cluster_name: manifest-validation
          wait: 60s

      - name: Stage B — create namespaces
        run: |
          for ns in ai-services java-tasks go-ecommerce monitoring; do
            kubectl create namespace "$ns"
          done

      - name: Stage B — server-side dry-run
        run: |
          # Apply every manifest directory recursively. --dry-run=server
          # exercises the real API server's validation and admission.
          # --validate=true is default; we're explicit for clarity.
          for dir in k8s java/k8s go/k8s; do
            find "$dir" -type f \( -name '*.yml' -o -name '*.yaml' \) \
              -print0 | xargs -0 -I{} kubectl apply --dry-run=server -f {}
          done

      # Stage C: portfolio-specific policy rules.
      - name: Stage C — policy check
        run: scripts/k8s-policy-check.sh
```

**Note on `-ignore-missing-schemas`:** kubeconform ships with CRDs it doesn't recognise (e.g. NGINX Ingress custom resources, if any). The flag prevents the run from failing on unknown kinds while still validating everything it does know.

- [ ] **Step 2: Validate YAML locally**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add k8s manifest validation job

Three-stage validation of k8s/, java/k8s/, go/k8s/ manifests:
  A. kubeconform — static OpenAPI schema validation.
  B. kind — server-side --dry-run=server via real API server.
  C. scripts/k8s-policy-check.sh — portfolio-specific rules.

Runs only on k8s path changes. Total runtime ~60s."
```

---

## Task 4: Create `mock-ollama` FastAPI service

**Files:**
- Create: `services/mock-ollama/main.py`
- Create: `services/mock-ollama/requirements.txt`
- Create: `services/mock-ollama/Dockerfile`
- Create: `services/mock-ollama/test_main.py`

**Context:** A minimal FastAPI app that stubs the two Ollama endpoints the Python services call: `/api/embeddings` (used by ingestion + chat for retrieval) and `/api/chat` (used by chat + debug for completions). Returns deterministic, syntactically correct responses so Qdrant and the streaming parsers in the Python services work end-to-end.

The embedding dim is **768** to match `nomic-embed-text` — if the real model and the mock disagree, Qdrant will reject inserts with a dimensionality error.

- [ ] **Step 1: Write the unit test (should fail — no module yet)**

Create `services/mock-ollama/test_main.py`:

```python
from fastapi.testclient import TestClient

from main import app

client = TestClient(app)


def test_embeddings_returns_fixed_768_vector():
    resp = client.post(
        "/api/embeddings",
        json={"model": "nomic-embed-text", "prompt": "hello"},
    )
    assert resp.status_code == 200
    body = resp.json()
    assert "embedding" in body
    assert isinstance(body["embedding"], list)
    assert len(body["embedding"]) == 768
    assert all(isinstance(x, float) for x in body["embedding"])


def test_chat_returns_ndjson_stream():
    resp = client.post(
        "/api/chat",
        json={
            "model": "qwen2.5:14b",
            "messages": [{"role": "user", "content": "hi"}],
            "stream": True,
        },
    )
    assert resp.status_code == 200
    # Response body is newline-delimited JSON objects. Last one has done: true.
    lines = [line for line in resp.text.splitlines() if line.strip()]
    assert len(lines) >= 2
    import json

    first = json.loads(lines[0])
    assert first["message"]["role"] == "assistant"
    assert "content" in first["message"]
    last = json.loads(lines[-1])
    assert last.get("done") is True


def test_tags_endpoint_returns_empty_list():
    # debug/main.py health check calls /api/tags to verify Ollama is up.
    resp = client.get("/api/tags")
    assert resp.status_code == 200
    assert resp.json() == {"models": []}
```

- [ ] **Step 2: Write `requirements.txt`**

```
fastapi==0.115.0
uvicorn[standard]==0.30.6
httpx==0.27.2
```

(`httpx` is a TestClient dependency.)

- [ ] **Step 3: Write `main.py`**

```python
"""Mock Ollama server for CI.

Stubs the three endpoints the Python AI services actually call:
- POST /api/embeddings  — returns a fixed 768-dim vector (nomic-embed-text).
- POST /api/chat        — returns a two-chunk NDJSON stream ending with done.
- GET  /api/tags        — returns an empty model list (used by health check).

The real Ollama response schemas are pinned here verbatim so a future
version bump in Ollama will break these tests loudly instead of drifting
silently. If Ollama's API changes, update both the real client code and
this stub together.
"""

import json

from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

app = FastAPI(title="mock-ollama")

# 768 matches the nomic-embed-text embedding size. Changing this will
# cause Qdrant to reject inserts from ingestion because the collection
# schema pins a fixed vector size.
EMBEDDING_DIM = 768


class EmbeddingsRequest(BaseModel):
    model: str
    prompt: str


@app.post("/api/embeddings")
def embeddings(req: EmbeddingsRequest) -> dict:
    # Deterministic per-prompt vector: simple hash-derived floats in [-1, 1].
    # Not meaningful for retrieval quality, but unique enough that two
    # different prompts get two different vectors.
    seed = sum(ord(c) for c in req.prompt) or 1
    vector = [((seed * (i + 1)) % 2000 - 1000) / 1000.0 for i in range(EMBEDDING_DIM)]
    return {"embedding": vector}


class ChatMessage(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    model: str
    messages: list[ChatMessage]
    stream: bool = True


def _chat_stream() -> bytes:
    # Two content chunks + a terminal done chunk. This exercises the
    # streaming parser in services/chat/app without pulling a real model.
    chunks = [
        {"message": {"role": "assistant", "content": "This is "}, "done": False},
        {"message": {"role": "assistant", "content": "a mock response."}, "done": False},
        {"message": {"role": "assistant", "content": ""}, "done": True},
    ]
    return ("\n".join(json.dumps(c) for c in chunks) + "\n").encode()


@app.post("/api/chat")
def chat(_req: ChatRequest) -> StreamingResponse:
    return StreamingResponse(
        iter([_chat_stream()]),
        media_type="application/x-ndjson",
    )


@app.get("/api/tags")
def tags() -> dict:
    return {"models": []}


@app.get("/health")
def health() -> dict:
    return {"status": "healthy"}
```

- [ ] **Step 4: Run the tests — they should pass**

Run from `services/mock-ollama/`:

```bash
cd services/mock-ollama
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt pytest
pytest test_main.py -v
deactivate
```

Expected: all 3 tests pass.

- [ ] **Step 5: Write `Dockerfile`**

```dockerfile
FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
# Pinned versions; no need for --no-cache-dir cleanup beyond the slim base.
RUN pip install --no-cache-dir -r requirements.txt

COPY main.py .

EXPOSE 11434

# Bind to 0.0.0.0 so other compose services can reach us via the service name.
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "11434"]
```

- [ ] **Step 6: Build the image locally to catch Dockerfile issues**

Run: `docker build -t mock-ollama:test services/mock-ollama/`
Expected: successful build, no warnings fatal to hadolint (the project uses hadolint in CI).

- [ ] **Step 7: Commit**

```bash
git add services/mock-ollama/
git commit -m "feat(mock-ollama): add FastAPI Ollama stub for CI

Mocks /api/embeddings (fixed 768-dim vector), /api/chat (canned
NDJSON stream), and /api/tags for the compose-smoke CI job. Pins
the exact Ollama response schemas so future drift is loud."
```

---

## Task 5: Create `docker-compose.ci.yml` overlay

**Files:**
- Create: `docker-compose.ci.yml`

**Context:** Docker Compose supports overlay files via multiple `-f` arguments. We override `env_file` and environment variables so the CI run doesn't need a real `.env` or real Ollama. The base `docker-compose.yml` references `env_file: .env` for ingestion/chat/debug — that file isn't committed, so we must either create an empty `.env` in CI or null out `env_file` in the overlay. Nulling is cleaner (one less CI step) but compose doesn't always honour `env_file: null`; safest is to create an empty `.env` in the CI step instead. We'll handle that in Task 7; this overlay just sets the overrides.

- [ ] **Step 1: Create the overlay file**

```yaml
# CI overlay: adds a mock Ollama service and points Python AI services at
# it instead of the real Ollama on host.docker.internal. Use with:
#   docker compose -f docker-compose.yml -f docker-compose.ci.yml up -d --build
#
# The empty .env required by the base compose file must be created by the
# caller before running `docker compose up`. The CI job does that explicitly.

services:
  mock-ollama:
    build:
      context: ./services/mock-ollama
    ports:
      - "11434:11434"
    healthcheck:
      test: ["CMD-SHELL", "python3 -c 'import urllib.request; urllib.request.urlopen(\"http://localhost:11434/health\")'"]
      interval: 5s
      timeout: 3s
      retries: 10

  ingestion:
    environment:
      OLLAMA_BASE_URL: http://mock-ollama:11434
    depends_on:
      mock-ollama:
        condition: service_healthy
      qdrant:
        condition: service_healthy

  chat:
    environment:
      OLLAMA_BASE_URL: http://mock-ollama:11434
    depends_on:
      mock-ollama:
        condition: service_healthy
      qdrant:
        condition: service_healthy

  debug:
    environment:
      OLLAMA_BASE_URL: http://mock-ollama:11434
    depends_on:
      mock-ollama:
        condition: service_healthy
      qdrant:
        condition: service_healthy
```

- [ ] **Step 2: Validate the merged compose config**

Run from repo root:

```bash
touch /tmp/empty.env
# Temporarily symlink .env to empty file so validation works.
[ -f .env ] || ln -s /tmp/empty.env .env
docker compose -f docker-compose.yml -f docker-compose.ci.yml config > /dev/null
# Clean up the symlink if we created one.
[ -L .env ] && rm .env
```

Expected: no output (valid merged config).

- [ ] **Step 3: Commit**

```bash
git add docker-compose.ci.yml
git commit -m "feat(compose): add CI overlay with mock-ollama

Overrides OLLAMA_BASE_URL on ingestion/chat/debug to point at the
mock-ollama service introduced in the previous commit. Used by the
compose-smoke CI job."
```

---

## Task 6: Add `smoke-ci` Playwright config + test

**Files:**
- Create: `frontend/playwright.smoke-ci.config.ts`
- Create: `frontend/e2e/smoke-ci.spec.ts`

**Context:** A new Playwright config that targets `localhost:8000` instead of the production API. The test reuses the existing `fixtures/test.pdf` and exercises the RAG happy path end-to-end: upload → ask → assert streamed response contains the mocked content string.

Note: unlike the existing `smoke.spec.ts`, this test does NOT navigate through the frontend page — the frontend isn't running in the compose-smoke job (Next.js dev server isn't part of docker-compose.yml). We hit the backend API directly. If the frontend later joins the compose stack, we can extend this test to also verify the page.

- [ ] **Step 1: Create the Playwright config**

`frontend/playwright.smoke-ci.config.ts`:

```typescript
import { defineConfig } from "@playwright/test";

// Config for the compose-smoke CI job. Targets a local docker-compose stack
// exposed at http://localhost:8000 via the nginx gateway defined in
// docker-compose.yml. Pair with docker-compose.ci.yml which overrides
// OLLAMA_BASE_URL to point at the mock-ollama stub.
export default defineConfig({
  testDir: "./e2e",
  testMatch: "smoke-ci.spec.ts",
  fullyParallel: false,
  retries: 1,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
  },
});
```

- [ ] **Step 2: Create the test spec**

`frontend/e2e/smoke-ci.spec.ts`:

```typescript
import { test, expect } from "@playwright/test";
import path from "path";

// Target the local docker-compose stack rather than production. The compose
// stack uses mock-ollama, so the chat response is a known canned string.
const API_URL = process.env.SMOKE_API_URL || "http://localhost:8000";

// This string must match what services/mock-ollama/main.py emits in
// _chat_stream(). If the mock changes, update both places together.
const EXPECTED_CHAT_SUBSTRING = "mock response";

test.describe("compose-smoke CI tests", () => {
  test("backend health checks pass", async ({ request }) => {
    for (const svc of ["chat", "ingestion", "debug"]) {
      const res = await request.get(`${API_URL}/${svc}/health`);
      expect(res.ok(), `${svc}/health should return 2xx`).toBeTruthy();
    }
  });

  test("RAG happy path: upload → ask → streamed mock response", async ({
    request,
  }) => {
    const collection = "ci-smoke";

    // 1. Upload the fixture PDF to a dedicated collection.
    const pdfPath = path.join(__dirname, "fixtures", "test.pdf");
    const fs = await import("fs");
    const pdfBuffer = fs.readFileSync(pdfPath);

    const uploadRes = await request.post(
      `${API_URL}/ingestion/upload?collection=${collection}`,
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
    expect(uploadRes.ok(), "upload should succeed").toBeTruthy();

    // 2. Ask a question; response is a streamed NDJSON body from chat service.
    const chatRes = await request.post(`${API_URL}/chat/ask`, {
      data: {
        question: "What is this document about?",
        collection,
      },
    });
    expect(chatRes.ok(), "chat/ask should return 2xx").toBeTruthy();

    const body = await chatRes.text();
    expect(
      body.toLowerCase().includes(EXPECTED_CHAT_SUBSTRING),
      `chat response should contain "${EXPECTED_CHAT_SUBSTRING}"; got: ${body.slice(
        0,
        500
      )}`
    ).toBeTruthy();

    // 3. Cleanup: delete the collection so CI runs are idempotent.
    const delRes = await request.delete(
      `${API_URL}/ingestion/collections/${collection}`
    );
    expect(delRes.ok(), "collection delete should succeed").toBeTruthy();
  });
});
```

**Important:** The exact upload/ask/delete endpoint shapes above must match the real Python services. Before committing, verify by grepping for the existing smoke test's endpoint calls and adjusting any mismatched paths/field names. The existing `frontend/e2e/smoke.spec.ts` is the source of truth for the current API contract.

- [ ] **Step 3: Verify endpoint shapes match the real services**

Run: `grep -n "ingestion/upload\|chat/ask\|ingestion/collections" frontend/e2e/smoke.spec.ts`
Expected: confirms the same endpoint names used above. If any endpoint name differs, update `smoke-ci.spec.ts` to match `smoke.spec.ts`. Do not guess — the existing smoke test already works against production and is the authority.

- [ ] **Step 4: Commit**

```bash
git add frontend/playwright.smoke-ci.config.ts frontend/e2e/smoke-ci.spec.ts
git commit -m "test(e2e): add compose-smoke Playwright spec

Runs against localhost:8000 (docker-compose + mock-ollama) instead of
production. Exercises the RAG happy path and asserts the mock chunk
string comes back, confirming the full streaming pipeline works."
```

---

## Task 7: Add `compose-smoke` CI job

**Files:**
- Modify: `.github/workflows/ci.yml`

**Context:** Final new job. Builds and starts the compose stack with the CI overlay, polls health endpoints until ready, runs the smoke spec, dumps logs on failure, and tears down in an always() step.

- [ ] **Step 1: Add the job to ci.yml**

Insert after `k8s-manifest-validation`:

```yaml
  compose-smoke:
    name: Compose Smoke (Python stack)
    runs-on: ubuntu-latest
    needs: [changes]
    if: needs.changes.outputs.python == 'true' || needs.changes.outputs.frontend == 'true'
    steps:
      - uses: actions/checkout@v4

      - name: Create empty .env (required by docker-compose.yml env_file refs)
        run: touch .env

      - name: Build and start compose stack with mock-ollama
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml up -d --build

      - name: Wait for gateway to be reachable
        run: |
          for i in $(seq 1 30); do
            if curl -fsS http://localhost:8000/chat/health >/dev/null 2>&1; then
              echo "gateway ready after ${i}s"
              exit 0
            fi
            sleep 2
          done
          echo "gateway did not come up within 60s" >&2
          exit 1

      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install frontend dependencies
        working-directory: frontend
        run: npm ci

      - name: Install Playwright browsers
        working-directory: frontend
        run: npx playwright install --with-deps chromium

      - name: Run smoke-ci Playwright tests
        working-directory: frontend
        run: npx playwright test --config=playwright.smoke-ci.config.ts

      - name: Dump compose logs on failure
        if: failure()
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml logs --no-color --tail=200

      - name: Tear down compose stack
        if: always()
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml down -v
```

- [ ] **Step 2: Validate YAML locally**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add compose-smoke job with mocked Ollama

Stands up the Python AI stack via docker compose + docker-compose.ci.yml
(which adds mock-ollama), polls /chat/health until ready, and runs the
smoke-ci Playwright spec. Catches startup failures, ingress routing
regressions, Qdrant dimension drift, and streaming parser bugs without
needing a GPU or real Ollama."
```

---

## Task 8: Document the compose↔k8s sync expectation in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Context:** Job 3 only catches regressions if `docker-compose.yml` stays in sync with the k8s manifests for the Python services. A short reminder in CLAUDE.md locks that expectation in so it isn't forgotten.

- [ ] **Step 1: Add the reminder**

Add to the existing CI/CD section of `CLAUDE.md` (immediately after the "Separate CI workflows:" line):

```markdown

**Compose-smoke realism:** Job 3 (`compose-smoke`) runs the Python AI stack via `docker-compose.yml` with a mocked Ollama. Any change to Python service configuration (env vars, ports, depends_on, env_file references) must be reflected in BOTH `docker-compose.yml` and the corresponding k8s manifests under `k8s/ai-services/`, or compose-smoke will drift from prod and stop catching real regressions.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): note compose↔k8s sync requirement for compose-smoke"
```

---

## Final validation

After all tasks are complete, verify the whole plan end-to-end before handing back to Kyle:

- [ ] **Step 1: Re-run the policy check locally**

Run: `scripts/k8s-policy-check.sh && scripts/test-k8s-policy-check.sh`
Expected: both pass.

- [ ] **Step 2: Re-validate the full merged compose config**

Run:
```bash
touch .env
docker compose -f docker-compose.yml -f docker-compose.ci.yml config > /dev/null
rm .env
```
Expected: no output.

- [ ] **Step 3: Re-run mock-ollama unit tests**

Run:
```bash
cd services/mock-ollama
source .venv/bin/activate
pytest test_main.py -v
deactivate
cd ../..
```
Expected: 3 tests pass.

- [ ] **Step 4: Validate the workflow file**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output.

- [ ] **Step 5: Summarise for Kyle**

Print a short status: branch name, commit count, which files touched, and the note that Kyle must push `robust-staging-checks` and watch the new jobs run. Any failures in the new jobs on the first push should be treated as plan bugs, not prod bugs — investigate and fix on the same branch.
