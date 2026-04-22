# DLQ Admin Panel — Frontend

- **Date:** 2026-04-21
- **Status:** Accepted

## Context

The order-service has admin REST endpoints for inspecting and replaying dead-lettered saga messages (`GET /admin/dlq/messages`, `POST /admin/dlq/replay`), built as part of the DLQ replay tooling (#97). These endpoints are not exposed through the Kubernetes ingress — they're only reachable from within the cluster or via SSH tunnel. Without a frontend, the only way to demo this capability is raw curl commands, which doesn't showcase the tooling in the portfolio.

The question was how to surface this in the frontend: where it lives, how it handles the backend being unreachable (the normal state for public visitors), and how to frame it as a portfolio piece without pretending it's production-ready admin tooling.

## Decision

### Route and navigation placement

Added `/go/admin` as a new top-level section alongside Store and Analytics in the GoSubHeader. Considered embedding it as a tab on the landing page or a drawer panel, but a dedicated route is the cleanest separation — it follows the same pattern as `/go/ecommerce` and `/go/analytics`, and can expand to hold other admin tooling later.

### No authentication

The backend admin endpoints have no auth (they're network-protected by not being in the ingress). Adding JWT auth on the frontend would be ceremony without security benefit — a logged-in user on the public site still can't reach the endpoints. The page is accessible to all visitors, which is actually desirable for the portfolio use case.

### Demo banner with production context

A yellow callout at the top of the page explains this is portfolio tooling and that production environments would use a CLI tool with role-based access control. This is honest framing — visitors see the capability and understand the architectural thinking, without the page pretending to be something it's not.

### Graceful unreachable state

When the backend fetch fails with a network error (the expected state for public visitors), the page shows an explanation panel with SSH tunnel instructions instead of a generic error. This turns a limitation into a teaching moment — visitors understand why the endpoints aren't public and how an operator would access them.

When the backend is reachable (local dev with SSH tunnel), the page shows a live message table with expandable rows, message body/headers inspection, and inline replay with loading state and success feedback.

### Self-contained page, no context provider

The admin page owns its own state via `useState` + `useCallback`. No context provider was needed — there's no shared state with the store or analytics sections. This keeps the component simple and avoids adding to the provider tree that wraps all `/go` routes.

### API client pattern

Created `go-admin-api.ts` with a simpler pattern than the existing `go-order-api.ts` — no auth token refresh, no credentials. Each function returns a `DLQResult<T>` with `{ data, error, connected }` so the page can distinguish network errors (show unreachable state) from API errors (show error message).

## Consequences

**Positive:**
- The DLQ tooling is now visible in the portfolio — visitors can see the admin panel design even without backend connectivity.
- The demo banner and unreachable state tell a clear story about operational thinking: "I built the tooling, I know it shouldn't be publicly exposed, and here's how operators would access it."
- The expandable row pattern (click to reveal body/headers) keeps the table clean while allowing deep inspection.
- 6 Playwright mocked E2E tests cover all states: render, table data, expand, replay, and unreachable.

**Trade-offs:**
- Public visitors see a page that can't connect to its backend by design. The unreachable state explains this, but it's still a slightly odd UX — a page that exists to show you it can't reach its API.
- The Admin nav link is visible to all users, including those browsing the store. This is fine for a portfolio but would need gating in a real product.
