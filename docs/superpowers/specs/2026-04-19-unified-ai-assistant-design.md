# Unified AI Assistant — Shopping Assistant Enhancement

**Date:** 2026-04-19
**Issue:** #77 (Phase 2: Unified AI assistant UI)
**Branch:** `agent/feat-unified-ai-assistant`
**Scope:** Frontend + seed data. No backend changes.

## Goal

Enhance the existing Shopping Assistant drawer to showcase the full agentic architecture — ecommerce tools + RAG document search — with rich tool result rendering and pre-seeded product content so the demo is useful out of the box.

## Context

The Go ai-service already has 12 tools registered: 9 ecommerce tools (catalog, cart, orders) and 3 RAG tools (search_documents, ask_document, list_collections). The RAG bridge to Python services is wired up. But:

- The Shopping Assistant drawer renders all tool results as raw JSON
- The product catalog has only 20 items — too small for a shopping assistant to feel useful
- There are no product documents ingested, so the RAG tools return nothing
- There's no UI guidance pointing users toward the RAG capability

## Deliverables

### 1. Expanded Product Catalog (~55 products)

Grow `go/ecommerce-service/seed.sql` from 20 to ~55 products across the existing 5 categories:

| Category | Current | Target | Examples of additions |
|----------|---------|--------|----------------------|
| Electronics | 4 | 12 | Laptop, tablet, monitor, smartwatch, wireless earbuds, webcam, USB hub, router |
| Clothing | 4 | 10 | Running shoes, denim jacket, polo shirt, hiking boots, athletic shorts, hoodie |
| Home | 4 | 12 | Robot vacuum, air purifier, smart thermostat, knife set, stand mixer, French press, towel set, cutting board |
| Books | 4 | 10 | AI/ML books, cloud infra, database internals, observability, Kubernetes, networking |
| Sports | 4 | 11 | Running watch, foam roller, pull-up bar, jump rope, gym bag, cycling gloves, hiking backpack |

Same idempotent pattern: `WHERE NOT EXISTS (SELECT 1 FROM products)`. Prices in cents, empty `image_url`. Descriptions should be detailed enough for the LLM to make useful search matches.

### 2. Product PDFs for RAG (~8 documents)

Create product documentation in `docs/product-catalog/` and commit to the repo.

**Buying Guides (3):**
- `electronics-buying-guide.pdf` — laptop/monitor/audio comparison tables, feature explanations, compatibility notes
- `home-kitchen-guide.pdf` — cookware materials, appliance features, care instructions
- `fitness-equipment-guide.pdf` — equipment selection (dumbbells, bands, mats), sizing, workout suggestions

**Individual Spec Sheets (5):**
- One per "hero" product (e.g., laptop, monitor, smartwatch, robot vacuum, stand mixer)
- 1-2 pages each: dimensions, weight, power, compatibility, warranty, detailed features
- Product names must match `seed.sql` exactly so the agent can correlate catalog results with document search

### 3. PDF Seed Script

`scripts/seed-product-docs.sh` — ingests PDFs into the RAG system:
- Checks if a `product-docs` collection already exists via the ingestion API's `list_collections`
- If not, uploads each PDF from `docs/product-catalog/` via `POST /ingestion/ingest` (multipart form data)
- Idempotent — safe to re-run on every deploy
- Runs as a step in `k8s/deploy.sh` after Python AI services are up
- Can be run manually for local dev: `./scripts/seed-product-docs.sh http://localhost:8001`

### 4. Rich Tool Result Rendering

Replace the generic `AiToolCallCard` JSON dump with typed components based on the `display.kind` field.

**New components** in `frontend/src/components/go/`:

| `display.kind` | Component | Renders |
|----------------|-----------|---------|
| `product_list` | `ProductListResult` | Compact rows: emoji, name, category, formatted price |
| `product_card` | `ProductCardResult` | Single product: name, description, price, stock |
| `cart` | `CartResult` | Line items with quantities and totals |
| `cart_item` | `CartItemResult` | "Added X to cart" confirmation |
| `search_results` | `SearchResultsResult` | Ranked chunks: text preview, filename badge, page, score |
| `rag_answer` | `RagAnswerResult` | Answer text + source citation badges |
| `order_list` | `OrderListResult` | Order ID, status badge, total, date |
| `order_card` | `OrderCardResult` | Single order detail |
| `inventory` | `InventoryResult` | Stock count + in/out indicator |
| `collections_list` | `CollectionsResult` | Collection names + document counts |
| `return_confirmation` | `ReturnResult` | Return ID, status, reason |

**Routing:** A `ToolResultDisplay` component switches on `display.kind`. Unknown kinds fall back to formatted JSON.

**Source labels:** Tool results from catalog tools get a blue left border + "CATALOG SEARCH" label. RAG tool results get a green left border + "PRODUCT KNOWLEDGE" label. This makes it visually obvious which system answered.

**Tool call indicator:** While a tool is running, show a subtle inline indicator (tool name + status dot). Expand to the rich result on completion.

### 5. Enhanced Drawer UX

**Collapsible context panel** at the top of the drawer:
- Two columns: "Product Catalog" (blue) and "Product Knowledge" (green)
- Catalog column: brief description of what it can do (search, cart, orders)
- Knowledge column: brief description of document types available, plus a link to `/ai/rag` for uploads
- "TRY ASKING:" section with 3 clickable sample questions that demonstrate both catalog and RAG queries
- Panel auto-collapses after the first message to preserve chat space

**Sample questions** (clickable, populate the input):
- "Compare laptops under $1000" (triggers catalog search)
- "What's the battery life of the Laptop Pro 15?" (triggers RAG document search)
- "Which cookware is oven-safe?" (triggers RAG buying guide search)

## What's NOT Changing

- The drawer stays mounted in the ecommerce layout only (`/go/ecommerce/layout.tsx`)
- No new pages — the `/ai/rag` page stays as-is
- No backend changes — Go ai-service, Python services, and the RAG bridge are untouched
- No changes to the debug assistant
- No global AI assistant page
- The drawer toggle button, position, and size stay the same

## Technical Notes

- The Go ai-service `display` payload already includes a `kind` field on every tool result, making the frontend routing straightforward
- The `sendChat()` async generator in `ai-service.ts` already yields typed `AiEvent` objects — no changes needed to the SSE client
- Tool results feed `display` to the frontend and `content` (different, LLM-optimized) back to the agent loop — the rich rendering is purely a frontend concern
- The Python ingestion service's `/ingest` endpoint accepts multipart form uploads and returns `{filename, chunks_created}` — the seed script just needs to call this per PDF
- The ingestion API accepts `?collection=<name>` on `/ingest` — the seed script uploads all PDFs to a `product-docs` collection. If omitted, documents go to the default `documents` collection. Qdrant collections are created automatically on first insert.
