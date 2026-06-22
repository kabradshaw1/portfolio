# Galaxy Generative-AI Explainer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deep-technical "How the Generative AI Works" explainer (text-gen flow, image-gen flow, prompt-engineering emphasis) to the existing `/galaxy` portfolio page.

**Architecture:** Pure frontend documentation change to a single Next.js App Router page. Three new `<section>` blocks appended after "Architecture" and before "Engineering Focus", reusing the existing `MermaidDiagram` component, the card-grid pattern, and the established `<pre>` code-block style. Content is grounded verbatim in the real `~/repos/story` services.

**Tech Stack:** Next.js (App Router), React, TypeScript, Tailwind, Mermaid (via existing `MermaidDiagram` client component).

## Global Constraints

- Single file modified: `frontend/src/app/galaxy/page.tsx`. No new components, no backend changes.
- Place new sections **between** the existing `Architecture` section (ends ~line 149) and the `Engineering Focus` section (starts ~line 185).
- Reuse existing patterns only:
  - Diagrams: `<MermaidDiagram chart={`...`} />` (already imported; dark theme).
  - Cards: `grid grid-cols-1 gap-3 sm:grid-cols-2` with `rounded-lg border border-foreground/10 p-4`.
  - Code/prompt blocks: `<pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">`.
  - Section wrappers: `<section className="mt-12">`, headings `text-2xl font-semibold`, body `mt-4 text-muted-foreground leading-relaxed`.
- Content must stay accurate to the real services: text gen = **synchronous gRPC streaming** (no broker); image gen = **async via RabbitMQ, retry ×3 → DLQ**.
- Verification per task: `npx tsc --noEmit` and `npm run lint` pass (CI-checks rule); page renders in dev server. Doc commits stay local (no push) until shipped.
- Work in a git worktree created before execution (per worktree-before-execution rule).

---

### Task 1: Intro section + prompt-engineering takeaway data

**Files:**
- Modify: `frontend/src/app/galaxy/page.tsx` (add a `promptPrinciples` data array near the top alongside `stack`/`graphDomains`; insert intro `<section>` after Architecture).

**Interfaces:**
- Produces: module-level `const promptPrinciples: { title: string; desc: string }[]` consumed by Task 4's card grid.

- [ ] **Step 1: Add the `promptPrinciples` data array**

Add after the existing `graphDomains` array (~line 43):

```tsx
const promptPrinciples = [
  {
    title: "Context injection over raw passthrough",
    desc: "Generation never sends only the user's text to the model. The service assembles a prompt from related worldbuilding entities so output stays consistent with the established universe.",
  },
  {
    title: "Relevance-scoped context",
    desc: "Injected entities are fetched cache-first (Redis, 10-minute TTL) with a Postgres fallback, and scene characters are filtered to only the organizations involved in that scene — focused prompts, not context dumps.",
  },
  {
    title: "Layered, compositional prompts",
    desc: "Images compose a global art-direction base, a per-entity-type overlay, and the user's subject. Text layers a task instruction, the writer's seed text, and a structured context block.",
  },
  {
    title: "Execution model matched to cost",
    desc: "Cheap, latency-sensitive text streams back token-by-token over gRPC. Expensive image jobs are queued through RabbitMQ with retries and a dead-letter queue.",
  },
];
```

- [ ] **Step 2: Insert the intro section**

Add immediately after the closing `</section>` of the Architecture section (~line 149), before the "Why GraphQL" section:

```tsx
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">How the Generative AI Works</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          GalaxyVoyagers has two generative capabilities with deliberately
          opposite execution models, and both are driven by prompt engineering
          rather than by passing raw user text to a model. Text generation runs
          synchronously and streams results token-by-token, so the writer sees
          prose appear immediately. Image generation is expensive, so it is
          decoupled onto an asynchronous queue with retries. In both cases the
          interesting work happens before the model call: the service assembles
          a structured prompt from the surrounding worldbuilding data.
        </p>
      </section>
```

- [ ] **Step 3: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: PASS (no errors). The `promptPrinciples` array will be flagged as unused by lint until Task 4 — if lint errors on unused var, proceed to Task 4 in the same working session before committing, or add the array in Task 4 instead. To keep tasks independently committable, **defer committing until Task 4 consumes it** OR add `promptPrinciples` in Task 4. Chosen approach: add the array here but commit at end of Task 1 only if lint passes; Next.js/eslint default does not error on unused module consts, so this is expected to pass.

