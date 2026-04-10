# Plan 5 — `go/ai-service` Deployment, CI, and Documentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Ship `go/ai-service` to production on Minikube, integrate it into the existing CI pipeline, and land the two documentation artifacts that explain the work (ADR notebook + rag-reevaluation ADR). Also fix one small latent bug in the frontend SSE parser that the Plan 4 E2E surfaced.

**Architecture:** K8s manifests join the existing `go-ecommerce` namespace using the same `Deployment` + `Service` + `ConfigMap` + `Secret` + `Ingress` pattern as `auth-service`/`ecommerce-service`. The ingress gets one new path `/ai-api(/|$)(.*)` routed to `go-ai-service:8093`. Ollama and cross-service calls go via cross-namespace DNS (`ollama.ai-services.svc.cluster.local`, `go-ecommerce-service.go-ecommerce.svc.cluster.local` — though the latter can just be `go-ecommerce-service` when both pods are in the same namespace). `.github/workflows/go-ci.yml`'s matrix strategy adds `ai-service` to lint/test/build; deploy happens automatically via the existing `ci.yml` deploy job, which already applies every YAML under `go/k8s/**`.

**Tech Stack:** Kubernetes manifests, GitHub Actions matrix, Jupyter notebook, Markdown ADR.

**Scope boundaries:**
- **No nightly real-LLM eval CI job.** Kyle handles scheduling separately if wanted.
- **No Cloudflare Tunnel config changes.** The existing wildcard rule covers `api.kylebradshaw.dev/ai-api/*` once the ingress accepts the path.
- **No production smoke test for `/chat`.** `/health` and `/ready` are checked; a real-chat smoke would need Ollama reachable from GHA, which is not how the environment is set up.

**Reference:** spec section 6.3 (deployment), 6.4 (CI), 6.5 (documentation).

---

## File Map

New:
```
go/k8s/
├── configmaps/ai-service-config.yml
├── deployments/ai-service.yml
├── services/ai-service.yml
docs/adr/
├── go-ai-service/
│   └── 01-agent-harness-in-go.ipynb
└── rag-reevaluation-2026-04.md
```

Modified:
- `go/k8s/ingress.yml` — add `/ai-api(/|$)(.*)` path
- `.github/workflows/go-ci.yml` — add `ai-service` to lint/test/build matrices
- `frontend/src/lib/ai-service.ts` — flush SSE buffer on stream close
- `CLAUDE.md` — add `go/ai-service/` to the project structure tree

---

## Task 1: K8s manifests for ai-service

**Files:**
- Create `go/k8s/configmaps/ai-service-config.yml`
- Create `go/k8s/services/ai-service.yml`
- Create `go/k8s/deployments/ai-service.yml`
- Modify `go/k8s/ingress.yml`

### 1a. ConfigMap

`go/k8s/configmaps/ai-service-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ai-service-config
  namespace: go-ecommerce
data:
  PORT: "8093"
  OLLAMA_URL: "http://ollama.ai-services.svc.cluster.local:11434"
  OLLAMA_MODEL: "qwen2.5:14b"
  ECOMMERCE_URL: "http://go-ecommerce-service:8092"
  REDIS_URL: "redis://redis.java-tasks.svc.cluster.local:6379"
```

### 1b. Service

`go/k8s/services/ai-service.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: go-ai-service
  namespace: go-ecommerce
spec:
  selector:
    app: go-ai-service
  ports:
    - port: 8093
      targetPort: 8093
```

### 1c. Deployment

`go/k8s/deployments/ai-service.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-ai-service
  namespace: go-ecommerce
spec:
  replicas: 1
  selector:
    matchLabels:
      app: go-ai-service
  template:
    metadata:
      labels:
        app: go-ai-service
    spec:
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: go-ai-service
          image: ghcr.io/kabradshaw1/portfolio/go-ai-service:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 8093
          envFrom:
            - configMapRef:
                name: ai-service-config
          env:
            - name: JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: go-secrets
                  key: jwt-secret
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8093
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /health
              port: 8093
            initialDelaySeconds: 30
            periodSeconds: 30
```

**Secret reuse:** `go-secrets/jwt-secret` already exists because `auth-service` and `ecommerce-service` use it. No new Secret needed.

**Cross-namespace DNS:** Ollama lives in the `ai-services` namespace as an `ExternalName` Service pointing at `host.minikube.internal`. From the `go-ecommerce` namespace, the full DNS name is `ollama.ai-services.svc.cluster.local`. Use that exact form in the ConfigMap above.

### 1d. Ingress update

Modify `go/k8s/ingress.yml`. Add a third path under `rules[0].http.paths`:

```yaml
          - path: /ai-api(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: go-ai-service
                port:
                  number: 8093
```

