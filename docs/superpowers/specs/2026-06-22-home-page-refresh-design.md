# Home Page (`/`) Refresh — Design

**Date:** 2026-06-22
**Files touched:**
- `frontend/src/app/page.tsx` (bio, sections, card copy/titles)
- `frontend/src/app/ai/page.tsx` (add a link to `/dspm` so it isn't orphaned by the AI card merge)
- `frontend/e2e/mocked/async-page.spec.ts` (update one homepage link assertion for the renamed Async card)

## Goal

Update the bio and project cards on the portfolio home page to reflect the
current state of the portfolio (now ~11 projects, not the "three areas of
specialization" the old bio claimed) and to position Kyle for the roles he's
actually interviewing for: **full-stack engineer, Go microservices, AI/RAG
integration, React + Go/Python APIs — mid-to-senior, 4 years experience.**

## Decisions (from brainstorming)

- **Audience:** full-stack / Go microservices / some AI integration. Most target
  roles list React, a mix of Go + Python APIs, and LLM/RAG. Mid-to-senior, 4 yrs.
- **Bio:** replace with Option B (skills-first, scannable, React in lead position).
- **Cards:** keep all 11, but organize into themed sections (no flat list).
- **Featured:** GalaxyVoyagers gets its own section at the top with a **subtle
  section heading** — no oversized card, no accent treatment.
- **Card copy:** light refresh — tighten stale/off-tone copy and replace
  job-title-style card titles with project-oriented titles. Not a full rewrite.
- **Title renames:** rename the home **cards only**; do NOT touch the destination
  project-page `<h1>`s. Card labels and page headings are allowed to differ
  (a card is a label; the page carries the full title). The one e2e assertion
  that checks a renamed card title gets updated.
- **AI cards merged:** the two AI projects (Document Q&A / RAG and DSPM
  Classifier) collapse into **one** card. It links to `/ai` and its description
  covers both the RAG assistant and DSPM classification. Because `/dspm` is not
  in the global nav, the `/ai` page gains a link to `/dspm` so it stays reachable.
- **React:** surfaced via the bio's lead position only. No forced React callouts
  in card copy, no "built with" meta line.

## Bio (new copy)

Replaces the two old paragraphs (the Grafana line is **kept as-is**).

> # Kyle Bradshaw
> Full-stack engineer — React, Go and Python microservices, and LLM/RAG
> integration. Four years of experience, the last stretch spent consulting and
> building production systems independently: designing the APIs, shipping the
> frontends, and running the whole stack on Kubernetes. Everything below is
> deployed and instrumented, not a demo.
>
> *(existing Grafana dashboard line unchanged)*

## Card organization

Four `<section>` blocks, each with an `<h2>` heading, in this order. The generic
"Portfolio" `<h2>` is removed in favor of these section headings. All cards reuse
the existing `Card` / `CardHeader` / `CardTitle` / `CardDescription` /
`CardContent` components — no new component types, same hover styling, same grid.

| Order | Section heading | Cards (in order) |
|-------|-----------------|------------------|
| 1 | **Featured Project** | GalaxyVoyagers (1 card) |
| 2 | **Backend & Data Engineering** | Go Ecommerce · Full-Stack Java · Database Engineering · Asynchronous Systems (4 cards) |
| 3 | **AI Systems** | Document Q&A + DSPM (1 merged card) |
| 4 | **Platform & Operations** | Observability · CI/CD · Infrastructure & Deployment · Security (4 cards) |

Total: **10 cards** across 4 sections (down from 11 — the two AI cards merged
into one). Single-card sections (Featured, AI Systems) still get a heading for
structural consistency; the card title differs from the heading so they don't
read as redundant.

Rationale: Featured leads with the flagship full-stack/Go/AI app. Backend & Data
comes next (the core "Go microservices" story). AI Systems third (the "some AI
integration" the listings ask for). Platform & Operations last as supporting depth.

## Card copy — proposed light refresh

Only titles/descriptions that read as stale job-titles or are inconsistent with
the new framing change. `href`s are unchanged. Where a row says "unchanged," keep
the existing copy verbatim.

### Featured Project
- **GalaxyVoyagers.com** — description/body unchanged (already accurate and on-tone).

### Backend & Data Engineering
- **Go Ecommerce Platform** (was "Go Backend Developer") — title changed from a
  job title to the project. Description: "Microservices ecommerce platform built
  with Go, PostgreSQL, Redis, and RabbitMQ." Body unchanged.
- **Full-Stack Java** (was "Full Stack Java Developer") — title de-job-titled.
  Description/body unchanged.
- **Database Engineering** — unchanged.
- **Asynchronous Systems** (was "Asynchronous Systems Engineering") — title
  shortened. Description/body unchanged.

### AI Systems (merged — one card linking to `/ai`)
- **Document Q&A Assistant** (was "AI Engineer") — title changed from a job
  title to the project; links to `/ai`.
- Description: covers both projects — e.g. "A full-stack RAG system (FastAPI,
  Qdrant, Ollama) for PDF Q&A, plus a Kafka-scale DSPM classifier that detects
  sensitive data with a tiered regex → NER → LLM pipeline."
- The old standalone **DSPM Classifier** card is removed from the home page.
- **Companion edit:** `/ai/page.tsx` gains a link to `/dspm` (e.g. a "Related
  work" link) so the DSPM page stays reachable now that its home card is gone.

### Platform & Operations
- **Observability** — unchanged.
- **CI/CD Pipeline** — unchanged.
- **Infrastructure & Deployment** — unchanged.
- **Security** — unchanged.

## Non-goals

- No layout/component library changes, no new sections beyond the four above.
- No changes to project-page `<h1>`s (card-vs-page label divergence is accepted).
- The only project-page edit is the `/ai` → `/dspm` link required by the merge.
- No changes to `layout.tsx` metadata (out of scope unless requested).

## Testing / verification

- `npx tsc --noEmit` (frontend type check) per repo convention.
- Update `frontend/e2e/mocked/async-page.spec.ts:56` — the homepage assertion for
  a link named `/Asynchronous Systems Engineering/` must match the renamed card
  title ("Asynchronous Systems"). Re-run the async spec.
- Visually confirm: four section headings render in order; **10** cards present
  exactly once; every card links to its existing route; the merged AI card links
  to `/ai`; `/ai` links onward to `/dspm`; hover styling intact.