- [ ] **Step 4: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add frontend/src/app/galaxy/page.tsx
git commit -m "feat(galaxy): add generative-AI intro section + prompt principles data"
```

---

### Task 2: Text Generation section (flow + assembly diagrams + prompt snippet)

**Files:**
- Modify: `frontend/src/app/galaxy/page.tsx` (insert section after the intro section from Task 1).

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing consumed downstream.

- [ ] **Step 1: Insert the Text Generation section**

Add immediately after the intro section's closing `</section>`:

```tsx
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Text Generation: Context-Injected Prompting
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          When a writer asks for a scene description, the request reaches the
          storygen service over gRPC. Before calling the model, storygen pulls
          the entities related to that scene — characters, locations, conflicts,
          organizations, and ship casualties — from a Redis cache, falling back
          to PostgreSQL on a miss. It folds them into a single context-injected
          prompt, then streams the model&apos;s output back token-by-token over a
          gRPC server stream.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  U[Browser]
  GW[Go GraphQL Gateway]
  SG[storygen-service<br/>gRPC :50054]
  CACHE[(Redis cache<br/>10-min TTL)]
  PG[(PostgreSQL<br/>world data)]
  AI[OpenAI /responses<br/>streaming SSE]
  U -->|request scene text| GW
  GW -->|gRPC StreamScene| SG
  SG -->|fetch related entities| CACHE
  CACHE -. cache miss .-> PG
  PG -. hydrate .-> CACHE
  SG -->|context-injected prompt| AI
  AI -->|output_text.delta chunks| SG
  SG -->|gRPC stream| GW
  GW -->|tokens stream to UI| U`}
          />
        </div>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          The prompt itself is built in layers. A fixed task instruction comes
          first, then the writer&apos;s seed text, then a context block populated
          with the related entities:
        </p>
        <div className="mt-4">
          <MermaidDiagram
            chart={`flowchart TB
  A["1 · Task instruction<br/>I need a description of this scene as an essay"]
  B["2 · Writer's seed text"]
  C["3 · For context: injected entities"]
  C1[Characters — filtered to in-scene organizations]
  C2[Locations · Conflicts · Organizations · Casualties]
  OUT[Assembled prompt → OpenAI /responses]
  A --> OUT
  B --> OUT
  C --> OUT
  C1 --> C
  C2 --> C`}
          />
        </div>
        <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`// storygen assembles the scene prompt by injecting related entities (Go)
prompt := "I need a description of this scene in the form of an essay.\\n"
prompt += req.Text                          // the writer's seed text
prompt += "\\nFor context:\\n"
prompt += "The following characters are involved in this scene: " + characters
prompt += "The scene takes place at the following locations: "    + locations
prompt += "This scene is associated with the following conflicts: " + conflicts
prompt += "The following organizations are involved in this scene: " + orgs

// characters are filtered to ONLY the organizations involved in this scene,
// and every entity is fetched cache-first (Redis, 10-min TTL) -> Postgres.`}
        </pre>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The result is a prompt grounded in the specific corner of the universe
          the writer is editing, which keeps generated prose consistent with
          established characters and places instead of inventing contradictions.
        </p>
      </section>
```

- [ ] **Step 2: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: PASS.

- [ ] **Step 3: Verify render**

Run: `cd frontend && npm run dev` (if not already running), open `http://localhost:3000/galaxy`.
Expected: Both new Mermaid diagrams render in dark theme; the prompt `<pre>` shows literal `\n` escapes as written and no JSX errors. Confirm `&apos;` renders as an apostrophe.

- [ ] **Step 4: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add frontend/src/app/galaxy/page.tsx
git commit -m "feat(galaxy): add text-generation flow + prompt-assembly explainer"
```

---

### Task 3: Image Generation section (flow + composer diagrams + YAML snippet)

**Files:**
- Modify: `frontend/src/app/galaxy/page.tsx` (insert section after the Text Generation section).

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing consumed downstream.

- [ ] **Step 1: Insert the Image Generation section**

Add immediately after the Text Generation section's closing `</section>`:

```tsx
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Image Generation: Layered Style Composition
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Image generation is slow and expensive, so it is decoupled from the
          request path. The image service accepts the job over gRPC and enqueues
          it on RabbitMQ. A consumer composes the final prompt, calls the image
          model, uploads the PNG to object storage, records metadata in
          PostgreSQL, and notifies the gateway with a signed URL through an HTTP
          callback. Failed jobs are retried up to three times before moving to a
          dead-letter queue.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  GW[Go GraphQL Gateway]
  IMG[image-service<br/>ProcessImage gRPC]
  MQ{{RabbitMQ<br/>image queue}}
  W[image-consumer<br/>generation worker]
  COMP[Style composer<br/>YAML templates]
  AI[OpenAI gpt-image-1.5<br/>1024x1024]
  OBJ[(Object storage<br/>signed URLs)]
  PG[(PostgreSQL<br/>GenImageJob / GenFile)]
  GW -->|gRPC| IMG
  IMG -->|enqueue job| MQ
  MQ -->|consume| W
  W --> COMP
  COMP -->|composed prompt| AI
  AI -->|PNG bytes| W
  W -->|upload| OBJ
  W -->|metadata| PG
  W -->|HTTP callback + signed URL| GW
  MQ -. retry x3 then DLQ .-> MQ`}
          />
        </div>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          The prompt is assembled by a style composer from a three-part template:
          a global art-direction base, a per-entity-type overlay that reframes
          the composition, and the user&apos;s subject text.
        </p>
        <div className="mt-4">
          <MermaidDiagram
            chart={`flowchart LR
  BASE["base<br/>global art direction"]
  OVL["overlay<br/>per entity type"]
  SUBJ["Subject: user_prompt"]
  OUT["{base}. {overlay}. Subject: {user_prompt}"]
  BASE --> OUT
  OVL --> OUT
  SUBJ --> OUT`}
          />
        </div>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The entire visual identity lives in a YAML configmap loaded at startup,
          so the art direction can be tuned without a code change:
        </p>
        <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# services/image/style/templates.yaml
version: 1
format: "{base}. {overlay}. Subject: {user_prompt}"
base: |-
  Anime sci-fi superhero aesthetic. Cel-shaded with crisp ink lines and
  vibrant saturated palette. Dramatic rim lighting against deep space or
  neon-lit backdrops. High detail, dynamic composition, painterly shading.
entities:
  characters:
    overlay: "Hero portrait, three-quarter angle, expressive pose, costume detail emphasized."
  ships:
    overlay: "Exterior hero shot, 3/4 angle, sense of scale, engines glowing, motion-blurred star field."
  scenes:
    overlay: "Wide cinematic establishing shot, atmospheric perspective, characters small in frame."`}
        </pre>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          A request to illustrate a character therefore composes to a single
          prompt — the base style, the character overlay, then{" "}
          <code>Subject: &lt;the user&apos;s description&gt;</code> — giving every
          generated image a consistent, art-directed look across the platform.
        </p>
      </section>
