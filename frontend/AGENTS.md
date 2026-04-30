<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes - APIs, conventions, and file structure may
all differ from your training data. Read the relevant guide in
`node_modules/next/dist/docs/` before writing any code. Heed deprecation
notices.
<!-- END:nextjs-agent-rules -->

# Frontend

The frontend is a Next.js + TypeScript application using shadcn/ui and Apollo
Client.

## Local Development

Run the frontend from `frontend/`:

```bash
npm run dev
```

Backend traffic normally points at `localhost:8000` through the SSH tunnel:

```bash
ssh -f -N -L 8000:localhost:8000 debian
```

Do not use Debian as a build or test worker for frontend checks.

## Vercel

Vercel CLI is installed and linked to `kabradshaw1s-projects`.

Useful commands:

```bash
vercel env ls production
vercel env add
vercel redeploy
```

Critical deployment rule: if frontend code adds a new `NEXT_PUBLIC_*` env var
with a `localhost` fallback, add the variable in Vercel and trigger a redeploy
before merging. Otherwise Vercel can bake the localhost fallback into the
production bundle.

## Browser Console Configuration

When console configuration is needed, first check whether the CLI can perform
the change. If not, give Kyle links to the exact pages to visit and the
specific values to configure.

## Verification

Before committing frontend changes, run:

```bash
make preflight-frontend
make preflight-e2e
```
