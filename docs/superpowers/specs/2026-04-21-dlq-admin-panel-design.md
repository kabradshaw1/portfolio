# DLQ Admin Panel — Frontend Design

**Date:** 2026-04-21
**Issues:** Follow-up to #97 (DLQ replay tooling)

## Context

The order-service now has admin REST endpoints for inspecting and replaying dead-lettered saga messages (`GET /admin/dlq/messages`, `POST /admin/dlq/replay`). These endpoints are not exposed through the Kubernetes ingress — they're only reachable from within the cluster or via SSH tunnel / port-forward. A frontend panel makes the tooling visible in the portfolio and provides a better demo experience than raw curl commands.

## Design

### Route & Navigation

- New route: `/go/admin`
- New link in GoSubHeader nav: Store | Analytics | **Admin**
- No authentication required (backend endpoints have no auth)
- No HealthGate wrapper — uses custom unreachable state handling

### Demo Banner

Yellow/amber callout at the top of the page:

> **Portfolio Demo** — This panel demonstrates DLQ operational tooling for the checkout saga. In a production environment, this would be implemented as a CLI tool with role-based access control.

### Stats Row

Two stat cards + refresh button:
- **DLQ Messages** — count from the API response
- **Refresh** button — re-fetches the message list

### Messages Table

| Column | Content |
|--------|---------|
| Index | Position in DLQ (0-based) |
| Routing Key | Original routing key (e.g. `saga.cart.commands`) |
| Timestamp | Relative time (e.g. "2m ago") |
| Retry Count | Number of previous replays |
| Action | Replay button |

**Expandable rows:** Click a row to reveal:
- Full message body (JSON, monospace preformatted)
- Headers (key-value list)
- Original exchange name

### Replay Flow

1. Click "Replay" button on a row
2. Button shows loading spinner, disabled during request
3. `POST /admin/dlq/replay` with `{"index": N}`
4. **Success:** Inline "Replayed" confirmation, auto-refresh message list
5. **Error:** Inline error message on the row

### Unreachable State

When the fetch fails with a network error (backend not reachable), show an explanation panel instead of the table:

> **Admin endpoints are not publicly exposed.** These endpoints are only reachable from within the Kubernetes cluster. To use this panel locally:
>
> ```
> ssh -f -N -L 8092:localhost:8092 debian
> ```

This is the expected state for visitors on the deployed portfolio site.

## API Client

**File:** `frontend/src/lib/go-admin-api.ts`

Two functions using native Fetch (no auth, no credentials):

```typescript
fetchDLQMessages(limit?: number): Promise<DLQListResponse>
// GET ${NEXT_PUBLIC_GO_ORDER_URL}/admin/dlq/messages?limit=N

replayDLQMessage(index: number): Promise<DLQReplayResponse>
// POST ${NEXT_PUBLIC_GO_ORDER_URL}/admin/dlq/replay  body: {"index": N}
```

Reuses existing `NEXT_PUBLIC_GO_ORDER_URL` env var. Returns a `connected: false` flag on network errors to trigger the unreachable state UI.

## Component Structure

| File | Purpose |
|------|---------|
| `frontend/src/app/go/admin/page.tsx` | Page component — data fetching, state, table rendering |
| `frontend/src/components/go/DLQMessageRow.tsx` | Expandable table row — collapsed/expanded states, replay button with loading |
| `frontend/src/components/go/DemoBanner.tsx` | Reusable demo/portfolio callout banner |

No context provider — page is self-contained. Fetches on mount and manual refresh.

## GoSubHeader Update

Add "Admin" link to the existing GoSubHeader navigation component, alongside Store and Analytics.

## Testing

Playwright mocked E2E test:
- Mock `/admin/dlq/messages` response with sample messages
- Verify table renders with correct columns
- Click row to expand, verify body/headers shown
- Click replay, verify POST request made
- Verify unreachable state renders when fetch fails

## Verification

1. `npm run dev` — navigate to `/go/admin`, verify page renders with demo banner
2. With SSH tunnel active — verify messages load, expand works, replay works
3. Without tunnel — verify unreachable state shows with instructions
4. `make preflight-frontend` — lint + type check pass
5. Playwright mocked test passes
