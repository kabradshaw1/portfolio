# Frontend Design Spec — Document Q&A Assistant

## Overview

A single-page Next.js chat interface for the Document Q&A Assistant. Users upload PDFs and ask questions, receiving streamed AI answers with source citations. The frontend is a thin shell over the backend RAG pipeline — styled enough to look professional in a portfolio, but not a showcase of frontend skills.

## Goals

- Functional demo of the full RAG pipeline (upload → ask → streamed answer with citations)
- Professional appearance via shadcn/ui — no custom design work
- Deployable to Vercel with configurable backend URL (tunnel to local backend for live demos)
- Minimal code surface — 4 components, no routing, no state library

## Architecture

Single Next.js 14 app (App Router). Everything lives on one page. The backend URLs are configured via `NEXT_PUBLIC_INGESTION_API_URL` and `NEXT_PUBLIC_CHAT_API_URL` environment variables, defaulting to `http://localhost:8001` and `http://localhost:8002` respectively for local development. On Vercel, these point to a tunnel (ngrok/Cloudflare) exposing the backend on the Windows machine.

### Components

| Component | Purpose | shadcn/ui parts |
|-----------|---------|-----------------|
| `ChatWindow` | Scrollable message list, auto-scrolls on new tokens | ScrollArea, Card |
| `MessageInput` | Text input + send button, disabled while streaming | Input, Button |
| `FileUpload` | Header button, opens file picker, POSTs to `/ingest`, shows status | Button |
| `SourceBadge` | Small pill showing `filename, p.N` below assistant messages | Badge |

### File Structure

```
frontend/
├── package.json
├── next.config.js
├── .env.local.example
├── src/
│   ├── app/
│   │   ├── layout.tsx        # HTML shell, fonts, metadata
│   │   ├── page.tsx          # Main page, assembles components
│   │   └── globals.css       # Tailwind imports
│   └── components/
│       ├── ChatWindow.tsx    # Message list with auto-scroll
│       ├── MessageInput.tsx  # Text input + send button
│       ├── FileUpload.tsx    # PDF upload button + status
│       └── SourceBadge.tsx   # Citation pill
```

## Layout

Single-page layout with three zones:

1. **Header** — app title ("Document Q&A Assistant"), document count, Upload PDF button
2. **Chat area** — full remaining height, scrollable. User messages right-aligned (blue), assistant messages left-aligned (dark). Source citation badges appear below each assistant message.
3. **Input bar** — fixed at bottom. Text field + Send button.

## Data Flow

### Chat Flow

1. User types question in `MessageInput`, hits Send (or Enter)
2. Component calls `POST {CHAT_API_URL}/chat` with `{ "question": "...", "collection": "default" }`
3. Response is SSE stream — `ChatWindow` appends tokens to the current assistant message in real-time
4. Final SSE event `{ "done": true, "sources": [...] }` triggers rendering of `SourceBadge` components below the message
5. `MessageInput` re-enables

### Upload Flow

1. User clicks "Upload PDF" in header
2. `FileUpload` opens native file picker (accept `.pdf`)
3. Selected file POSTed as multipart to `{INGESTION_API_URL}/ingest`
4. On success: inline status message "Uploaded test.pdf (42 chunks)"
5. Document count in header updates

### SSE Consumption

Use `fetch` with `ReadableStream` to consume the SSE stream from `/chat`. No external SSE library needed. Parse each `data: {...}` line, extract `token` field for streaming display, and `done`/`sources` fields for completion.

## State Management

React `useState` only. No external state library.

| State | Type | Location |
|-------|------|----------|
| `messages` | `Array<{ role: 'user' \| 'assistant', content: string, sources?: Source[] }>` | `page.tsx` |
| `isStreaming` | `boolean` | `page.tsx` |
| `documentCount` | `number` | `page.tsx` |
| `uploadStatus` | `string \| null` | `FileUpload` |

## Styling

- **shadcn/ui** for all interactive components (Button, Input, ScrollArea, Card, Badge)
- **Tailwind CSS** for layout and spacing
- **Dark theme** — consistent with the mockup. shadcn's dark mode.
- **No custom CSS** beyond Tailwind utilities

## Environment Variables

```
# .env.local.example
NEXT_PUBLIC_INGESTION_API_URL=http://localhost:8001
NEXT_PUBLIC_CHAT_API_URL=http://localhost:8002
```

On Vercel, set these to the tunnel URL (e.g., `https://your-tunnel.ngrok.io:8001`).

## CORS

The FastAPI backend services need CORS middleware added to allow requests from the Vercel origin. This is a backend change, not a frontend concern, but is required for the Vercel deployment to work.

```python
# Added to both services' main.py
from fastapi.middleware.cors import CORSMiddleware

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Tighten for production
    allow_methods=["*"],
    allow_headers=["*"],
)
```

## Error Handling

- **Ollama/backend unreachable:** Show inline error message in chat: "Could not connect to the backend. Make sure the services are running."
- **Upload fails:** Show error status in header area near upload button
- **Empty response:** If SSE stream completes with no tokens, show "No response received"
- **Non-PDF file:** File picker restricted to `.pdf` via `accept` attribute. Backend also validates.

## Testing

Minimal — the frontend is a thin UI layer:

- Manual smoke test: upload PDF, ask question, verify streaming + citations
- No unit tests for components (the backend is the tested surface)

## Tech Stack

- Next.js 14 (App Router)
- shadcn/ui (Card, Button, Input, ScrollArea, Badge)
- Tailwind CSS
- TypeScript
- No additional runtime dependencies

## Out of Scope

- Document list sidebar (may add later)
- Multi-collection support in UI (backend supports it, frontend uses "default")
- Authentication
- Mobile-optimized layout
- Component unit tests