Keep the existing `/go-auth` and `/go-api` paths. The `nginx.ingress.kubernetes.io/rewrite-target: /$2` annotation already rewrites so `/ai-api/chat` becomes `/chat` on the upstream.

### Steps

- [ ] **Step 1: Create the three new manifest files and modify ingress.yml.**

- [ ] **Step 2: Dry-run validate YAML locally**

```bash
for f in go/k8s/configmaps/ai-service-config.yml go/k8s/services/ai-service.yml go/k8s/deployments/ai-service.yml go/k8s/ingress.yml; do
  python3 -c "import yaml,sys; list(yaml.safe_load_all(open('$f'))); print('ok: $f')"
done
```

Expected: `ok: <file>` for each.

- [ ] **Step 3: Commit**

```bash
git add go/k8s/
git commit -m "feat(ai-service): add K8s manifests and /ai-api ingress path"
```

---

## Task 2: Wire ai-service into `go-ci.yml`

**Files:**
- Modify: `.github/workflows/go-ci.yml`

Three matrix strategies (`lint`, `test`, `build`) currently list `[auth-service, ecommerce-service]`. Add `ai-service` to each.

- [ ] **Step 1: Read the file and update the matrices**

Find each `matrix:` block (there are three) and change:
```yaml
        service: [auth-service, ecommerce-service]
```
to:
```yaml
        service: [auth-service, ecommerce-service, ai-service]
```

Verify that the `lint`, `test`, and `build` jobs use `working-directory: go/${{ matrix.service }}` — they do. The existing pattern expects `go/<service>/go.mod`, `go/<service>/Dockerfile`, and a `tags: ghcr.io/${{ github.repository }}/go-${{ matrix.service }}:latest` image name, all of which match what Plan 1 Task 10 created (`go/ai-service/go.mod`, `go/ai-service/Dockerfile`).

- [ ] **Step 2: Verify the file parses**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/go-ci.yml'))" && echo ok
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/go-ci.yml
git commit -m "ci(go): add ai-service to lint/test/build matrices"
```

---

## Task 3: Fix SSE parser buffer flush on stream close

**Files:**
- Modify: `frontend/src/lib/ai-service.ts`

The parser currently only emits an event when it sees a `\n\n` separator in the buffer. If the server closes the connection with a trailing event that doesn't end in a blank line, that event is never yielded. Plan 4's E2E worked around this by padding the test fixture. Fix the parser so it flushes any remaining buffered chunk when the reader returns `done`.

### Step 1: Read the current file and locate the while loop

The current loop body is:

```ts
while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  buffer += decoder.decode(value, { stream: true });
  // … split on \n\n, yield events …
}
```

### Step 2: Add a final flush after the loop

Replace the `if (done) break;` structure with:

```ts
while (true) {
  const { done, value } = await reader.read();
  if (done) {
    // Flush any remaining chunk that wasn't terminated by a blank line.
    buffer += decoder.decode(); // flush decoder
    const tail = buffer.trim();
    if (tail.length > 0) {
      const parsed = parseSseChunk(tail);
      if (parsed) yield parsed;
    }
    break;
  }
  buffer += decoder.decode(value, { stream: true });
  let sep: number;
  while ((sep = buffer.indexOf("\n\n")) !== -1) {
    const chunk = buffer.slice(0, sep);
    buffer = buffer.slice(sep + 2);
    const parsed = parseSseChunk(chunk);
    if (parsed) yield parsed;
  }
}
```

### Step 3: Type-check + run the mocked E2E with the padding removed

Edit `frontend/e2e/mocked/go-ai-assistant.spec.ts` and remove the extra empty string that was added to the `sseBody` array to pad the final event. The spec should now pass with a natural SSE tail.

```bash
cd frontend && npx tsc --noEmit
cd frontend && npm run e2e -- e2e/mocked/go-ai-assistant.spec.ts
```

Expected: tsc clean, both tests PASS.

### Step 4: Commit

```bash
git add frontend/src/lib/ai-service.ts frontend/e2e/mocked/go-ai-assistant.spec.ts
git commit -m "fix(frontend): flush SSE buffer on stream close"
```

---

## Task 4: Standalone ADR — `rag-reevaluation-2026-04.md`

**Files:**
- Create: `docs/adr/rag-reevaluation-2026-04.md`

Short, dated, honest. The interview value of this doc is that it shows Kyle changed his mind for stated reasons.

### Contents

```markdown
# RAG Reevaluation — 2026-04

**Date:** 2026-04-09
**Status:** Accepted
**Context:** Portfolio AI work, Gen AI Engineer job applications (Go-focused roles)

## Decision

Pivot the portfolio's AI roadmap away from "more RAG" and toward **agents with tool use in a Go microservice**. Keep the existing Python Doc Q&A and Debug Assistant as RAG evidence, but do not invest further in them. Build a new Go service (`go/ai-service`) whose center of gravity is an LLM agent loop orchestrating typed tool calls against real ecommerce backends.

