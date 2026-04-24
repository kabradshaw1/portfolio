# Go Page: Event Sourcing Tab

**Date:** 2026-04-23
**Status:** Draft

## Context

The order-projector service (Kafka event sourcing + CQRS) was just deployed. The `/go` page needs a new tab to highlight this feature alongside the existing Microservices, Original, AI Assistant, Analytics, and Admin tabs. The existing architecture diagram and service count also need updating to reflect the new 8th service.

## Scope

### In Scope

- New "Event Sourcing" tab component
- New CTA button linking to `/go/ecommerce/orders`
- Update service count "7 Go microservices" → "8 Go microservices" in MicroservicesTab tech stack pills
- Update architecture Mermaid diagram in MicroservicesTab to include order-projector
- Update bio text service count if it references a number

### Out of Scope

- Changes to the order-projector service itself
- Changes to the order detail page or timeline component

## Changes

### 1. New file: `frontend/src/components/go/tabs/EventSourcingTab.tsx`

Follows the same pattern as AnalyticsTab.tsx. Sections:

**"Why Event Sourcing?"** — 1 paragraph explaining event sourcing + CQRS, framed around enterprise relevance (audit trails, independent scaling, schema evolution, replay).

**Architecture diagram** — Mermaid flowchart:
```
order-service → Kafka (ecommerce.order-events) → order-projector → projectordb
                                                                  ↓
                                                           3 read models:
                                                           timeline, summary, stats
```
Also show the existing analytics-service on the separate `ecommerce.orders` topic for contrast.

**Tech stack pills** — "Event Sourcing", "CQRS", "Kafka Log Compaction", "Schema Evolution", "Event Replay", "Versioned JSON"

**"Event Flow" sequence diagram** — Mermaid sequence diagram showing one order's lifecycle: 7 event types flowing from saga → Kafka → projector → 3 read models → frontend timeline.

**"What It Demonstrates" cards** — 2x2 grid:
- **Immutable Event Log** — Every saga step recorded as an append-only event. Full audit trail queryable via timeline endpoint.
- **CQRS Read/Write Split** — Separate projector service with its own database. Denormalized schemas optimized for reads.
- **Schema Evolution** — Versioned JSON events with chainable upgrade functions. V1→V2 demonstrated with currency field backfill.
- **Event Replay** — Admin endpoint truncates read models and rebuilds from the full event stream. Disaster recovery in one API call.

**ADR link** — Link to `docs/adr/ecommerce/kafka-event-sourcing-cqrs.md` on GitHub, same style as the database optimization ADR link.

### 2. Modify `frontend/src/app/go/page.tsx`

- Add `"event-sourcing"` to the Tab type union
- Add `{ key: "event-sourcing", label: "Event Sourcing" }` to the tabs array (between "analytics" and "admin")
- Import and render `<EventSourcingTab />` for the new tab
- Add CTA button "Order Timeline →" linking to `/go/ecommerce/orders`, styled as bordered (secondary) like "Streaming Analytics" and "Admin Panel"

### 3. Modify `frontend/src/components/go/tabs/MicroservicesTab.tsx`

- Update tech stack pill from "7 Go microservices" to "8 Go microservices"
- Add order-projector to the architecture Mermaid diagram:
  - New node: `PROJ[order-projector<br/>REST :8097]`
  - New database: `PG_PROJ[(projectordb)]`
  - New edges: `KF --> PROJ`, `PROJ --> PG_PROJ`

### 4. Update bio text in `frontend/src/app/go/page.tsx`

The bio paragraph mentions "seven Go services" in the Grafana link sentence. Update to "eight Go services".

## Verification

- `npx tsc --noEmit` passes
- `npx next lint` passes
- Tab renders correctly in browser
- Mermaid diagrams render
- CTA button navigates to orders page
- Service count is consistent (8) across bio, tech stack pills, and architecture diagram
