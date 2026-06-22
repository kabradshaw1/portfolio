# Galaxy Page — "How the Generative AI Works" Section Design

**Date:** 2026-06-22
**Route:** `/galaxy` (`frontend/src/app/galaxy/page.tsx`)
**Status:** Approved (design), pending spec review

## Goal

Expand the existing `/galaxy` portfolio page to explain how GalaxyVoyagers'
generative AI works, with logic-flow diagrams for both text generation and
image generation, and a clear emphasis on the prompt-engineering techniques
used. Audience is deep-technical (Gen AI Engineer interviewers), so the page
shows real prompt templates and design rationale, not abstractions.

## Source of Truth

Content is grounded in the actual service code at `~/repos/story`:

- **Text:** `services/storygen/` (gRPC `:50054`, sync + streaming variants;
  `service/services.go`, `service/other.go`, `llmclient/openai.go`).
- **Image:** `services/image/` (`service/generation_worker.go`,
  `provider/openai.go`, `style/style.go`, `style/templates.yaml`).

Key accurate facts the page must reflect:

- **Text generation is synchronous gRPC streaming.** No message broker. Prompt
  is built by **context injection**: `[task instruction] + [user text] +
  "For context:" + [related entities]`. Related entities are fetched
  **cache-first (Redis, 10-min TTL) with Postgres fallback**. For scenes,
  injected characters are **filtered to only the organizations involved in the
  scene** (relevance-scoped, not a context dump). Streams via OpenAI
  `/responses` SSE deltas, relayed over gRPC server streaming.
- **Image generation is asynchronous via RabbitMQ** (max 3 attempts → DLQ).
  Prompt is built by a **3-tier style composer**:
  `{base}. {overlay}. Subject: {user_prompt}`, loaded from a YAML configmap.
  Model `gpt-image-1.5`, `1024x1024`, quality `medium`. Output uploaded to
  object storage; metadata in Postgres (`GenImageJob` / `GenFile`); result
  returned via HTTP callback carrying a signed URL. Idempotency via
  `ensureImageJob`.

## Placement

Append three new `<section>` blocks **after** the existing "Architecture"
section and **before** "Engineering Focus" in `galaxy/page.tsx`. Single-file
change. Reuse existing patterns: `<MermaidDiagram chart={...}/>` (dark theme),
the `grid grid-cols-1 gap-3 sm:grid-cols-2` card pattern, and the established
code-block style `<pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">`.

## Sections

### Section A — "How the Generative AI Works" (intro)

Short framing prose: GalaxyVoyagers has **two generative capabilities with
opposite execution models**, both driven by prompt engineering that injects
structured worldbuilding context rather than passing raw user text to a model:

- **Text** — synchronous, streaming gRPC; latency hidden by token-by-token
  streaming.
- **Image** — asynchronous, RabbitMQ-queued with retries/DLQ; expensive work
  decoupled from the request path.

Call out the contrast explicitly as a design decision.

### Section B — "Text Generation: Context-Injected Prompting"

- Prose: how a request (e.g. a scene description) flows through `storygen`.
- **Flow diagram** (Mermaid `flowchart`): `Client → Gateway → storygen (:50054)
  → fetch related entities (Redis cache → Postgres fallback) → assemble
  context-injected prompt → OpenAI /responses (stream) → SSE deltas → gRPC
  stream → UI`.
- **Prompt-assembly diagram** (Mermaid): layered composition —
  `[task instruction] + [user text] + [For context: characters (org-filtered) ·
  locations · conflicts · organizations · casualties]`.
- **Verbatim snippet** (`<pre>`): the real scene-prompt construction, e.g.

  ```
  "I need a description of this scene in the form of an essay.\n"
    + userText
    + "\nFor context:\n"
    + "The following characters are involved in this scene: ..."
    + "The scene takes place at the following locations: ..."
    + "This scene is associated with the following conflicts: ..."
  ```

- Callout: context is **cache-first** and **relevance-scoped** (scene
  characters filtered to in-scene organizations).

### Section C — "Image Generation: Layered Style Composition"

- Prose: why image generation is async (cost/latency) and the job lifecycle.
- **Flow diagram** (Mermaid): `Gateway → image-service ProcessImage →
  RabbitMQ image queue → image-consumer → style composer → gpt-image-1.5 →
  object storage (signed URL) + Postgres (GenImageJob/GenFile) → HTTP callback
  (signed URL) → UI`. Annotate `retry ×3 → DLQ`.
- **Composer diagram** (Mermaid): three tiers →
  `{base}. {overlay}. Subject: {user_prompt}`.
- **Verbatim snippet** (`<pre>`): the real `templates.yaml` (`version`,
  `format`, `base`, and representative `overlay` entries for characters / ships
  / scenes), plus one fully-composed example prompt.
- Callout: **deterministic, template-driven prompts from a YAML configmap** +
  idempotency → reproducible, swappable styling without code changes.

### Section D — "Prompt Engineering Takeaways" (4 cards)

Reuse the existing card grid. Four cards:

1. **Context injection over raw passthrough** — related entities assembled into
   the prompt, not just the user's text.
2. **Relevance-scoped context** — org-filtered, cache-backed (Redis 10-min
   TTL → Postgres) so prompts stay focused and fast.
3. **Layered / compositional prompts** — `base → overlay → subject` for images;
   instruction → context layering for text.
4. **Execution model matched to cost** — stream text synchronously, queue
   images asynchronously with retries.

## Out of Scope

- No backend/service changes; this is a frontend documentation page only.
- No live demo or API calls from the page; static explanatory content.
- No new shared components; reuse `MermaidDiagram` and existing Tailwind
  patterns.

## Verification

- `tsc` (type check) and lint pass before commit (per CI-checks rule).
- Both Mermaid diagrams render under the existing dark theme.
- Doc-only spec commit stays local; page change committed with the
  implementation.