## Why

### 1. RAG is commodity in 2026

Large context windows (1M tokens) have eroded naive RAG's differentiation for small corpora. The "stuff the docs in the prompt" approach beats a naive RAG pipeline on accuracy, dev speed, and complexity for most portfolio-sized use cases. RAG still wins for corpora that don't fit, cost at scale, freshness, and citation — all real — but the portfolio's Doc Q&A already demonstrates RAG competence. Doubling down on more RAG is diminishing returns.

### 2. Agents and tool use are the scarce skill

The shift toward agentic AI (tool calling, MCP, multi-step reasoning) is the single biggest capability change since the portfolio was started. Job postings for AI-focused Go roles increasingly probe for structured output, function calling, and "AI as a system citizen" (evals, observability, cost/latency awareness). These are underrepresented in candidate portfolios relative to RAG demos.

### 3. The portfolio's center of gravity should match the job search

Kyle applies primarily to Go roles. The existing AI work lives in Python, and the previous roadmap's integration plan leaned on the Java stack. A Go hiring manager skimming the repo should see **Go services doing interesting AI-adjacent work**, not Python services reaching into Java. The `go/ai-service` plan puts Go at the center: agent loop, tool registry, HTTP/SSE handler, structured-output validation, evals, caching, metrics — all in Go. Python stays as the model-serving / embedding layer (realistic — nobody writes embedding pipelines in Go).

## Consequences

- The roadmap doc `docs/superpowers/specs/2026-04-07-ai-enhancements-roadmap.md` is superseded by `2026-04-09-go-ai-service-agent-design.md`.
- Tracks A, B-Go, and C from the old roadmap are collapsed into the new Go service. Java Track B integration is dropped.
- The nine-tool catalog that ships with `go/ai-service` deliberately includes `summarize_orders` (a sub-LLM call over structured rows) as the one "LLM over non-doc structured data" capability the RAG pair didn't already show.
- The tool registry is designed so a future MCP adapter can be added without touching any existing code. MCP is the 2026 hype topic; this lets the portfolio pick it up later without rewrite.
- `place_order` / checkout is deliberately **not** a tool. "I drew a boundary here and here's why" is a stronger interview answer than a flashier demo.

## Alternatives considered

- **Keep building more RAG.** Would produce more work, less signal. Rejected.
- **Build an MCP server from scratch as the headline.** Tempting — MCP is the hottest topic. But the Go MCP ecosystem is young and an MCP server could eat the whole project and leave a thin agent behind. The tool registry's interface-based design lets us add MCP as a one-file follow-up once the agent itself is solid.
- **Put the agent work in Python.** Faster to build (existing FastAPI scaffolding, mature libraries), but mismatches the job search and loses the "Go + LLM" talking point that's the scarcest part of the pitch.

## Evidence this was right