```

- [ ] **Step 2: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: PASS.

- [ ] **Step 3: Verify render**

Open `http://localhost:3000/galaxy`.
Expected: Both diagrams render; the YAML `<pre>` preserves indentation and quotes; `<code>` inline renders the `Subject:` example.

- [ ] **Step 4: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add frontend/src/app/galaxy/page.tsx
git commit -m "feat(galaxy): add image-generation flow + style-composer explainer"
```

---

### Task 4: Prompt Engineering Takeaways cards + final verification

**Files:**
- Modify: `frontend/src/app/galaxy/page.tsx` (insert card section after the Image Generation section; consumes `promptPrinciples` from Task 1).

**Interfaces:**
- Consumes: `promptPrinciples` (module const from Task 1).

- [ ] **Step 1: Insert the takeaways section**

Add immediately after the Image Generation section's closing `</section>`:

```tsx
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Prompt Engineering Takeaways</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Across both generation paths, the same prompt-engineering principles
          recur: ground the model in real domain data, keep that data relevant
          and cheap to fetch, compose prompts in layers, and match the execution
          model to the cost of the work.
        </p>
        <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {promptPrinciples.map((item) => (
            <div
              key={item.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h3 className="text-sm font-semibold">{item.title}</h3>
              <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
                {item.desc}
              </p>
            </div>
          ))}
        </div>
      </section>
```

- [ ] **Step 2: Add "Prompt engineering" to the stack chip list**

In the `stack` array (~line 5-24), add `"Prompt engineering"` after `"Image generation"`:

```tsx
  "Image generation",
  "Prompt engineering",
```

- [ ] **Step 3: Type-check and lint**

Run: `cd frontend && npx tsc --noEmit && npm run lint`
Expected: PASS, no unused-variable warnings (`promptPrinciples` now consumed).

- [ ] **Step 4: Full-page render verification**

Open `http://localhost:3000/galaxy`. Verify section order top-to-bottom:
Deployed project → Technology Stack (now includes "Prompt engineering") →
Architecture → **How the Generative AI Works → Text Generation → Image
Generation → Prompt Engineering Takeaways** → Why GraphQL → Engineering Focus.
Expected: all four new diagrams render in dark theme; both prompt blocks
readable; cards laid out two-up on `sm+`.

- [ ] **Step 5: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add frontend/src/app/galaxy/page.tsx
git commit -m "feat(galaxy): add prompt-engineering takeaways + stack chip"
```

---

## Self-Review

**Spec coverage:**
- Section A (intro) → Task 1. ✓
- Section B (text gen flow + assembly diagram + verbatim snippet + relevance-scoped callout) → Task 2. ✓
- Section C (image gen flow + composer diagram + YAML snippet + configmap callout) → Task 3. ✓
- Section D (4 takeaway cards) → Task 4. ✓
- "Append after Architecture, before Engineering Focus" → Tasks insert between those sections. ✓
- Reuse `MermaidDiagram` / card grid / `<pre>` patterns → all tasks use existing classes. ✓
- Accuracy: text sync streaming (Task 2 diagram, no broker) / image async retry×3→DLQ (Task 3 diagram). ✓
- Verification tsc+lint+render → every task. ✓

**Placeholder scan:** No TBD/TODO; all code blocks are complete and literal.

**Type consistency:** `promptPrinciples` defined in Task 1 (`{title, desc}[]`), consumed in Task 4 with `item.title` / `item.desc` matching the existing `graphDomains` card pattern. ✓