- `go/ai-service` ships nine tools, JWT auth, a cache, metrics, guardrails, an eval harness, and a frontend drawer — all in Go, all testable offline, all deployed to Minikube.
- The existing Python Doc Q&A and Debug Assistant still demonstrate RAG.
- The tool registry lives behind a single-file interface that an MCP adapter can implement without changes to any consumer.
```

- [ ] **Commit**

```bash
git add docs/adr/rag-reevaluation-2026-04.md
git commit -m "docs(adr): record RAG reevaluation and Go pivot (2026-04)"
```

---

## Task 5: ADR notebook — `go-ai-service/01-agent-harness-in-go.ipynb`

**Files:**
- Create: `docs/adr/go-ai-service/01-agent-harness-in-go.ipynb`

This is the interview artifact. It should walk a reader through:
1. **Why Go** — the RAG-reevaluation summary, one paragraph referencing the standalone ADR.
2. **Architecture** — a diagram-lite description of `llm.Client` → `agent.Agent` → `tools.Registry` → `ecommerce-service`, plus the ascii flowchart from the spec.
3. **The agent loop** — the actual `Run` method pseudocode with the design decisions called out (tool errors fed back not bubbled, hard step cap + wall-clock timeout, registry passed in not global, user id explicit parameter).
4. **Tool registry shape** — the `Tool` interface, why `Result` has both `Content` and `Display`, and how a future MCP adapter slots in.
5. **Ollama tool-calling contract** — what the request looks like going out, what comes back.
6. **Auth model** — shared HS256 secret, JWT forwarded through `context`, ecommerce-service is the sole authz authority.
7. **Python vs. Go agent loop** — brief comparison to the existing Python Debug Assistant agent loop, what's the same, what's different, what Go makes easier.
8. **Operating it** — caching, metrics (list of counters/histograms), rate limiting, refusal detection, eval harness.
9. **What's deliberately out of scope** — no `place_order`, no embedding cache (yet), no MCP in v1.

### Step 1: Create the notebook

Use the same notebook shape as `docs/adr/document-qa/01-*.ipynb` or `docs/adr/document-debugger/01-*.ipynb` — look at one of those first to match section conventions (Overview, Architecture Context, Package Introductions, Go/TS Comparison, Build It, Experiment, Check Your Understanding).

The notebook should be markdown-heavy, not code-heavy. Include short Go snippets pulled from the real files to illustrate, but don't try to make them executable. Jupyter notebooks render markdown cells fine, and Go cells without a Go kernel will show as raw code blocks which is what we want here.

A good target length: 15–20 cells, roughly half markdown half code. The reader should be able to skim the whole thing in under 10 minutes.

**Include these specific code blocks pulled from the real files:**
- The `Tool` interface definition from `internal/tools/registry.go`
- The `Agent.Run` signature and the loop body (pseudocode is fine) from `internal/agent/agent.go`
- The `llm.Client` interface from `internal/llm/client.go`
- An example tool (`get_product` from `internal/tools/catalog.go`) showing how thin tools stay
- One Prometheus counter declaration from `internal/metrics/metrics.go`

**Include a "Check Your Understanding" section** at the end with 3–4 short questions:
1. If the registry `Register`s two tools with the same name, which one wins and why?
2. What does the agent loop do when a tool returns an error? What does it do when the LLM call returns an error?
3. Why is `userID` an explicit parameter on `Tool.Call` instead of being read from the `context.Context`?
4. What would it take to add an MCP adapter?

### Step 2: Validate the notebook JSON

```bash
python3 -c "import json; json.load(open('docs/adr/go-ai-service/01-agent-harness-in-go.ipynb')); print('ok')"
```

Expected: `ok`.

### Step 3: Commit

```bash
git add docs/adr/go-ai-service/01-agent-harness-in-go.ipynb
git commit -m "docs(adr): add go-ai-service agent harness ADR notebook"
```

---

## Task 6: Update root `CLAUDE.md` project structure

**Files:**
- Modify: `CLAUDE.md`

Find the `## Project Structure` section and add `go/ai-service/` to the `go/` block. The current block looks like:

```
go/                         # Go microservices
├── auth-service/           # JWT auth (register, login, refresh), PostgreSQL
├── ecommerce-service/      # Products, cart, orders, Redis caching, RabbitMQ worker pool
├── k8s/                    # Go-specific K8s manifests
```

Change to:

```
go/                         # Go microservices
├── auth-service/           # JWT auth (register, login, refresh), PostgreSQL
├── ecommerce-service/      # Products, cart, orders, Redis caching, RabbitMQ worker pool
├── ai-service/             # Agent loop over Ollama tool-calling, 9 tools wrapping ecommerce
├── k8s/                    # Go-specific K8s manifests
```

Also find the `docs/adr/` block and add:

```
├── go-ai-service/          # 1 notebook (agent harness in Go, tool registry, evals)
```

And add `rag-reevaluation-2026-04.md` to the standalone ADRs list in the same block if the block enumerates them.

### Steps

- [ ] **Step 1: Read `CLAUDE.md` and make the two edits.**

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): add go/ai-service and new ADRs to project structure"
```

---

## Done criteria for Plan 5

- `go/k8s/configmaps/ai-service-config.yml`, `go/k8s/services/ai-service.yml`, `go/k8s/deployments/ai-service.yml` exist and parse as valid YAML.
- `go/k8s/ingress.yml` routes `/ai-api(/|$)(.*)` to `go-ai-service:8093`.
- `.github/workflows/go-ci.yml` lint/test/build matrices include `ai-service`.
- `frontend/src/lib/ai-service.ts` flushes its SSE buffer on stream close; the E2E spec works without the padding workaround.
- `docs/adr/go-ai-service/01-agent-harness-in-go.ipynb` exists and is valid JSON.
- `docs/adr/rag-reevaluation-2026-04.md` exists.
- `CLAUDE.md` lists `go/ai-service/` and the two new ADRs in the project structure.
- After Kyle pushes `go-ai-service-design` → `staging` → `main`, the existing `ci.yml` deploy job rolls out ai-service alongside the other Go services automatically (no changes needed to that file).

## What is still Kyle's job after Plan 5 merges

- Push `go-ai-service-design` → `staging` → `main` manually.
- Monitor the GHA run and the Minikube rollout.
- Add `NEXT_PUBLIC_AI_SERVICE_URL=https://api.kylebradshaw.dev/ai-api` to Vercel production env vars before merging to main. Redeploy the Vercel frontend so the new env var takes effect.
- Seed at least a few products in `ecommercedb` so the agent has something to find.
- Optional: add a nightly scheduled workflow that runs `make preflight-ai-service-evals` against a real Ollama via SSH — not included in this plan.
